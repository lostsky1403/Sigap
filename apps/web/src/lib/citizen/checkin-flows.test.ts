import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import CheckIn from '../../routes/appointments/check-in/+page.svelte';

/**
 * /appointments/check-in.
 *
 * The centre of gravity here is the 401 rule. On this public route a 401 does
 * not mean "you are signed out" — the backend answers 401 "Kode check-in tidak
 * cocok." when the submitted code does not match. So a page that treats
 * `unauthorized` generically shows a sign-in prompt to a citizen who typed
 * their code wrong, and there is no account that would help them. Several
 * tests below assert the ABSENCE of auth-flavoured language for exactly that
 * reason: a test that only checks the positive case passes even when the
 * generic handler is still in the file, one branch away from being triggered.
 *
 * The legacy deep-link aliases are tested too. Links generated before this
 * phase are already in people's messages, and a citizen holding one of those
 * plus a valid code must still be able to check in.
 */

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(`${JSON.stringify(body)}\n`, {
		status,
		headers: { 'content-type': 'application/json; charset=utf-8' }
	});
}

const TICKET = {
	appointment_id: 'appt-1',
	queue_ticket_id: 'ticket-1',
	formatted_number: 'A-012',
	status: 'queued' as const,
	estimated_wait_minutes: 25,
	processing_time: '45μs'
};

const APPT_ID = '11111111-1111-4111-8111-111111111111';
const CODE = 'AB12CD';

/** Records the request so the exact endpoint and body can be asserted. */
function mockCheckin(handler: () => Response | Promise<Response>) {
	const calls: { url: string; method: string; body: unknown }[] = [];
	globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
		calls.push({
			url: String(input),
			method: (init?.method ?? 'GET').toUpperCase(),
			body: init?.body ? JSON.parse(String(init.body)) : undefined
		});
		return await handler();
	}) as never;
	return calls;
}

function field(name: string): HTMLInputElement {
	return screen.getByLabelText(new RegExp(name, 'i')) as HTMLInputElement;
}

beforeEach(() => {
	document.body.innerHTML = '';
	window.history.replaceState({}, '', '/appointments/check-in');
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe('/appointments/check-in: deep links', () => {
	it('prefills from the canonical appointment_id and checkin_code', async () => {
		window.history.replaceState(
			{},
			'',
			`/appointments/check-in?appointment_id=${APPT_ID}&checkin_code=${CODE}`
		);
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);

		await waitFor(() => expect(field('ID janji temu').value).toBe(APPT_ID));
		expect(field('Kode check-in').value).toBe(CODE);
	});

	it('still accepts the legacy id and code aliases', async () => {
		// Links made before this phase are already in people's messages. A
		// citizen holding one plus a valid code must still get in.
		window.history.replaceState(
			{},
			'',
			`/appointments/check-in?id=${APPT_ID}&code=${CODE}`
		);
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);

		await waitFor(() => expect(field('ID janji temu').value).toBe(APPT_ID));
		expect(field('Kode check-in').value).toBe(CODE);
	});

	it('prefers the canonical name when a link carries both spellings', async () => {
		window.history.replaceState(
			{},
			'',
			`/appointments/check-in?appointment_id=canonical&checkin_code=NEW&id=legacy&code=OLD`
		);
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);

		// Canonical wins, so a link can be upgraded by renaming the parameter
		// without the old value silently taking over.
		await waitFor(() => expect(field('ID janji temu').value).toBe('canonical'));
		expect(field('Kode check-in').value).toBe('NEW');
	});

	it('starts empty when the link carries nothing', async () => {
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);

		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());
		expect(field('ID janji temu').value).toBe('');
		expect(field('Kode check-in').value).toBe('');
	});
});

describe('/appointments/check-in: submission', () => {
	it('posts to the appointment check-in route with only the code', async () => {
		const calls = mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: CODE } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));

		await waitFor(() => expect(calls).toHaveLength(1));
		expect(calls[0].url).toBe(`/api/v1/appointments/${APPT_ID}/check-in`);
		expect(calls[0].method).toBe('POST');
		// The id travels in the path, so the body is the code alone.
		expect(calls[0].body).toEqual({ checkin_code: CODE });
	});

	it('blocks an empty submit without sending anything', async () => {
		const calls = mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));
		await waitFor(() => expect(document.body.textContent).toContain('wajib diisi'));
		expect(calls).toHaveLength(0);
	});

	it('shows the queue number, the ticket ids, and an estimate', async () => {
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: CODE } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));

		await waitFor(() => expect(document.querySelector('[data-testid="queue-number"]')).toBeTruthy());
		// The number is the payload and it is unmissable.
		expect(document.querySelector('[data-testid="queue-number"]')?.textContent?.trim()).toBe('A-012');
		// The response fields build-verification.test.js requires.
		expect(document.querySelector('[data-testid="ticket-appointment-id"]')?.textContent?.trim()).toBe('appt-1');
		expect(document.querySelector('[data-testid="ticket-queue-id"]')?.textContent?.trim()).toBe('ticket-1');
		expect(document.querySelector('[data-testid="ticket-formatted-number"]')?.textContent?.trim()).toBe('A-012');
	});

	it('words the wait as an estimate and never as a live figure', async () => {
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: CODE } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));

		await waitFor(() => expect(document.body.textContent).toContain('A-012'));
		const text = (document.body.textContent ?? '').toLowerCase();
		expect(text).toContain('estimasi');
		expect(text).toContain('bisa berubah');
		// The words that would turn a registration-time estimate into a promise.
		for (const forbidden of ['realtime', 'real-time', 'live', 'dipastikan', 'pasti']) {
			expect(text, `must not claim ${forbidden}`).not.toContain(forbidden);
		}
	});

	it('offers the status lookup and a walk-in cross-link', async () => {
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		// Walk-in is a different flow, offered but not conflated.
		const walkin = screen.getByRole('link', { name: /Ambil antrean walk-in/ });
		expect(walkin.getAttribute('href')).toBe('/queues/new');

		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: CODE } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));

		await waitFor(() => expect(screen.getByRole('link', { name: /Cek status kunjungan/ })).toBeTruthy());
		expect(
			screen.getByRole('link', { name: /Cek status kunjungan/ }).getAttribute('href')
		).toBe('/patient/status');
	});
});

/* ================================================================== *
 * The 401 rule
 * ================================================================== */

describe('/appointments/check-in: 401 means a wrong code', () => {
	const wrongCode = () =>
		jsonResponse({ success: false, error: 'Kode check-in tidak cocok.' }, 401);

	async function submitWithWrongCode() {
		const calls = mockCheckin(wrongCode);
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());
		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: 'WRONG1' } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));
		await waitFor(() => expect(document.querySelector('[data-testid="checkin-error"]')).toBeTruthy());
		return calls;
	}

	it('says the code does not match', async () => {
		await submitWithWrongCode();
		expect(document.body.textContent).toContain('Kode check-in tidak cocok.');
	});

	it('never renders any auth-flavoured wording', async () => {
		await submitWithWrongCode();
		const text = (document.body.textContent ?? '').toLowerCase();
		/*
			The load-bearing assertion of this phase. Every word here is advice
			this page must never give: there is no session on this route, no
			account that would help, and sending someone to a sign-in page to fix
			a typo costs them the page that can actually help.

			Matched on word boundaries. A bare substring test for "masuk" also
			hits "masukkan" in the page's own subtitle ("Masukkan kode check-in"),
			which is exactly the kind of false positive that trains people to
			stop reading a test's failure message.
		*/
		const forbidden = [
			'login',
			'log in',
			'sign in',
			'sesi',
			'session',
			'kedaluwarsa',
			'expired',
			'unauthenticated',
			'tidak terautentikasi',
			// A sign-in prompt, not the word "masukkan".
			'\bmasuk\b',
			'\bmasukkan\b',
			'\bdaftar\b'
		];
		for (const word of forbidden) {
			expect(text, `401 must never mention "${word}"`).not.toMatch(new RegExp(word));
		}
		// And no sign-in control is offered at all.
		expect(document.querySelector('a[href*="login"], a[href*="auth"]')).toBeNull();
		expect(screen.queryByRole('button', { name: /Masuk|Log in|Sign in/ })).toBeNull();
		// The placeholders are the only "Masukkan"-shaped text, and they belong
		// to the inputs rather than to an error.
		expect(document.querySelector('[data-testid="checkin-error"]')?.textContent?.toLowerCase()).not.toMatch(
			/\bmasuk\b/
		);
	});

	it('announces the failure to assistive technology', async () => {
		await submitWithWrongCode();
		// role="alert" so the message is read out when it appears; otherwise a
		// screen reader user gets no confirmation that anything happened.
		const alert = document.querySelector('[data-testid="checkin-error"]');
		expect(alert?.getAttribute('role')).toBe('alert');
	});

	it('tells the citizen what to do next', async () => {
		await submitWithWrongCode();
		// "Retype the code" is a different instruction from "try again later",
		// and this is the case that needs the first one.
		expect(document.body.textContent).toMatch(/Periksa kembali kode/i);
	});

	it('keeps the form usable so the code can be corrected', async () => {
		/*
			A stateful mock, because a stateless one cannot model "the citizen
			retypes the code and it now works". Returning 401 forever would make
			this test assert the opposite of what it is named for.
		*/
		let attempts = 0;
		const calls = mockCheckin(() => {
			attempts += 1;
			return attempts === 1
				? jsonResponse({ success: false, error: 'Kode check-in tidak cocok.' }, 401)
				: jsonResponse({ success: true, data: TICKET });
		});
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: 'WRONG1' } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));
		await waitFor(() => expect(document.querySelector('[data-testid="checkin-error"]')).toBeTruthy());

		// A wrong code is a typo, and the fix is to type again. The form must
		// still be there with the appointment id intact — a wrong code is not a
		// reason to make someone find their booking again.
		expect(field('ID janji temu').value).toBe(APPT_ID);

		await fireEvent.input(field('Kode check-in'), { target: { value: CODE } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));

		await waitFor(() => expect(document.querySelector('[data-testid="queue-number"]')).toBeTruthy());
		// The error is gone, replaced by the ticket.
		expect(document.querySelector('[data-testid="checkin-error"]')).toBeNull();
		expect(calls[1].body).toEqual({ checkin_code: CODE });
	});
});

describe('/appointments/check-in: other server states', () => {
	async function submitExpectingError(
		body: unknown,
		status: number,
		query: string
	) {
		mockCheckin(() => jsonResponse(body, status));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());
		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: CODE } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));
		await waitFor(() => expect(document.querySelector('[data-testid="checkin-error"]')).toBeTruthy());
		expect(document.body.textContent).toContain(query);
	}

	it('renders a 404 with the backend message verbatim', async () => {
		await submitExpectingError(
			{ success: false, error: 'Janji temu tidak ditemukan.' },
			404,
			'Janji temu tidak ditemukan.'
		);
	});

	it('renders a 409 with the backend message verbatim', async () => {
		// The backend names the actual status, which is the useful part.
		await submitExpectingError(
			{ success: false, error: 'Janji temu tidak dapat check-in dengan status: queued' },
			409,
			'status: queued'
		);
	});

	it('renders a 429 with the backend message verbatim', async () => {
		await submitExpectingError(
			{ success: false, error: 'Terlalu banyak percobaan. Coba lagi nanti.' },
			429,
			'Terlalu banyak percobaan.'
		);
	});

	it('distinguishes 404, 409, and 429 from a wrong code', async () => {
		// Each is a different problem with a different fix, so none of them may
		// borrow the wrong-code copy.
		for (const [status, message] of [
			[404, 'Janji temu tidak ditemukan.'],
			[409, 'Janji temu tidak dapat check-in dengan status: completed'],
			[429, 'Terlalu banyak percobaan. Coba lagi nanti.']
		] as const) {
			document.body.innerHTML = '';
			await submitExpectingError({ success: false, error: message }, status, message);
			expect(document.body.textContent).not.toContain('Kode check-in tidak cocok.');
		}
	});

	it('renders a 500 with the single shared generic message', async () => {
		await submitExpectingError(
			{ success: false, error: 'Gagal memperbarui status check-in.' },
			500,
			'Layanan sedang bermasalah.'
		);
		// A 5xx must not leak the raw string.
		expect(document.body.textContent).not.toContain('Gagal memperbarui status check-in.');
	});

	it('stays silent on an abort', async () => {
		// A real AbortError, because that is what apiFetch checks for.
		mockCheckin(() =>
			Promise.reject(new DOMException('aborted', 'AbortError'))
		);
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());
		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: CODE } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));
		await new Promise((resolve) => setTimeout(resolve, 40));

		expect(document.querySelector('[data-testid="checkin-error"]')).toBeNull();
		expect(field('ID janji temu').value).toBe(APPT_ID);
	});
});

describe('/appointments/check-in: structure', () => {
	it('has exactly one h1 and a named landmark', async () => {
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
		// The walk-in alternative is a nav with a name, so it is reachable as a
		// landmark rather than being an unlabelled link in a corner.
		expect(screen.getByRole('navigation', { name: /Alternatif check-in/ })).toBeTruthy();
	});

	it('marks an invalid control and links the message to it', async () => {
		mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));
		await waitFor(() => {
			const code = field('Kode check-in');
			expect(code.getAttribute('aria-invalid')).toBe('true');
			const describedBy = code.getAttribute('aria-describedby');
			expect(describedBy).toBeTruthy();
			const nodes = (describedBy ?? '')
				.split(' ')
				.filter(Boolean)
				.map((id) => document.getElementById(id));
			// The referenced node must exist, or the message reads as nothing.
			expect(nodes.length).toBeGreaterThan(0);
			expect(nodes.every((node) => node !== null)).toBe(true);
		});
	});

	it('never calls the walk-in generation endpoint', async () => {
		// The two flows are separate by contract. A page that reached for
		// /api/v1/queues/generate here could create a second ticket for a
		// citizen who already holds an appointment.
		const calls = mockCheckin(() => jsonResponse({ success: true, data: TICKET }));
		render(CheckIn);
		await waitFor(() => expect(field('ID janji temu')).toBeTruthy());

		await fireEvent.input(field('ID janji temu'), { target: { value: APPT_ID } });
		await fireEvent.input(field('Kode check-in'), { target: { value: CODE } });
		await fireEvent.click(screen.getByRole('button', { name: /^Check-In$/ }));
		await waitFor(() => expect(calls).toHaveLength(1));

		for (const call of calls) {
			expect(call.url).not.toContain('/api/v1/queues/generate');
		}
	});
});
