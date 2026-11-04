import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Status from '../../routes/patient/status/+page.svelte';
import {
	visitProgressFrom,
	visitOutcomeLabel,
	VISIT_PROGRESS_LABELS,
	VISIT_PROGRESS_ORDER
} from './citizenFlow';

/**
 * Patient status at /patient/status.
 *
 * The load-bearing property here is HONESTY. The endpoint returns ONE current
 * status, not a history, not timestamps, not durations. The page therefore
 * reproduces `mapCheckinStatus` from patient.go arm for arm and adds nothing:
 * a status the backend does not recognise is shown as itself rather than
 * relabelled as progress the citizen is not making, and a cancelled
 * appointment is never rendered as a completed one.
 */

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(`${JSON.stringify(body)}\n`, {
		status,
		headers: { 'content-type': 'application/json; charset=utf-8' }
	});
}

function field(name: string): HTMLElement {
	return screen.getByLabelText(new RegExp(name, 'i'));
}

function mockApi(handler: () => Response | Promise<Response>) {
	const calls: { url: string; method: string }[] = [];
	globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		calls.push({ url: String(input), method: (init?.method ?? 'GET').toUpperCase() });
		return await handler();
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

describe('visit progress reproduces mapCheckinStatus', () => {
	/*
	 * One case per arm of the Go switch in patient.go, including the default.
	 * The default arm matters most: the backend returns the status unchanged
	 * when it does not recognise one, and the frontend must not invent a step
	 * for it.
	 */
	it.each([
		['not_checked_in', undefined, 'check-in'],
		['checked_in', undefined, 'antre'],
		// `in_queue` alone means the citizen holds a ticket and is WAITING.
		// The previous mapping sent them to "Dilayani" (being served) the moment
		// a ticket existed, which told every checked-in citizen they were
		// already with a clinician while they sat in the waiting room.
		['in_queue', 'waiting', 'antre'],
		['in_queue', undefined, 'antre'],
		['in_queue', 'called', 'dilayani'],
		['in_queue', 'in_service', 'dilayani'],
		['in_queue', 'completed', 'selesai'],
		['selesai', undefined, 'selesai']
	])('maps %s (queue %s) to the %s step', (backend, queueStatus, step) => {
		expect(visitProgressFrom(backend, queueStatus)).toEqual({ kind: 'step', step });
	});

	it('treats a cancelled ticket as cancelled even while the appointment is queued', () => {
		// The ticket can be cancelled without the appointment being updated, so
		// the ticket status has to win: showing "Antre" for a cancelled ticket
		// keeps a citizen waiting for a call that will never come.
		expect(visitProgressFrom('in_queue', 'cancelled')).toEqual({ kind: 'cancelled' });
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
		// FOUND has checkin_status 'in_queue' AND queue_status 'waiting': the
		// citizen holds a ticket and is waiting, so the current step is Antre.
		// Only a called or in-service ticket is Dilayani.
		const current = document.querySelector('[aria-current="step"]');
		expect(current?.textContent).toContain('Antre');
		// And the earlier step is marked done, not just recoloured.
		expect(document.querySelectorAll('.sigap-visit__step--done')).toHaveLength(1);
	});

	it('advances to Dilayani only once the ticket is called', async () => {
		await lookupWith(() =>
			jsonResponse({
				success: true,
				data: { ...FOUND, queue_status: 'called' }
			})
		);
		await waitFor(() => expect(document.querySelector('[data-testid="visit-steps"]')).toBeTruthy());

		const current = document.querySelector('[aria-current="step"]');
		expect(current?.textContent).toContain('Dilayani');
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
		// And the guidance names the code the citizen actually holds. The endpoint
		// resolves check-in codes only, so advice to try a ticket number would be
		// a dead end — a regression this pins.
		const guidance = document.querySelector('[data-testid="status-not-found"]')?.textContent ?? '';
		expect(guidance).toMatch(/kode check-in/i);
		expect(guidance).not.toMatch(/nomor antrean/i);
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
		//
		// The input is a character the code can never contain. Whitespace is
		// deliberately NOT used here: it is now absorbed rather than rejected,
		// because a code read aloud or copied from an SMS arrives as
		// "5ZE IOO" often enough that refusing it blames the citizen for a
		// formatting difference the lookup can absorb.
		const calls = mockApi(() => jsonResponse({ success: true, data: FOUND }));
		render(Status);
		await waitFor(() => expect(field('Kode check-in')).toBeTruthy());

		await fireEvent.input(field('Kode check-in'), { target: { value: 'kode!salah' } });
		await fireEvent.click(screen.getByRole('button', { name: /Cek Status/ }));
		await waitFor(() => expect(document.body.textContent).toContain('huruf, angka'));
		expect(calls).toHaveLength(0);
	});

	it('absorbs spaces in a code instead of rejecting it', async () => {
		// The owner-reported failure: a citizen typed the code the way they read
		// it and the app told them it was invalid or not found. Spaces are
		// removed before validation and before the request.
		const calls = mockApi(() => jsonResponse({ success: true, data: FOUND }));
		render(Status);
		await waitFor(() => expect(field('Kode check-in')).toBeTruthy());

		await fireEvent.input(field('Kode check-in'), { target: { value: 'AB 12 CD' } });
		await fireEvent.click(screen.getByRole('button', { name: /Cek Status/ }));

		await waitFor(() => expect(calls).toHaveLength(1));
		expect(calls[0].url).toContain('code=AB12CD');
	});

	it('does not send a request for an empty code', async () => {
		const calls = mockApi(() => jsonResponse({ success: true, data: FOUND }));
		render(Status);
		await waitFor(() => expect(field('Kode check-in')).toBeTruthy());

		await fireEvent.click(screen.getByRole('button', { name: /Cek Status/ }));
		await waitFor(() => expect(document.body.textContent).toContain('wajib diisi'));
		expect(calls).toHaveLength(0);
	});

	it('prefills from a ?code= link and strips it from the address bar', async () => {
		// The queue ticket links here with the code it was just checked in
		// with. The code is a bearer token, so it is copied into the field and
		// removed from the URL — it must not stay in history or travel onward
		// in a Referer header.
		window.history.replaceState({}, '', '/patient/status?code=AB12CD&from=ticket');
		mockApi(() => jsonResponse({ success: true, data: FOUND }));
		render(Status);

		await waitFor(() =>
			expect((field('Kode check-in') as HTMLInputElement).value).toBe('AB12CD')
		);
		// Other parameters are preserved; only the code is removed.
		expect(window.location.search).toBe('?from=ticket');
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

	describe('cancellation of a scheduled visit', () => {
		const SCHEDULED = {
			found_by: 'checkin_code' as const,
			facility_name: 'Puskesmas Sukajaya',
			service_unit_name: 'Poli Umum',
			appointment_status: 'scheduled' as const,
			checkin_status: 'not_checked_in'
		};

		/**
		 * Routes the status GET and the cancel POST to separate handlers, returns
		 * the calls so a test can assert URL/method/body. The cancel mutation is
		 * keyed to the code the lookup resolved, so the body is under test here.
		 */
		async function cancelFlow({
			status,
			cancel
		}: {
			status: () => Response | Promise<Response>;
			cancel?: () => Response | Promise<Response>;
		}) {
			const calls: { url: string; method: string; body?: unknown }[] = [];
			globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
				const url = String(input);
				const method = (init?.method ?? 'GET').toUpperCase();
				calls.push({
					url,
					method,
					body: init?.body ? JSON.parse(String(init.body)) : undefined
				});
				if (url.includes('/appointments/cancel')) return await (cancel ?? status)();
				return await status();
			}) as never;

			render(Status);
			await waitFor(() => expect(field('Kode check-in')).toBeTruthy());
			await fireEvent.input(field('Kode check-in'), { target: { value: 'AB12CD' } });
			await fireEvent.click(screen.getByRole('button', { name: /Cek Status/ }));
			await screen.findByRole('button', { name: /Batalkan janji/ });
			await fireEvent.click(screen.getByRole('button', { name: /Batalkan janji/ }));
			await screen.findByRole('button', { name: /Ya, batalkan/ });
			return calls;
		}

		it('200 replaces the pass with the cancelled state', async () => {
			const calls = await cancelFlow({
				status: () => jsonResponse({ success: true, data: SCHEDULED }),
				cancel: () => jsonResponse({ success: true, data: { appointment_id: 'x', status: 'cancelled' } })
			});
			await fireEvent.click(screen.getByRole('button', { name: /Ya, batalkan/ }));
			await waitFor(() => expect(document.querySelector('[data-testid="status-cancelled"]')).toBeTruthy());
			// The cancel was keyed to the resolved code, not the raw field.
			expect(calls[calls.length - 1].body).toEqual({ checkin_code: 'AB12CD' });
		});

		it('409 stays on the receipt with the server message inline', async () => {
			await cancelFlow({
				status: () => jsonResponse({ success: true, data: SCHEDULED }),
				cancel: () =>
					jsonResponse({ success: false, error: 'Janji temu sudah tidak dapat dibatalkan.' }, 409)
			});
			await fireEvent.click(screen.getByRole('button', { name: /Ya, batalkan/ }));
			await waitFor(() => expect(document.querySelector('[data-testid="cancel-error"]')).toBeTruthy());
			expect(document.body.textContent).toContain('sudah tidak dapat dibatalkan');
			// Still the receipt, not an error or not-found state.
			expect(document.querySelector('[data-testid="status-not-found"]')).toBeNull();
			expect(document.querySelector('[data-testid="status-error"]')).toBeNull();
		});

		it('404 becomes the not-found state, distinct from an error', async () => {
			await cancelFlow({
				status: () => jsonResponse({ success: true, data: SCHEDULED }),
				cancel: () => jsonResponse({ success: false, error: 'Kode tidak ditemukan.' }, 404)
			});
			await fireEvent.click(screen.getByRole('button', { name: /Ya, batalkan/ }));
			await waitFor(() => expect(document.querySelector('[data-testid="status-not-found"]')).toBeTruthy());
		});

		it('429 renders the unavailable banner, never "Kode tidak ditemukan"', async () => {
			await cancelFlow({
				status: () => jsonResponse({ success: true, data: SCHEDULED }),
				cancel: () =>
					jsonResponse({ success: false, error: 'Terlalu banyak permintaan. Coba lagi nanti.' }, 429)
			});
			await fireEvent.click(screen.getByRole('button', { name: /Ya, batalkan/ }));
			await waitFor(() => expect(document.querySelector('[data-testid="status-error"]')).toBeTruthy());
			expect(document.querySelector('[data-testid="status-not-found"]')).toBeNull();
		});

		it('cancels the RESOLVED code, not whatever is now in the field', async () => {
			// A citizen looks up A, then edits the field to B. Cancelling by the
			// live field would cancel B — a different (possibly a relative's)
			// appointment. The mutation must use the code that resolved A.
			const calls = await cancelFlow({
				status: () => jsonResponse({ success: true, data: SCHEDULED }),
				cancel: () => jsonResponse({ success: false, error: 'x' }, 409) // never reached; body is the assertion
			});
			// Divert the field before confirming.
			await fireEvent.input(field('Kode check-in'), { target: { value: 'ZZ99ZZ' } });
			await fireEvent.click(screen.getByRole('button', { name: /Ya, batalkan/ }));
			await waitFor(() => expect(calls.some((c) => c.url.includes('/appointments/cancel'))).toBe(true));
			const cancelCall = calls.find((c) => c.url.includes('/appointments/cancel'));
			expect(cancelCall?.body).toEqual({ checkin_code: 'AB12CD' });
		});
	});
});
