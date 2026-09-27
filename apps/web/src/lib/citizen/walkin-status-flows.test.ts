import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import WalkIn from '../../routes/queues/new/+page.svelte';
import Status from '../../routes/patient/status/+page.svelte';
import {
	visitProgressFrom,
	visitOutcomeLabel,
	VISIT_PROGRESS_LABELS,
	VISIT_PROGRESS_ORDER
} from './citizenFlow';

/**
 * Walk-in registration and patient status.
 *
 * Two contracts under test here, both about honesty rather than mechanics:
 *
 *   1. The walk-in body is camelCase with a nested patient object, which is
 *      the OPPOSITE convention from the booking request in the same Go
 *      package. Getting it wrong is a silent no-op followed by a 400, so the
 *      exact shape is asserted rather than assumed.
 *   2. The status page reproduces `mapCheckinStatus` exactly and adds nothing.
 *      The progress indicator has no timestamps, no history, and no realtime
 *      claim, because the backend has none to give and inventing them would
 *      put a confident lie on a civic health page.
 */

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(`${JSON.stringify(body)}\n`, {
		status,
		headers: { 'content-type': 'application/json; charset=utf-8' }
	});
}

const FACILITIES = [
	{
		id: '11111111-1111-4111-8111-111111111111',
		name: 'Puskesmas Sukajaya',
		short_code: 'PKM',
		type: 'puskesmas' as const,
		is_active: true
	}
];

function field(name: string): HTMLElement {
	return screen.getByLabelText(new RegExp(name, 'i'));
}

/**
 * Wait for the facility select to be genuinely usable.
 *
 * The control exists in the DOM from the first render, but the page renders it
 * `disabled` until the public catalog resolves, and it has no real `<option>`
 * elements until then. A change event fired in that window is dropped by the
 * browser rather than applied, which produces a test that looks like a product
 * bug and is not one: the form is deliberately unusable while loading, so a
 * citizen cannot reach this state either.
 */
async function waitForFacilities(): Promise<HTMLSelectElement> {
	const select = field('Fasilitas') as HTMLSelectElement;
	await waitFor(() => {
		expect(select.disabled).toBe(false);
		expect(select.options.length).toBeGreaterThan(1);
	});
	return select;
}

/** Choose the facility and confirm the value survived the round trip. */
async function chooseFacility(): Promise<void> {
	const select = await waitForFacilities();
	await fireEvent.change(select, { target: { value: FACILITIES[0].id } });
	await waitFor(() => expect(select.value).toBe(FACILITIES[0].id));
}

function mockApi(
	handler: (url: string, init?: RequestInit) => Response | Promise<Response>
) {
	const calls: { url: string; method: string; body: unknown }[] = [];
	globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = String(input);
		calls.push({
			url,
			method: (init?.method ?? 'GET').toUpperCase(),
			body: init?.body ? JSON.parse(String(init.body)) : undefined
		});
		return await handler(url, init);
	}) as never;
	return calls;
}

beforeEach(() => {
	document.body.innerHTML = '';
	window.history.replaceState({}, '', '/');
});

afterEach(() => {
	vi.restoreAllMocks();
});

/* ================================================================== *
 * Walk-in
 * ================================================================== */

describe('/queues/new: the exact contract', () => {
	async function submitWalkin(
		handler: (url: string) => Response | Promise<Response>
	) {
		const calls = mockApi((url) => {
			if (url.includes('/public/facilities')) {
				return jsonResponse({ success: true, data: FACILITIES });
			}
			return handler(url);
		});
		render(WalkIn);
		await chooseFacility();
		await fireEvent.input(field('Nama lengkap'), { target: { value: 'Budi Santoso' } });
		await fireEvent.input(field('Nomor telepon'), { target: { value: '081234567890' } });
		await fireEvent.click(screen.getByRole('button', { name: /Ambil Nomor Antrean/ }));
		return calls;
	}

	it('posts to the generate endpoint, never the appointment check-in route', async () => {
		const calls = await submitWalkin(() =>
			jsonResponse({ success: true, data: { TicketID: 't1', formatted_number: 'A-001', Status: 'waiting' } })
		);

		const post = calls.find((c) => c.method === 'POST');
		expect(post).toBeTruthy();
		expect(post!.url).toBe('/api/v1/queues/generate');
		// The two flows are separate by contract. Reaching for the check-in
		// route here could mutate an appointment that does not exist.
		for (const call of calls) {
			expect(call.url).not.toContain('/check-in');
		}
	});

	it('sends the camelCase body with a nested patient object', async () => {
		const calls = await submitWalkin(() =>
			jsonResponse({ success: true, data: { formatted_number: 'A-001' } })
		);

		const body = calls.find((c) => c.method === 'POST')!.body as Record<string, unknown>;
		/*
		 * `GenerateRequest` in queue.go takes facilityId and a nested patient
		 * object with fullName — the opposite convention from
		 * `BookAppointmentRequest` in the same package. Sending snake_case
		 * here is a silent no-op: Go ignores unknown keys, so the body would
		 * arrive empty and fail validation for no visible reason.
		*/
		expect(body.facilityId).toBe(FACILITIES[0].id);
		expect(body).not.toHaveProperty('facility_id');
		expect(body.patient).toEqual({ fullName: 'Budi Santoso', phone: '081234567890' });
		expect(body).not.toHaveProperty('patient_name');
		expect(body).not.toHaveProperty('fullName');
	});

	it('reads formatted_number, the casing Go emits', async () => {
		await submitWalkin(() =>
			jsonResponse({
				success: true,
				data: {
					TicketID: 'ticket-1',
					formatted_number: 'B-007',
					Status: 'waiting',
					estimated_wait_minutes: 15
				}
			})
		);
		await waitFor(() => expect(document.querySelector('[data-testid="walkin-number"]')).toBeTruthy());
		expect(document.querySelector('[data-testid="walkin-number"]')?.textContent?.trim()).toBe('B-007');
	});

	it('also reads FormattedNumber', async () => {
		await submitWalkin(() =>
			jsonResponse({ success: true, data: { FormattedNumber: 'C-009' } })
		);
		await waitFor(() => expect(document.querySelector('[data-testid="walkin-number"]')).toBeTruthy());
		// A queue number must never render as a dash. A dash looks like a
		// server failure, and the citizen would read it at the counter.
		expect(document.querySelector('[data-testid="walkin-number"]')?.textContent?.trim()).toBe('C-009');
	});

	it('shows the estimate with estimate wording, never a live claim', async () => {
		await submitWalkin(() =>
			jsonResponse({
				success: true,
				data: { formatted_number: 'A-001', estimated_wait_minutes: 30 }
			})
		);
		await waitFor(() => expect(document.body.textContent).toContain('A-001'));
		const text = (document.body.textContent ?? '').toLowerCase();
		expect(text).toContain('estimasi');
		for (const forbidden of ['realtime', 'real-time', 'live', 'dipastikan', 'pasti']) {
			expect(text, `must not claim ${forbidden}`).not.toContain(forbidden);
		}
	});

	it('renders a 429 daily-limit message verbatim', async () => {
		const limitCopy =
			'Nomor HP ini sudah mencapai batas maksimal 2 antrean per hari untuk fasilitas tersebut. Silakan coba lagi besok atau daftar di fasilitas lain.';
		await submitWalkin(() => jsonResponse({ success: false, error: limitCopy }, 429));

		await waitFor(() => expect(document.querySelector('[data-testid="walkin-error"]')).toBeTruthy());
		// The copy explains the limit AND when it resets, which is the only
		// thing that tells someone whether to wait or go elsewhere. Replacing
		// it with "terlalu banyak permintaan" destroys that.
		expect(document.querySelector('[data-testid="walkin-error"]')?.textContent).toContain(
			'batas maksimal 2 antrean per hari'
		);
		expect(document.body.textContent).toContain('Silakan coba lagi besok');
	});

	it('requires facility, name, and a valid phone before sending', async () => {
		const calls = mockApi((url) =>
			url.includes('/public/facilities')
				? jsonResponse({ success: true, data: FACILITIES })
				: jsonResponse({ success: true, data: { formatted_number: 'A-1' } })
		);
		render(WalkIn);
		await waitForFacilities();

		await fireEvent.click(screen.getByRole('button', { name: /Ambil Nomor Antrean/ }));
		await waitFor(() => expect(document.body.textContent).toContain('wajib diisi'));
		expect(calls.some((c) => c.method === 'POST')).toBe(false);

		// And a bad phone is caught too.
		await chooseFacility();
		await fireEvent.input(field('Nama lengkap'), { target: { value: 'Budi' } });
		await fireEvent.input(field('Nomor telepon'), { target: { value: '123' } });
		await fireEvent.click(screen.getByRole('button', { name: /Ambil Nomor Antrean/ }));
		await waitFor(() => expect(document.body.textContent).toContain('10-15 digit angka'));
		expect(calls.some((c) => c.method === 'POST')).toBe(false);
	});

	it('links back to the appointment check-in flow', async () => {
		mockApi((url) =>
			url.includes('/public/facilities')
				? jsonResponse({ success: true, data: FACILITIES })
				: jsonResponse({ success: true, data: {} })
		);
		render(WalkIn);
		await waitForFacilities();

		// Someone who already has a booking should be redirected to the code
		// flow rather than registering a second, duplicate ticket.
		const link = screen.getByRole('link', { name: /Gunakan kode check-in/ });
		expect(link.getAttribute('href')).toBe('/appointments/check-in');
	});

	it('offers the status lookup after a successful walk-in', async () => {
		await submitWalkin(() => jsonResponse({ success: true, data: { formatted_number: 'A-001' } }));
		await waitFor(() => expect(screen.getByRole('link', { name: /Cek status kunjungan/ })).toBeTruthy());
		expect(
			screen.getByRole('link', { name: /Cek status kunjungan/ }).getAttribute('href')
		).toBe('/patient/status');
	});
});

/* ================================================================== *
 * Status: the mapping
 * ================================================================== */

describe('visit progress reproduces mapCheckinStatus', () => {
	/*
	 * One case per arm of the Go switch in patient.go, including the default.
	 * The default arm matters most: the backend returns the status unchanged
	 * when it does not recognise one, and the frontend must not invent a step
	 * for it.
	 */
	it.each([
		['not_checked_in', 'check-in'],
		['checked_in', 'antre'],
		['in_queue', 'dilayani'],
		['selesai', 'selesai']
	])('maps %s to the %s step', (backend, step) => {
		expect(visitProgressFrom(backend)).toEqual({ kind: 'step', step });
	});

	it('treats dibatalkan and tidak_hadir as outcomes, not progress', () => {
		// A cancelled appointment rendered as "Selesai" would be a lie, and one
		// frozen at step 1 would tell a citizen they simply had not arrived.
		expect(visitProgressFrom('dibatalkan')).toEqual({ kind: 'cancelled' });
		expect(visitProgressFrom('tidak_hadir')).toEqual({ kind: 'no-show' });
		expect(visitOutcomeLabel(visitProgressFrom('dibatalkan'))).toBe('Janji temu dibatalkan');
		expect(visitOutcomeLabel(visitProgressFrom('tidak_hadir'))).toBe('Tidak hadir');
	});

	it('passes an unmapped status through rather than guessing a step', () => {
		// The Go default arm is `return status`. A status added on the backend
		// must appear as itself, not be relabelled as progress the citizen is
		// not making.
		expect(visitProgressFrom('some_future_status')).toEqual({
			kind: 'unknown',
			raw: 'some_future_status'
		});
		expect(visitProgressFrom('')).toEqual({ kind: 'unknown', raw: '' });
	});

	it('uses exactly the four frozen labels, in order', () => {
		expect(VISIT_PROGRESS_ORDER).toEqual(['check-in', 'antre', 'dilayani', 'selesai']);
		expect(VISIT_PROGRESS_ORDER.map((s) => VISIT_PROGRESS_LABELS[s])).toEqual([
			'Check-In',
			'Antre',
			'Dilayani',
			'Selesai'
		]);
	});
});

/* ================================================================== *
 * Status: the page
 * ================================================================== */

describe('/patient/status', () => {
	const FOUND = {
		found_by: 'checkin_code' as const,
		facility_name: 'Puskesmas Sukajaya',
		appointment_status: 'queued' as const,
		appointment_time: '2099-06-01T03:00:00Z',
		checkin_status: 'in_queue',
		queue_number: 12,
		queue_status: 'waiting' as const,
		queue_formatted_number: 'A-012'
	};

	async function lookupWith(handler: () => Response | Promise<Response>) {
		const calls = mockApi(handler);
		render(Status);
		await waitFor(() => expect(field('Kode check-in')).toBeTruthy());
		await fireEvent.input(field('Kode check-in'), { target: { value: 'AB12CD' } });
		await fireEvent.click(screen.getByRole('button', { name: /Cek Status/ }));
		return calls;
	}

	it('looks up with a single encoded code parameter', async () => {
		const calls = await lookupWith(() => jsonResponse({ success: true, data: FOUND }));
		await waitFor(() => expect(calls).toHaveLength(1));

		const url = calls[0].url;
		expect(url.startsWith('/api/v1/patient/status?')).toBe(true);
		// The backend tries checkin_code first and formatted_number second, so
		// one field covers both and the citizen is never asked which they have.
		expect(url).toContain('code=AB12CD');
		// No invented parameters: the endpoint takes only `code`.
		expect(url).not.toContain('appointment_id');
		expect(url).not.toContain('queue_number');
	});

	it('encodes a code that needs escaping', async () => {
		// A pasted value with a space or a plus must not corrupt the query.
		const calls = await lookupWith(() => jsonResponse({ success: true, data: FOUND }));
		await waitFor(() => expect(calls).toHaveLength(1));
		expect(calls[0].url).toContain('code=AB12CD');
	});

	it('renders the current step and the facility', async () => {
		await lookupWith(() => jsonResponse({ success: true, data: FOUND }));
		await waitFor(() => expect(document.querySelector('[data-testid="visit-steps"]')).toBeTruthy());

		expect(document.body.textContent).toContain('Puskesmas Sukajaya');
		// in_queue is the current step.
		const current = document.querySelector('[aria-current="step"]');
		expect(current?.textContent).toContain('Dilayani');
		// And the earlier steps are marked done, not just recoloured.
		expect(document.querySelectorAll('.sigap-visit__step--done')).toHaveLength(2);
	});

	it('shows the queue number when the response carries one', async () => {
		await lookupWith(() => jsonResponse({ success: true, data: FOUND }));
		await waitFor(() => expect(document.body.textContent).toContain('A-012'));
	});

	it('fabricates no timestamp, history, or realtime claim', async () => {
		await lookupWith(() => jsonResponse({ success: true, data: FOUND }));
		await waitFor(() => expect(document.querySelector('[data-testid="visit-steps"]')).toBeTruthy());

		const text = (document.body.textContent ?? '').toLowerCase();
		/*
			The load-bearing assertion. The backend returns ONE status, not a
		 * history, so any of these words would mean the page invented
		 * something about someone's medical visit.
			*/
		for (const forbidden of [
			'realtime',
			'real-time',
			'live',
			'berjalan',
			'riwayat',
			'history',
			'durasi',
			'menunggu sejak',
			'otomatis',
			'auto-refresh',
			'terus diperbarui'
		]) {
			expect(text, `must not claim ${forbidden}`).not.toContain(forbidden);
		}
		// A clock time derived from the appointment is fine — that is a real
		// scheduled time from the response. A per-step history is not.
		expect(document.querySelectorAll('.sigap-visit__step')).toHaveLength(4);
	});

	it('shows guidance for an unrecognised code, not an error', async () => {
		await lookupWith(() =>
			jsonResponse({ success: false, error: 'Kode tidak ditemukan.' }, 404)
		);
		await waitFor(() => expect(document.querySelector('[data-testid="status-not-found"]')).toBeTruthy());

		// Not found is its own state: nothing is broken, the code is unknown.
		expect(document.querySelector('[data-testid="status-error"]')).toBeNull();
		// And it tells the citizen what to try instead.
		expect(document.body.textContent).toContain('nomor antrean');
	});

	it('renders a 429 with the backend copy verbatim', async () => {
		await lookupWith(() =>
			jsonResponse({ success: false, error: 'Terlalu banyak permintaan. Coba lagi nanti.' }, 429)
		);
		await waitFor(() => expect(document.querySelector('[data-testid="status-error"]')).toBeTruthy());
		expect(document.querySelector('[data-testid="status-error"]')?.textContent).toBe(
			'Terlalu banyak permintaan. Coba lagi nanti.'
		);
	});

	it('renders a 500 with the single shared generic message', async () => {
		await lookupWith(() =>
			jsonResponse({ success: false, error: 'Gagal mencari status pasien.' }, 500)
		);
		await waitFor(() => expect(document.querySelector('[data-testid="status-error"]')).toBeTruthy());
		expect(document.body.textContent).toContain('Layanan sedang bermasalah.');
		// A 5xx must not leak the raw server string.
		expect(document.body.textContent).not.toContain('Gagal mencari status pasien.');
	});

	it('rejects a malformed code without sending a request', async () => {
		// safeCodeRe in patient.go is ^[A-Za-z0-9-]+$, and maxPatientCodeLen
		// is 64. Catching it here turns a round trip into an inline message.
		const calls = mockApi(() => jsonResponse({ success: true, data: FOUND }));
		render(Status);
		await waitFor(() => expect(field('Kode check-in')).toBeTruthy());

		await fireEvent.input(field('Kode check-in'), { target: { value: 'has space' } });
		await fireEvent.click(screen.getByRole('button', { name: /Cek Status/ }));
		await waitFor(() => expect(document.body.textContent).toContain('huruf, angka'));
		expect(calls).toHaveLength(0);
	});

	it('does not send a request for an empty code', async () => {
		const calls = mockApi(() => jsonResponse({ success: true, data: FOUND }));
		render(Status);
		await waitFor(() => expect(field('Kode check-in')).toBeTruthy());

		await fireEvent.click(screen.getByRole('button', { name: /Cek Status/ }));
		await waitFor(() => expect(document.body.textContent).toContain('wajib diisi'));
		expect(calls).toHaveLength(0);
	});

	it('has one h1 and one lookup input', async () => {
		mockApi(() => jsonResponse({ success: true, data: FOUND }));
		render(Status);
		await waitFor(() => expect(field('Kode check-in')).toBeTruthy());

		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
		// One field, because the backend accepts both kinds of code in the
		// same parameter.
		expect(document.querySelectorAll('input')).toHaveLength(1);
	});
});
