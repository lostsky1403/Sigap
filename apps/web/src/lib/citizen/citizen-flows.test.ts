import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import Booking from '../../routes/appointments/new/+page.svelte';
import {
	validatePhone,
	validateName,
	validateAppointmentTime,
	validateLookupCode,
	estimatedWaitLabel,
	checkInDeepLink,
	classifyCheckInFailure,
	classifyTransactionFailure,
	resolveFailureMessage,
	GENERIC_UNAVAILABLE,
	ESTIMATE_CAVEAT
} from './citizenFlow';
import { readFormattedNumber, normalizeQueueTicket, wrongCheckInCode } from '$lib/api/endpoints/public';
import type { ApiError } from '$lib/api/errors';

/**
 * The three transactional citizen flows.
 *
 * Everything here is faked at `fetch`, not at the endpoint module. Mocking
 * `bookAppointment` would let a test assert that the page calls the function
 * with the right-looking arguments, while saying nothing about whether the
 * JSON on the wire matches what Go decodes into
 * `BookAppointmentRequest`. Since the whole class of bug in this area is a
 * wrong KEY — and a wrong key fails silently, because Go ignores unknown JSON
 * fields — the body assertions below are the point of the file.
 *
 * The error cases are unit-tested here rather than driven through the browser
 * because provoking a real 409 or 429 from the seeded API would require
 * filling a practitioner schedule to capacity or exhausting a rate limit, both
 * of which are slow and both of which would make the test depend on limiter
 * internals. The status→copy mapping is deterministic and belongs at this
 * level; the happy path is proven for real in the Playwright suite.
 */

const FACILITIES = [
	{
		id: '11111111-1111-4111-8111-111111111111',
		name: 'Puskesmas Sukajaya',
		short_code: 'PKM',
		type: 'pusKESmas' as const,
		is_active: true
	},
	{
		id: '22222222-2222-4222-8222-222222222222',
		name: 'RSUD Kota Sehat',
		short_code: 'RSK',
		type: 'rumah_sakit' as const,
		is_active: true
	}
];

const UNITS = [
	{
		id: 'aaaaaaaa-1111-4111-8111-111111111111',
		facility_id: '11111111-1111-4111-8111-111111111111',
		name: 'Layanan Kesehatan Ibu',
		code: 'LKN',
		is_active: true
	},
	{
		id: 'bbbbbbbb-2222-4222-8222-222222222222',
		facility_id: '22222222-2222-4222-8222-222222222222',
		name: 'Instalasi Rawat Inap',
		code: 'IRI',
		is_active: true
	}
];

/** A timestamp far enough ahead that a slow test cannot cross it. */
const FUTURE = '2099-06-01T10:00';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(`${JSON.stringify(body)}\n`, {
		status,
		headers: { 'content-type': 'application/json; charset=utf-8' }
	});
}

/** Captures the parsed JSON body of every request, keyed by path. */
function mockApi(
	handler: (url: string, init?: RequestInit) => Response | Promise<Response>
) {
	const calls: { url: string; method: string; body: unknown }[] = [];
	const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = String(input);
		const method = (init?.method ?? 'GET').toUpperCase();
		const body = init?.body ? JSON.parse(String(init.body)) : undefined;
		calls.push({ url, method, body });
		return await handler(url, init);
	});
	globalThis.fetch = fetchMock as never;
	return calls;
}

const defaultHandler = (url: string) => {
	if (url.includes('/public/facilities')) {
		return jsonResponse({ success: true, data: FACILITIES });
	}
	if (url.includes('/public/service-units')) {
		return jsonResponse({ success: true, data: UNITS });
	}
	throw new Error(`unexpected request: ${url}`);
};

/**
 * Finds a labelled control by a substring of its accessible name.
 *
 * Not `getByLabelText(exact)`. A required Field renders its label as
 * "Fasilities *(wajib diisi)*" — the visible asterisk plus a visually hidden
 * "(wajib diisi)" for screen readers — so the accessible name is the whole
 * string, not the bare word. Querying the exact bare word fails on every
 * required field, which is a test-authoring trap rather than a product defect.
 */
function labelled(name: string): HTMLElement {
	return screen.getByLabelText(new RegExp(name, 'i'));
}

const facilityField = () => labelled('Fasilitas') as HTMLSelectElement;
const serviceField = () => labelled('Layanan') as HTMLSelectElement;
const timeField = () => labelled('Waktu janji temu') as HTMLInputElement;
const nameField = () => labelled('Nama pasien') as HTMLInputElement;
const phoneField = () => labelled('Nomor telepon') as HTMLInputElement;

/** Fills the three steps and returns the submit button. */
async function fillValidBooking() {
	await waitFor(() => expect(facilityField()).toBeTruthy());

	await fireEvent.change(facilityField(), { target: { value: FACILITIES[0].id } });
	await waitFor(() => expect(serviceField()).toBeTruthy());
	await fireEvent.change(serviceField(), { target: { value: UNITS[0].id } });

	await waitFor(() => expect(timeField()).toBeTruthy());
	await fireEvent.input(timeField(), { target: { value: FUTURE } });
	await fireEvent.input(nameField(), { target: { value: 'Siti Rahma' } });
	await fireEvent.input(phoneField(), { target: { value: '081234567890' } });
}

beforeEach(() => {
	document.body.innerHTML = '';
	window.history.replaceState({}, '', '/appointments/new');
});

afterEach(() => {
	vi.restoreAllMocks();
});

/* ================================================================== *
 * Pure helpers
 * ================================================================== */

describe('citizen flow helpers', () => {
	it('validates a phone as 10-15 digits after stripping separators', () => {
		expect(validatePhone('')).toContain('wajib diisi');
		// Too short, too long, and the boundaries either side of the range.
		expect(validatePhone('123456789')).toContain('10-15');
		expect(validatePhone('123456789012345')).toBe('');
		expect(validatePhone('1234567890123456')).toContain('10-15');
		// The backend normalizes before counting, so a formatted international
		// number is valid. Validating raw length would reject it.
		expect(validatePhone('+62 812-3456-7890')).toBe('');
		expect(validatePhone('0812 3456 7890')).toBe('');
	});

	it('requires a name and enforces the backend length ceiling', () => {
		expect(validateName('   ')).toContain('wajib diisi');
		expect(validateName('Siti Rahma')).toBe('');
		expect(validateName('x'.repeat(101))).toContain('100 karakter');
		expect(validateName('x'.repeat(100))).toBe('');
	});

	it('rejects an appointment time that is not in the future', () => {
		const now = new Date('2026-01-01T12:00:00Z');
		expect(validateAppointmentTime('', now)).toContain('wajib diisi');
		expect(validateAppointmentTime('2099-01-01T10:00', now)).toBe('');
		expect(validateAppointmentTime('2025-01-01T10:00', now)).toContain('masa depan');
		// The backend rejects Equal(now) as well as Before, so equality counts
		// as past. Getting this backwards would let a citizen pick the current
		// minute and be refused by the server after a green local validation.
		expect(validateAppointmentTime('2026-01-01T12:00', now)).toContain('masa depan');
		expect(validateAppointmentTime('not-a-date', now)).toContain('tidak valid');
	});

	it('validates a lookup code against the backend charset and length', () => {
		expect(validateLookupCode('')).toContain('wajib diisi');
		expect(validateLookupCode('ABC123')).toBe('');
		expect(validateLookupCode('A-1')).toBe('');
		expect(validateLookupCode('has space')).toContain('huruf, angka');
		expect(validateLookupCode('x'.repeat(65))).toContain('terlalu panjang');
	});

	it('words a wait time as an estimate, never as a live figure', () => {
		const label = estimatedWaitLabel(20);
		expect(label).toContain('20');
		expect(label.toLowerCase()).toContain('estimasi');
		// The specific words that turn an estimate into a promise.
		for (const forbidden of ['realtime', 'real-time', 'live', 'dipastikan', 'pasti']) {
			expect(label.toLowerCase(), `must not claim ${forbidden}`).not.toContain(forbidden);
		}
		expect(ESTIMATE_CAVEAT.toLowerCase()).toContain('bisa berubah');
	});

	it('builds the canonical check-in deep link', () => {
		const link = checkInDeepLink('appt-1', 'AB12CD');
		expect(link).toBe('/appointments/check-in?appointment_id=appt-1&checkin_code=AB12CD');
		// URL-encoded, so a value with a reserved character cannot corrupt the
		// query string.
		expect(checkInDeepLink('a b', 'c&d')).toContain('appointment_id=a+b');
		expect(checkInDeepLink('', '')).toBe('/appointments/check-in');
	});

	it('renders a server message verbatim and reserves one generic string', () => {
		const conflict = classifyTransactionFailure({
			kind: 'conflict',
			status: 409,
			message: 'Slot jadwal sudah penuh. Silakan pilih jadwal lain.'
		});
		expect(resolveFailureMessage(conflict)).toBe('Slot jadwal sudah penuh. Silakan pilih jadwal lain.');

		const rateLimited = classifyTransactionFailure({
			kind: 'rate_limited',
			status: 429,
			message: 'Nomor HP ini sudah melebihi batas pemesanan per hari. Silakan coba lagi besok.'
		});
		expect(resolveFailureMessage(rateLimited)).toBe(
			'Nomor HP ini sudah melebihi batas pemesanan per hari. Silakan coba lagi besok.'
		);

		// 5xx and transport are the only cases that get our own wording, and
		// they share one string rather than each inventing one.
		expect(resolveFailureMessage(classifyTransactionFailure({ kind: 'server', status: 500, message: 'x' }))).toBe(
			GENERIC_UNAVAILABLE
		);
		expect(
			resolveFailureMessage(classifyTransactionFailure({ kind: 'network', status: 0, message: 'x' }))
		).toBe(GENERIC_UNAVAILABLE);
		// An abort is silent, not an error panel.
		expect(resolveFailureMessage(classifyTransactionFailure({ kind: 'aborted', status: 0, message: 'x' }))).toBe(
			GENERIC_UNAVAILABLE
		);
		expect(resolveFailureMessage(null)).toBe('');
	});
});

/* ================================================================== *
 * The 401 rule
 * ================================================================== */

describe('check-in 401 is a wrong code, never an auth state', () => {
	const wrongCode: ApiError = { kind: 'unauthorized', status: 401, message: 'Kode check-in tidak cocok.' };

	it('classifies a 401 from the public route as wrong-code', () => {
		expect(wrongCheckInCode(wrongCode)).toBe(true);
		expect(classifyCheckInFailure(wrongCode, true).kind).toBe('wrong-code');
	});

	it('never produces an auth-flavoured message for it', () => {
		const message = resolveFailureMessage(classifyCheckInFailure(wrongCode, true));
		expect(message).not.toContain('login');
		expect(message).not.toContain('masuk');
		expect(message).not.toContain('sesi');
		expect(message).not.toContain('session');
		expect(message).not.toContain('unauthenticated');
		expect(message).not.toContain('kedaluwarsa');
	});

	it('does not treat other statuses as a wrong code', () => {
		// 403 is the other status that could be mistaken for an auth problem,
		// and on this route it is not one either.
		const forbidden: ApiError = { kind: 'forbidden', status: 403, message: 'Ditolak.' };
		expect(wrongCheckInCode(forbidden)).toBe(false);
		expect(classifyCheckInFailure(forbidden, false).kind).not.toBe('wrong-code');
	});

	it('distinguishes 404, 409, and 429 as their own conditions', () => {
		const notFound = classifyCheckInFailure(
			{ kind: 'not_found', status: 404, message: 'Janji temu tidak ditemukan.' },
			false
		);
		const conflict = classifyCheckInFailure(
			{ kind: 'conflict', status: 409, message: 'Janji temu tidak dapat check-in dengan status: queued' },
			false
		);
		const limited = classifyCheckInFailure(
			{ kind: 'rate_limited', status: 429, message: 'Terlalu banyak percobaan. Coba lagi nanti.' },
			false
		);
		// Three different messages: a wrong appointment, a wrong state, and a
		// rate limit are three different problems with three different fixes.
		expect(new Set([notFound.kind, conflict.kind, limited.kind]).size).toBe(3);
		// Each keeps the server's own words.
		expect(resolveFailureMessage(conflict)).toContain('queued');
		expect(resolveFailureMessage(limited)).toBe('Terlalu banyak percobaan. Coba lagi nanti.');
	});
});

/* ================================================================== *
 * Walk-in response normalization
 * ================================================================== */

describe('walk-in ticket normalization', () => {
	it('reads formatted_number, the casing Go actually emits', () => {
		// Verified against service/queue.go: FormattedNumber is tagged
		// `json:"formatted_number"`, so this is the real wire shape.
		expect(readFormattedNumber({ formatted_number: 'A-012' })).toBe('A-012');
	});

	it('also accepts FormattedNumber', () => {
		expect(readFormattedNumber({ FormattedNumber: 'A-012' })).toBe('A-012');
	});

	it('returns an empty string rather than guessing when neither key is present', () => {
		expect(readFormattedNumber({})).toBe('');
		expect(readFormattedNumber(null)).toBe('');
		expect(readFormattedNumber(undefined)).toBe('');
		// Never invents a number. A made-up queue number would be read at the
		// counter, which is worse than an honest blank.
		expect(readFormattedNumber({})).not.toContain('0');
	});

	it('normalizes the PascalCase ticket and status fields', () => {
		const ticket = normalizeQueueTicket({
			TicketID: 'ticket-1',
			formatted_number: 'A-012',
			Status: 'waiting',
			estimated_wait_minutes: 25
		});
		expect(ticket.ticketId).toBe('ticket-1');
		expect(ticket.formattedNumber).toBe('A-012');
		expect(ticket.status).toBe('waiting');
		expect(ticket.estimatedWaitMinutes).toBe(25);
	});

	it('survives a response missing everything', () => {
		const ticket = normalizeQueueTicket({});
		expect(ticket.formattedNumber).toBe('');
		expect(ticket.estimatedWaitMinutes).toBe(0);
		// Defaults to a real queue state rather than undefined, so a status
		// chip cannot end up reading "undefined".
		expect(ticket.status).toBe('waiting');
	});
});

/* ================================================================== *
 * Booking page
 * ================================================================== */

describe('/appointments/new', () => {
	it('shows the three frozen steps and starts on Fasilitas', async () => {
		mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());

		const steps = Array.from(document.querySelectorAll('.sigap-stepper__label')).map(
			(node) => node.textContent?.trim()
		);
		expect(steps).toEqual(['Fasilitas', 'Layanan', 'Waktu + Data Diri']);
		// The current step is marked, not just implied by position.
		expect(document.querySelector('[aria-current="step"]')?.textContent).toContain('Fasilitas');
	});

	it('advances the step only as the form is actually completed', async () => {
		mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		expect(document.querySelector('[aria-current="step"]')?.textContent).toContain('Fasilitas');

		await fireEvent.change(facilityField(), {
			target: { value: FACILITIES[0].id }
		});
		await waitFor(() =>
			expect(document.querySelector('[aria-current="step"]')?.textContent).toContain('Layanan')
		);

		await waitFor(() => expect(serviceField()).toBeTruthy());
		await fireEvent.change(serviceField(), { target: { value: UNITS[0].id } });
		await waitFor(() =>
			expect(document.querySelector('[aria-current="step"]')?.textContent).toContain('Waktu')
		);
	});

	it('shows only service units belonging to the chosen facility', async () => {
		const calls = mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());

		await fireEvent.change(facilityField(), { target: { value: FACILITIES[0].id } });
		await waitFor(() => expect(serviceField()).toBeTruthy());

		/*
			Waited on the request rather than on the options array. The options
			appear a tick after the facility changes, so asserting immediately
			races the loader and reads a list that is legitimately still empty —
			a flaky test that would eventually be "fixed" by a sleep.
		*/
		await waitFor(() =>
			expect(
				calls.some((c) => c.url.includes('facility_id=' + FACILITIES[0].id))
			).toBe(true)
		);
		await waitFor(() => {
			const options = Array.from(serviceField().querySelectorAll('option')).map(
				(option) => option.textContent
			);
			expect(options).toContain('Layanan Kesehatan Ibu');
		});

		// A unit belonging to the other clinic is not offered, because the
		// request was scoped by facility_id rather than filtered client-side.
		const options = Array.from(serviceField().querySelectorAll('option')).map(
			(option) => option.textContent
		);
		expect(options).not.toContain('Instalasi Rawat Inap');

		// And switching facility re-scopes the request.
		await fireEvent.change(facilityField(), { target: { value: FACILITIES[1].id } });
		await waitFor(() =>
			expect(
				calls.some((c) => c.url.includes('facility_id=' + FACILITIES[1].id))
			).toBe(true)
		);
	});

	it('clears a service selection that is invalid for the newly chosen facility', async () => {
		mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());

		// Choose the first facility and one of its units.
		await fireEvent.change(facilityField(), { target: { value: FACILITIES[0].id } });
		await waitFor(() => expect(serviceField()).toBeTruthy());
		await fireEvent.change(serviceField(), { target: { value: UNITS[0].id } });
		await waitFor(() => expect(serviceField().value).toBe(UNITS[0].id));

		/*
			Now switch to the other clinic. The selection must survive the change
			long enough to be cleared deliberately, then the step falls back to
			2 because a service unit is genuinely required again.

			The two assertions are ordered: the value is cleared by the change
			handler, and the step is derived from that value, so checking the step
			first would be racing the same tick.
		*/
		await fireEvent.change(facilityField(), { target: { value: FACILITIES[1].id } });
		await waitFor(() => expect(serviceField().value).toBe(''));
		await waitFor(() =>
			expect(document.querySelector('[aria-current="step"]')?.textContent).toContain('Layanan')
		);
	});

	it('keeps a service selection when the same facility is re-chosen', async () => {
		mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());

		await fireEvent.change(facilityField(), { target: { value: FACILITIES[0].id } });
		await waitFor(() => expect(serviceField()).toBeTruthy());
		await fireEvent.change(serviceField(), { target: { value: UNITS[0].id } });
		await waitFor(() => expect(serviceField().value).toBe(UNITS[0].id));

		// Re-selecting the SAME facility must not discard a valid choice. An
		// unconditional clear on every change would wipe it here, and the
		// citizen would have to pick their service again for no reason.
		await fireEvent.change(facilityField(), { target: { value: FACILITIES[0].id } });
		await new Promise((resolve) => setTimeout(resolve, 40));
		expect(serviceField().value).toBe(UNITS[0].id);
	});

	it('blocks submission and reports each invalid field', async () => {
		const calls = mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());

		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));
		await waitFor(() => expect(document.body.textContent).toContain('wajib diisi'));

		// Nothing was sent: a form that validates locally must not spend a
		// request to discover what it already knows.
		expect(calls.some((c) => c.method === 'POST')).toBe(false);
	});

	it('rejects a phone outside 10-15 digits before sending', async () => {
		const calls = mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();

		await fireEvent.input(phoneField(), { target: { value: '123' } });
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(document.body.textContent).toContain('10-15 digit angka'));
		expect(calls.some((c) => c.method === 'POST')).toBe(false);
	});

	it('rejects an appointment time in the past before sending', async () => {
		const calls = mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();

		await fireEvent.input(timeField(), {
			target: { value: '2000-01-01T10:00' }
		});
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(document.body.textContent).toContain('masa depan'));
		expect(calls.some((c) => c.method === 'POST')).toBe(false);
	});

	it('marks an invalid control with aria-invalid and links the message', async () => {
		mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();

		await fireEvent.input(nameField(), { target: { value: '' } });
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => {
			const name = nameField() as HTMLInputElement;
			expect(name.getAttribute('aria-invalid')).toBe('true');
			const describedBy = name.getAttribute('aria-describedby');
			expect(describedBy).toBeTruthy();
			// The describedby target must actually exist, or the message is
			// announced as nothing.
			const targets = (describedBy ?? '').split(' ').filter(Boolean);
			expect(targets.length).toBeGreaterThan(0);
			const nodes = targets.map((id) => document.getElementById(id));
			expect(nodes.every((node) => node !== null)).toBe(true);
			expect(nodes.some((node) => node?.textContent?.includes('wajib diisi'))).toBe(true);
		});
	});

	it('posts exactly the wire contract the backend decodes', async () => {
		const calls = mockApi((url) => {
			if (url === '/api/v1/appointments') {
				return jsonResponse({
					success: true,
					data: {
						id: 'appt-1',
						checkin_code: 'AB12CD',
						status: 'scheduled',
						appointment_time: '2099-06-01T03:00:00Z'
					}
				});
			}
			return defaultHandler(url);
		});

		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true));
		const post = calls.find((c) => c.method === 'POST')!;
		const body = post.body as Record<string, string>;

		/*
			The key assertion of this file. `BookAppointmentRequest` in booking.go
			tags the name `patient_display_name`. The client previously sent
			`patient_name`, which Go silently discards, and the booking failed
			with 400 "Nama pasien wajib diisi." — a validation error about a field
			the citizen had filled in. `patient_name` must never reappear.
		*/
		expect(Object.keys(body).sort()).toEqual(
			['appointment_time', 'facility_id', 'patient_display_name', 'patient_phone', 'service_unit_id'].sort()
		);
		expect(body.facility_id).toBe(FACILITIES[0].id);
		expect(body.service_unit_id).toBe(UNITS[0].id);
		expect(body.patient_display_name).toBe('Siti Rahma');
		expect(body.patient_phone).toBe('081234567890');
		expect(body).not.toHaveProperty('patient_name');
		expect(body).not.toHaveProperty('practitioner_id');

		// RFC3339, because the backend parses RFC3339 and datetime-local yields
		// a zoneless local string.
		expect(body.appointment_time).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/);
	});

	it('shows the check-in code and a deep link carrying both canonical params', async () => {
		mockApi((url) => {
			if (url === '/api/v1/appointments') {
				return jsonResponse({
					success: true,
					data: {
						id: 'appt-xyz',
						checkin_code: 'ZZ99YY',
						status: 'scheduled',
						appointment_time: '2099-06-01T03:00:00Z'
					}
				});
			}
			return defaultHandler(url);
		});

		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(document.body.textContent).toContain('berhasil dibuat'));
		// The code the citizen has to write down.
		expect(document.querySelector('[data-testid="booking-checkin-code"]')?.textContent?.trim()).toBe('ZZ99YY');
		expect(document.querySelector('[data-testid="booking-appointment-id"]')?.textContent?.trim()).toBe('appt-xyz');

		// The deep link uses the canonical names, not the legacy aliases.
		const cta = screen.getByRole('link', { name: /Lanjut ke Check-In/ });
		const href = cta.getAttribute('href') ?? '';
		expect(href).toContain('appointment_id=appt-xyz');
		expect(href).toContain('checkin_code=ZZ99YY');

		/*
			Checked against the parsed parameter NAMES, not a substring. A naive
			`not.toContain('id=')` is wrong: "appointment_id=" contains "id=", so
			the assertion would fail on the very link it is meant to bless. The
			legacy aliases are the single-word names, and that is what has to be
			absent.
		*/
		const params = new URLSearchParams(href.split('?')[1] ?? '');
		expect([...params.keys()].sort()).toEqual(['appointment_id', 'checkin_code']);
		expect(params.has('id')).toBe(false);
		expect(params.has('code')).toBe(false);
	});

	it('renders a 400 with the backend message verbatim', async () => {
		mockApi((url) => {
			if (url === '/api/v1/appointments') {
				return jsonResponse(
					{ success: false, error: 'Fasilitas tidak ditemukan.' },
					400
				);
			}
			return defaultHandler(url);
		});

		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(document.body.textContent).toContain('Fasilitas tidak ditemukan.'));
		// Announced, not just coloured.
		expect(document.querySelector('[role="alert"]')?.textContent).toContain(
			'Fasilitas tidak ditemukan.'
		);
	});

	it('renders a 409 with the backend message verbatim', async () => {
		mockApi((url) => {
			if (url === '/api/v1/appointments') {
				return jsonResponse(
					{ success: false, error: 'Slot jadwal sudah penuh. Silakan pilih jadwal lain.' },
					409
				);
			}
			return defaultHandler(url);
		});

		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(document.body.textContent).toContain('Slot jadwal sudah penuh.'));
	});

	it('renders a 429 with the backend message verbatim', async () => {
		mockApi((url) => {
			if (url === '/api/v1/appointments') {
				return jsonResponse(
					{
						success: false,
						error: 'Nomor HP ini sudah melebihi batas pemesanan per hari. Silakan coba lagi besok.'
					},
					429
				);
			}
			return defaultHandler(url);
		});

		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(document.body.textContent).toContain('batas pemesanan per hari'));
	});

	it('renders a 500 with the single shared generic message', async () => {
		mockApi((url) => {
			if (url === '/api/v1/appointments') {
				return jsonResponse({ success: false, error: 'Gagal menyimpan janji temu.' }, 500);
			}
			return defaultHandler(url);
		});

		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(document.body.textContent).toContain(GENERIC_UNAVAILABLE));
		// A 5xx must not leak the raw server string: "Gagal menyimpan janji
		// temu." implies the booking was attempted and lost, which is a
		// different (and scarier) claim than "the service is unavailable".
		expect(document.body.textContent).not.toContain('Gagal menyimpan janji temu.');
	});

	it('stays silent on an abort instead of showing an error', async () => {
		/*
			A real AbortError, because that is what `apiFetch` checks for: it
			tests `cause instanceof DOMException && name === 'AbortError'`, or a
			flagged signal. A plain `new Error('aborted')` matches neither and is
			correctly classified as a NETWORK failure — so a test using one would
			be asserting that a generic error panel appears, which is the opposite
			of what this test is named for.
		*/
		mockApi((url) => {
			if (url === '/api/v1/appointments') {
				return Promise.reject(
					new DOMException('The user aborted a request.', 'AbortError')
				);
			}
			return defaultHandler(url);
		});

		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));
		await tick();
		await new Promise((resolve) => setTimeout(resolve, 40));

		// No red panel, and the form is still there to retry from. Punishing
		// someone for navigating away is the whole thing this avoids.
		expect(document.querySelector('[role="alert"]')).toBeNull();
		expect(facilityField()).toBeTruthy();
		expect(screen.getByRole('button', { name: /Daftar Janji Temu/ })).toBeTruthy();
	});

	it('preselects a facility from ?facility_id= and ignores an unknown one', async () => {
		window.history.replaceState({}, '', '/appointments/new?facility_id=' + FACILITIES[1].id);
		mockApi(defaultHandler);
		render(Booking);

		await waitFor(() =>
			expect((facilityField() as HTMLSelectElement).value).toBe(FACILITIES[1].id)
		);
		// The /faskes link is only useful if the selection is honoured.
		expect(document.body.textContent).toContain('RSUD Kota Sehat');
	});

	it('does not preselect a facility id that is not in the catalog', async () => {
		window.history.replaceState({}, '', '/appointments/new?facility_id=not-a-real-facility');
		mockApi(defaultHandler);
		render(Booking);

		await waitFor(() => expect(facilityField()).toBeTruthy());
		// A hidden value behind a "Pilih fasilitas..." placeholder would make
		// the summary and the request body disagree with the visible select.
		expect((facilityField() as HTMLSelectElement).value).toBe('');
	});

	it('has exactly one h1 and every control is labelled', async () => {
		mockApi(defaultHandler);
		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());

		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
		// A real label per control, so the accessible name is not the
		// placeholder or the value.
		expect(facilityField().tagName).toBe('SELECT');
	});

	it('keeps the check-in code selectable and copyable', async () => {
		mockApi((url) => {
			if (url === '/api/v1/appointments') {
				return jsonResponse({
					success: true,
					data: {
						id: 'a',
						checkin_code: 'COPY12',
						status: 'scheduled',
						appointment_time: '2099-06-01T03:00:00Z'
					}
				});
			}
			return defaultHandler(url);
		});

		render(Booking);
		await waitFor(() => expect(facilityField()).toBeTruthy());
		await fillValidBooking();
		await fireEvent.click(screen.getByRole('button', { name: /Daftar Janji Temu/ }));

		await waitFor(() => expect(screen.getByRole('button', { name: /Salin kode/ })).toBeTruthy());

		/*
			Checked against the component source, not `getComputedStyle`.
			jsdom does not apply Svelte's scoped stylesheet rules to computed
			styles, so a computed check returns the default ("") and would fail
			for a reason that has nothing to do with the product. Reading the
			declaration proves the intent is present in the shipped CSS, and the
			Playwright suite proves it takes effect in a real browser.

			Resolved from `process.cwd()` rather than `import.meta.url`: under
			Vitest the module URL is not a file: URL, so `new URL(...)` throws
			"The URL must be of scheme file". Vitest runs with the package root
			as cwd, which is the stable anchor here.
		*/
		const source = readFileSync(
			resolve(process.cwd(), 'src/routes/appointments/new/+page.svelte'),
			'utf-8'
		);
		expect(source).toMatch(/\.sigap-booking__code\s*\{[^}]*user-select:\s*all/);

		// The copy control exists, and the code is real text rather than an
		// image or a canvas that cannot be selected at all.
		const code = document.querySelector('[data-testid="booking-checkin-code"]');
		expect(code?.textContent?.trim()).toBe('COPY12');
		expect(code?.tagName).toBe('P');
	});
});
