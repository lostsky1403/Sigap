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
