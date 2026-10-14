import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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

	/**
	 * The reference "today" the schedule tests are written against.
	 *
	 * WHY THIS EXISTS. `nextSchedule` decides what "next" means by comparing each
	 * row against the REAL current date, so a test that hardcodes calendar
	 * literals is a test with an expiry date. This one did exactly that: it
	 * asserted a row dated `2026-09-30` was "today", which was true until the
	 * calendar rolled to `2026-10-01` and the implementation correctly reclassified
	 * that row as past. The failure was in the FIXTURE, never in the logic — but a
	 * green suite that comes back red on a date boundary is a suite nobody trusts,
	 * so the clock is pinned instead.
	 *
	 * WHY A PINNED CLOCK RATHER THAN DATES DERIVED FROM `Date.now()`. Deriving the
	 * fixtures from the real clock would make the test pass forever, but it would
	 * also make it assert almost nothing: the boundary it checks would move with
	 * the calendar, so "today" is only ever today and the PAST-row case could
	 * never actually be exercised. Pinning the clock lets one fixed set of
	 * literals assert a real past/today/future boundary, which is the whole point
	 * of the contract.
	 *
	 * WHY THE TIME IS LOCAL AND UNSUFFIXED. `nextSchedule` reads the local calendar
	 * day via `getFullYear`/`getMonth`/`getDate`, and it builds its comparison key
	 * the same way. A `Z`-suffixed instant would shift the local day for half the
	 * planet and make this test timezone-dependent — it would pass in Jakarta and
	 * fail in Los Angeles. `new Date('2026-09-30T12:00:00')` with no zone is parsed
	 * in local time, so the local calendar day is `2026-09-30` everywhere. Noon
	 * rather than midnight so a DST transition cannot move the day either.
	 *
	 * WHY EVERY FIXTURE IS DERIVED FROM `TODAY` RATHER THAN TYPED OUT. This is
	 * the part that makes the control below meaningful. An earlier draft of this
	 * repair pinned the clock but left the surrounding dates as independent
	 * literals — `2026-10-05`, `2026-09-29` and so on. Moving the pinned "today"
	 * then changed nothing the assertions depended on: the boundary being tested
	 * was never actually connected to the clock, so a negative control that moved
	 * "today" into the past still went green. A test whose parts do not reference
	 * each other cannot detect a violation of the relationship between them.
	 *
	 * Deriving each side from one anchor (`dayOffset`) makes the past/today/future
	 * relationship structural. `YESTERDAY` is yesterday BY CONSTRUCTION, so if the
	 * implementation ever stopped excluding past rows, these tests fail — which is
	 * exactly what the control demonstrates.
	 */
	const TODAY = new Date('2026-09-30T12:00:00');

	/**
	 * A local calendar day `offset` days from `TODAY`, as `YYYY-MM-DD`.
	 *
	 * Local-time arithmetic on purpose, to match how `nextSchedule` reads the
	 * current day. `setDate` on a local `Date` steps whole calendar days, so this
	 * cannot drift across a DST boundary the way adding milliseconds would.
	 */
	function dayOffset(offset: number): string {
		const day = new Date(TODAY);
		day.setDate(day.getDate() + offset);
		const month = String(day.getMonth() + 1).padStart(2, '0');
		return `${day.getFullYear()}-${month}-${String(day.getDate()).padStart(2, '0')}`;
	}

	const YESTERDAY = dayOffset(-1);
	const TOMORROW = dayOffset(1);
	const NEXT_WEEK = dayOffset(7);
	const LATER_STILL = dayOffset(20);

	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(TODAY);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('picks the next active schedule by date then start time', () => {
		const schedules = [
			// FUTURE, correctly considered but outranked: today still has rows.
			schedule({ id: 'later', schedule_date: NEXT_WEEK }),
			// TODAY, earliest start: the answer.
			schedule({ id: 'same-day-early', schedule_date: dayOffset(0), start_time: '08:00:00' }),
			// TODAY, later start: same day, so start time is the tiebreak.
			schedule({ id: 'same-day-late', schedule_date: dayOffset(0), start_time: '13:00:00' })
		];
		expect(nextSchedule(schedules)?.id).toBe('same-day-early');
	});

	it("excludes yesterday's rows even when they are the only ones it can see", () => {
		// THE PAST-HALF OF THE BOUNDARY, stated as its own claim.
		//
		// The test above proves today's rows win on start time, but it would still
		// pass if the implementation ignored "today or later" entirely and returned
		// the first future row. This one cannot: every row here is strictly before
		// today by construction, so the correct answer is NOTHING. An
		// implementation that dropped the `>= today` filter would return a row and
		// fail. Together the two pin both sides of the boundary.
		const past = [
			schedule({ id: 'yesterday', schedule_date: YESTERDAY }),
			schedule({ id: 'last-week', schedule_date: dayOffset(-7) })
		];
		expect(nextSchedule(past), 'a past row is not "next"').toBeNull();
	});

	it('falls forward to a future row when today has nothing left', () => {
		// The contract is "today or later", so a day with no remaining rows must
		// surface the next one rather than showing an empty board. This is the
		// third side of the boundary: today wins, past is excluded, future is the
		// fallback — and never an empty board while a future row exists.
		const futureOnly = [
			schedule({ id: 'tomorrow', schedule_date: TOMORROW }),
			schedule({ id: 'next-week', schedule_date: NEXT_WEEK }),
			schedule({ id: 'later-still', schedule_date: LATER_STILL })
		];
		expect(nextSchedule(futureOnly)?.id, 'the SOONEST future row wins').toBe('tomorrow');
	});

	it('treats today as still upcoming, not as elapsed', () => {
		// THE INCLUSIVE EDGE, which is the one an off-by-one here would silently
		// break. `>=` rather than `>`: a schedule running later TODAY is the most
		// useful thing an operator can be shown, so "upcoming" must include it. If
		// the comparison were ever tightened to `>`, today's rows would vanish and
		// an operator would be told there is nothing scheduled while the clinic is
		// in fact booked.
		const todayOnly = [
			schedule({ id: 'this-morning', schedule_date: dayOffset(0), start_time: '07:00:00' }),
			schedule({ id: 'this-evening', schedule_date: dayOffset(0), start_time: '19:00:00' })
		];
		expect(nextSchedule(todayOnly)?.id, 'today is still "next"').toBe('this-morning');
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
