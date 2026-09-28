import { describe, expect, it } from 'vitest';
import {
	activeFacilityCount,
	activeQueueCount,
	activeScheduleCount,
	attentionItems,
	headlineMetrics,
	nextSchedule,
	scheduledAppointmentCount
} from './ringkasan';
import {
	NOTIFICATION_DEFAULT_LIMIT,
	NOTIFICATION_MAX_LIMIT,
	NOTIFICATION_PAGE_LIMIT,
	NOTIFICATION_STATUS_ORDER,
	applyFiltersToUrl,
	buildNotificationQuery,
	emptyFilters,
	filtersFromParams,
	hasActiveFilters,
	zeroSummary
} from './notifications';
import { facilityStateLabel, isFacilityActive } from './facilityState';
import type {
	AdminAppointment,
	AdminFacility,
	AdminQueueTicket,
	AdminSchedule
} from '$lib/api/types/api';

/**
 * T-3B4-03 / 07 / 08: the derived-value contracts.
 *
 * These are the claims that are invisible in a screenshot and expensive in
 * production: that no number on Ringkasan was invented, that the notification
 * list claims no pagination it cannot perform, and that a deactivated facility
 * cannot be rendered as active.
 */

function queue(status: AdminQueueTicket['status'], id: string): AdminQueueTicket {
	return {
		id,
		facility_id: 'f1',
		queue_number: 1,
		formatted_number: 'RSK-0001',
		status,
		registered_at: '2026-09-28T10:00:00Z'
	};
}

function appointment(status: AdminAppointment['status'], id: string): AdminAppointment {
	return {
		id,
		facility_id: 'f1',
		service_unit_id: 'u1',
		status,
		patient_display_name: 'Pasien'
	};
}

function schedule(over: Partial<AdminSchedule> = {}): AdminSchedule {
	return {
		id: 's1',
		facility_id: 'f1',
		service_unit_id: 'u1',
		schedule_date: '2026-09-30',
		start_time: '08:00:00',
		end_time: '12:00:00',
		slot_minutes: 30,
		capacity_per_slot: 4,
		is_active: true,
		...over
	};
}

function facility(active: boolean, id = 'f1'): AdminFacility {
	return {
		id,
		name: 'RSUD Kota Sehat',
		type: 'rumah_sakit',
		address: '',
		kecamatan: '',
		kabupaten_kota: '',
		provinsi: '',
		phone: '',
		total_beds: 10,
		available_beds: 8,
		is_active: active,
		short_code: 'RSK'
	};
}

describe('Ringkasan: every figure is derived from the scoped lists', () => {
	const input = {
		queues: [
			queue('waiting', 'q1'),
			queue('waiting', 'q2'),
			queue('called', 'q3'),
			queue('in_service', 'q4'),
			queue('completed', 'q5'),
			queue('cancelled', 'q6')
		],
		appointments: [
			appointment('scheduled', 'a1'),
			appointment('scheduled', 'a2'),
			appointment('checked_in', 'a3'),
			appointment('no_show', 'a5'),
			appointment('completed', 'a4')
		],
		schedules: [schedule(), schedule({ id: 's2', is_active: false })],
		facilities: [facility(true), facility(false, 'f2')]
	};

	it('counts an active queue as everything not yet closed', () => {
		// A completed or cancelled ticket is history; counting it would make a quiet
		// afternoon look like a backlog.
		expect(activeQueueCount(input.queues)).toBe(4);
	});

	it('counts scheduled appointments only', () => {
		expect(scheduledAppointmentCount(input.appointments)).toBe(2);
	});

	it('counts active schedules and facilities', () => {
		expect(activeScheduleCount(input.schedules)).toBe(1);
		expect(activeFacilityCount(input.facilities)).toBe(1);
	});

	it('labels every headline figure with the source it came from', () => {
		// A count an operator cannot trace is a number they have to take on faith.
		for (const metric of headlineMetrics(input)) {
			expect(metric.source, `${metric.label} must name its source`).toBeTruthy();
			expect(typeof metric.value).toBe('number');
		}
	});

	it('cross-links only to real admin destinations', () => {
		for (const metric of headlineMetrics(input)) {
			if (!metric.href) continue;
			expect(metric.href, `${metric.label} links outside admin`).toMatch(/^\/admin(\/|$)/);
		}
	});

	it('invents no percentage, trend, SLA, or occupancy figure', () => {
		// Each of these would need a denominator or a history the scoped reads do
		// not carry, and an invented number on a clinic dashboard gets acted on.
		const metrics = headlineMetrics(input);
		expect(metrics.every((m) => Number.isInteger(m.value))).toBe(true);
		for (const metric of metrics) {
			const text = `${metric.label} ${metric.detail ?? ''}`.toLowerCase();
			for (const forbidden of ['%', 'persen', 'trend', 'naik', 'turun', 'sla', 'okupansi']) {
				expect(text, `Ringkasan must not claim ${forbidden}`).not.toContain(forbidden);
			}
		}
	});

	it('never derives a notification count from the lists it does not load', () => {
		// Ringkasan does not read the outbox, so any figure there is invented — and
		// a hardcoded zero is the most dangerous kind, because it asserts that
		// nothing has ever failed and an operator stops checking.
		const labels = attentionItems(input).map((item) => item.label.toLowerCase());
		for (const label of labels) {
			expect(label).not.toContain('notifikasi');
		}
		expect(labels.some((l) => l.includes('menunggu'))).toBe(true);
		expect(labels.some((l) => l.includes('tidak datang'))).toBe(true);
	});

	it('reports nothing to attend to when the scoped data is genuinely quiet', () => {
		const quiet = {
			queues: [queue('completed', 'q1')],
			appointments: [appointment('completed', 'a1')],
			schedules: [],
			facilities: [facility(true)]
		};
		// An attention list that never empties trains an operator to ignore it.
		expect(attentionItems(quiet)).toEqual([]);
	});

	it('picks the next active schedule by date then start time', () => {
		const schedules = [
			schedule({ id: 'later', schedule_date: '2026-10-05' }),
			schedule({ id: 'same-day-early', schedule_date: '2026-09-30', start_time: '08:00:00' }),
			schedule({ id: 'same-day-late', schedule_date: '2026-09-30', start_time: '13:00:00' })
		];
		const next = nextSchedule(schedules);
		expect(next?.id).toBe('same-day-early');
	});

	it('skips an inactive or unparseable schedule rather than showing it first', () => {
		// An unparseable date means the record is malformed, and putting it first
		// would present the least trustworthy row as the most urgent.
		const schedules = [
			schedule({ id: 'broken', schedule_date: 'not-a-date' }),
			schedule({ id: 'inactive', is_active: false })
		];
		expect(nextSchedule(schedules)).toBeNull();
	});
});

describe('notifications: no pagination that cannot work', () => {
	it('uses the backend default rather than inventing a page size', () => {
		expect(NOTIFICATION_DEFAULT_LIMIT).toBe(100);
		expect(NOTIFICATION_MAX_LIMIT).toBe(500);
		expect(NOTIFICATION_PAGE_LIMIT).toBe(NOTIFICATION_DEFAULT_LIMIT);
	});

	it('sends no offset or page parameter, because the endpoint has none', () => {
		const query = buildNotificationQuery(emptyFilters());
		// A page-number control over an endpoint with no cursor would be a control
		// that cannot work, and "of N" would imply a total the server never sent.
		expect(query).not.toHaveProperty('offset');
		expect(query).not.toHaveProperty('page');
		expect(query).not.toHaveProperty('limit');
	});

	it('always offers all five status cards, zero included', () => {
		// A card that vanishes when its count is zero is how a backlog becomes
		// invisible.
		expect(NOTIFICATION_STATUS_ORDER).toHaveLength(5);
		expect(zeroSummary()).toEqual({
			pending: 0,
			processing: 0,
			delivered: 0,
			failed: 0,
			cancelled: 0
		});
	});

	it('round-trips filters through the URL so a shared link reproduces the view', () => {
		const filters = {
			status: 'failed',
			channel: 'sms',
			templateKey: 'reminder',
			createdFrom: '2026-09-01',
			createdTo: '2026-09-28'
		};
		const url = applyFiltersToUrl(new URL('http://local/admin/notifications'), filters);
		const restored = filtersFromParams(url.searchParams);
		expect(restored).toEqual(filters);
	});

	it('deletes an unset filter rather than sending an empty one', () => {
		// `?status=` is a different request to the backend than an absent one: it
		// matches the empty status and returns nothing.
		const url = applyFiltersToUrl(
			new URL('http://local/admin/notifications?status=failed'),
			emptyFilters()
		);
		expect(url.searchParams.has('status')).toBe(false);
		expect(filtersFromParams(url.searchParams).status).toBe('');
	});

	it('expands a date filter to the whole day', () => {
		const query = buildNotificationQuery({
			...emptyFilters(),
			createdFrom: '2026-09-01',
			createdTo: '2026-09-28'
		});
		expect(query.created_from).toBe('2026-09-01T00:00:00Z');
		expect(query.created_to).toBe('2026-09-28T23:59:59Z');
	});

	it('reports whether any filter is narrowing', () => {
		expect(hasActiveFilters(emptyFilters())).toBe(false);
		expect(hasActiveFilters({ ...emptyFilters(), status: 'failed' })).toBe(true);
	});
});

describe('facility state: the is_active string trap', () => {
	it('reads a real boolean unchanged', () => {
		expect(isFacilityActive(true)).toBe(true);
		expect(isFacilityActive(false)).toBe(false);
	});

	it('reads the deactivate response STRING "false" as inactive', () => {
		// The backend builds a map[string]string for that response, so the same
		// field arrives as a string. Boolean("false") is TRUE in JavaScript, so the
		// obvious coercion would render a just-deactivated facility as active — on
		// the screen whose whole job is saying which facilities still take patients.
		expect(isFacilityActive('false')).toBe(false);
		expect(isFacilityActive('FALSE')).toBe(false);
		expect(isFacilityActive(' false ')).toBe(false);
	});

	it('reads the string "true" as active', () => {
		expect(isFacilityActive('true')).toBe(true);
		expect(isFacilityActive('TRUE')).toBe(true);
	});

	it('fails safe on an unrecognised value', () => {
		// Showing a facility as active when its state is unknown is recoverable;
		// hiding a live one sends a patient to a closed door.
		expect(isFacilityActive(null)).toBe(true);
		expect(isFacilityActive(undefined)).toBe(true);
		expect(isFacilityActive('unknown')).toBe(true);
	});

	it('always pairs the state with a word, never colour alone', () => {
		expect(facilityStateLabel(true)).toBe('Aktif');
		expect(facilityStateLabel(false)).toBe('Nonaktif');
	});
});
