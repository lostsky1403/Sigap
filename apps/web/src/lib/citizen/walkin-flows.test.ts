import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import WalkIn from '../../routes/queues/new/+page.svelte';

/**
 * Walk-in queue registration at /queues/new.
 *
 * The contract under test is the WIRE SHAPE, because getting it wrong is a
 * silent no-op. `GenerateRequest` in queue.go takes camelCase `facilityId`
 * and a nested `patient` object with `fullName` — the OPPOSITE convention from
 * `BookAppointmentRequest` in the same Go package, which is snake_case. Go
 * decodes JSON by ignoring unknown keys, so a snake_case body here would
 * arrive as an empty struct and fail validation with no visible cause. So the
 * exact body is asserted rather than assumed, and a test asserts the page
 * never reaches for the appointment check-in route, which would mutate a
 * booking that does not exist for a walk-in.
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
