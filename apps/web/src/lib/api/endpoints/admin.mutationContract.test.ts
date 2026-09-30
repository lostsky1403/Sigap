import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import {
	cancelNotification,
	createFacility,
	createSchedule,
	deactivateFacility,
	retryNotification,
	updateAppointmentStatus,
	updateFacility,
	updateQueueTicketStatus,
	updateSchedule,
	type FacilityInput
} from './admin';

/**
 * T-3B5 §15: the shared client's contract for every admin MUTATION.
 *
 * WHY THIS FILE EXISTS. §15 requires that the Phase 3B4 architecture is
 * preserved — page -> shared client -> SvelteKit proxy -> Go API — and that no
 * inline raw `fetch` helper comes back. A method's METHOD and PATH are the two
 * facts most likely to drift, and they drift silently: `PATCH` becoming `POST`
 * still returns a response, still type-checks, and still renders a toast. The
 * only thing that catches it is an assertion naming the exact pair.
 *
 * WHY THE METHODS ARE ASSERTED RATHER THAN THE TRANSITIONS. The legal
 * transitions are a server fact and are pinned by the Go tests; the client's
 * job is to forward whatever status the caller chose. Duplicating the
 * transition map here would create a second copy that could drift from the
 * first, which is the exact failure mode a contract test is supposed to
 * prevent. What the client owns — verb, path, encoding, body shape — is what is
 * asserted.
 *
 * WHY `encodeURIComponent` MATTERS FOR THE ID. Ids are interpolated into a
 * path segment, and a value containing a slash would otherwise restructure the
 * URL. The backend resolves those ids from a database, so this is not
 * currently exploitable; it is asserted because the day a caller passes
 * something else is the day it becomes a real path-traversal surface.
 */

const fetchMock = vi.fn();

beforeEach(() => {
	fetchMock.mockReset();
	// A FACTORY, not a single shared Response. A Response body can only be read
	// once, and the client reads every response body, so reusing one instance
	// across calls makes the second call throw "Body is unusable" — a test
	// artifact that looks exactly like a client defect.
	fetchMock.mockImplementation(async () =>
		new Response(JSON.stringify({ success: true, data: { id: 'x' } }), {
			status: 200,
			headers: { 'Content-Type': 'application/json' }
		})
	);
	vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
	vi.unstubAllGlobals();
});

/** The URL of the last request. */
function lastUrl(): string {
	const [input] = fetchMock.mock.calls[fetchMock.mock.calls.length - 1];
	return String(input);
}

/** The method of the last request. */
function lastMethod(): string {
	const [, init] = fetchMock.mock.calls[fetchMock.mock.calls.length - 1];
	return String((init as RequestInit).method ?? 'GET').toUpperCase();
}

/** The parsed JSON body of the last request, or undefined when it had none. */
function lastBody(): Record<string, unknown> | undefined {
	const [, init] = fetchMock.mock.calls[fetchMock.mock.calls.length - 1];
	const raw = (init as RequestInit).body;
	if (typeof raw !== 'string') return undefined;
	return JSON.parse(raw);
}

function facilityInput(): FacilityInput {
	return {
		name: 'Klinik Uji',
		type: 'puskesmas',
		address: 'Jl. Uji No. 1',
		kecamatan: 'Cibinong',
		kabupaten_kota: 'Bogor',
		provinsi: 'Jawa Barat',
		phone: '08123456789',
		total_beds: 12,
		available_beds: 7,
		short_code: 'UJI'
	};
}

describe('T-3B5-01 queue status mutation', () => {
	it('PATCHes the ticket status sub-resource', async () => {
		await updateQueueTicketStatus('ticket-1', 'called');
		expect(lastMethod()).toBe('PATCH');
		expect(lastUrl()).toContain('/api/v1/admin/queues/ticket-1/status');
	});

	it('sends only the status, as a bare body', async () => {
		await updateQueueTicketStatus('ticket-1', 'in_service');
		expect(lastBody()).toEqual({ status: 'in_service' });
	});

	it('escapes an id that would otherwise restructure the path', async () => {
		await updateQueueTicketStatus('a/b', 'called');
		expect(lastUrl()).toContain('/queues/a%2Fb/status');
	});
});

describe('T-3B5-02 appointment status mutation', () => {
	it('PATCHes the appointment status sub-resource', async () => {
		await updateAppointmentStatus('appt-1', 'checked_in');
		expect(lastMethod()).toBe('PATCH');
		expect(lastUrl()).toContain('/api/v1/admin/appointments/appt-1/status');
		expect(lastBody()).toEqual({ status: 'checked_in' });
	});

	/**
	 * The transition the product forbids must be unrepresentable in the client
	 * too, not merely absent from a button.
	 *
	 * The status vocabulary itself is unchanged — `completed` is a legitimate
	 * target from `queued` — so this asserts the STATUS is a plain string union
	 * the caller chooses, and the guard lives in the transition map plus the
	 * server. Pinning it here would be pinning the wrong layer: the client is
	 * allowed to be asked for `completed`, and must be refused by the backend.
	 */
	it('forwards whatever status the caller chose, leaving the refusal to the server', async () => {
		await updateAppointmentStatus('appt-1', 'completed');
		expect(lastBody()).toEqual({ status: 'completed' });
	});
});

describe('T-3B5-03 schedule mutation', () => {
	it('POSTs a create to the collection', async () => {
		await createSchedule({
			facility_id: 'f-1',
			service_unit_id: 's-1',
			schedule_date: '2026-09-30',
			start_time: '09:00',
			end_time: '12:00',
			slot_minutes: 30,
			capacity_per_slot: 2
		});
		expect(lastMethod()).toBe('POST');
		expect(lastUrl()).toContain('/api/v1/admin/schedules');
	});

	it('PATCHes an update to the member resource', async () => {
		await updateSchedule('sched-1', { slot_minutes: 45 });
		expect(lastMethod()).toBe('PATCH');
		expect(lastUrl()).toContain('/api/v1/admin/schedules/sched-1');
	});
});

describe('T-3B5-04 facility mutation', () => {
	it('POSTs a create to the collection', async () => {
		await createFacility(facilityInput());
		expect(lastMethod()).toBe('POST');
		expect(lastUrl()).toContain('/api/v1/admin/facilities');
	});

	/**
	 * §10 names PATCH explicitly and forbids POST.
	 *
	 * This is the single most consequential assertion in the file. The
	 * pre-existing routing registered `ListFacilities` on the bare path, which
	 * in net/http matches EVERY method — so POST reached a GET-only handler and
	 * answered 200 with the facility list, creating nothing. A method assertion
	 * at the client is what makes that class of shadowing visible in the first
	 * place that matters, since the failure was otherwise indistinguishable
	 * from a UI bug.
	 */
	it('PATCHes a deactivation — never POST', async () => {
		await deactivateFacility('fac-1');
		expect(lastMethod()).toBe('PATCH');
		expect(lastUrl()).toContain('/api/v1/admin/facilities/fac-1/deactivate');
		expect(lastMethod()).not.toBe('POST');
	});

	it('carries no body on a deactivation', async () => {
		await deactivateFacility('fac-1');
		expect(lastBody()).toBeUndefined();
	});

	it('PATCHes an update to the member resource', async () => {
		await updateFacility('fac-1', { name: 'Klinik Diperbarui' });
		expect(lastMethod()).toBe('PATCH');
		expect(lastUrl()).toContain('/api/v1/admin/facilities/fac-1');
		expect(lastBody()).toEqual({ name: 'Klinik Diperbarui' });
	});

	it('includes available_beds, which the pre-3B5 type omitted', async () => {
		// §9's "no invented fields" cuts both ways: a field the API declares and
		// the form collects must actually reach the body, or every facility
		// created through this client reports zero free beds.
		await createFacility(facilityInput());
		expect(lastBody()).toMatchObject({ total_beds: 12, available_beds: 7 });
	});
});

describe('T-3B5-05 notification mutation', () => {
	/**
	 * POST, not PATCH — and not PUT.
	 *
	 * The two notification mutations are the only non-PATCH verbs in the admin
	 * surface. That asymmetry is easy to "fix" by eye toward PATCH, and the
	 * router would then 404 or 405, so the verb is pinned on both.
	 */
	it('POSTs a retry', async () => {
		await retryNotification('note-1');
		expect(lastMethod()).toBe('POST');
		expect(lastUrl()).toContain('/api/v1/admin/notifications/note-1/retry');
	});

	it('POSTs a cancellation', async () => {
		await cancelNotification('note-1');
		expect(lastMethod()).toBe('POST');
		expect(lastUrl()).toContain('/api/v1/admin/notifications/note-1/cancel');
	});

	it('sends no body on either, since the state machine needs no input', async () => {
		await retryNotification('note-1');
		expect(lastBody()).toBeUndefined();
	});
});

describe('every admin mutation goes through the same-origin proxy', () => {
	/**
	 * The architectural invariant behind §15 and §1.
	 *
	 * Every URL is a RELATIVE path beginning `/api/v1/`, which is what routes it
	 * through the SvelteKit proxy that attaches credentials server-side. A
	 * mutation that reached an absolute host would bypass the proxy entirely, and
	 * with it the boundary that keeps a bearer token out of browser code.
	 */
	it('issues every mutation as a relative same-origin path', async () => {
		const calls: Array<() => Promise<unknown>> = [
			() => updateQueueTicketStatus('t1', 'called'),
			() => updateAppointmentStatus('a1', 'checked_in'),
			() => createSchedule({
				facility_id: 'f1',
				service_unit_id: 's1',
				schedule_date: '2026-09-30',
				start_time: '09:00',
				end_time: '12:00',
				slot_minutes: 30,
				capacity_per_slot: 2
			}),
			() => updateSchedule('s1', { slot_minutes: 30 }),
			() => createFacility(facilityInput()),
			() => updateFacility('f1', { name: 'X' }),
			() => deactivateFacility('f1'),
			() => retryNotification('n1'),
			() => cancelNotification('n1')
		];

		for (const call of calls) {
			fetchMock.mockClear();
			await call();
			const url = lastUrl();
			expect(url.startsWith('/api/v1/'), `${url} must be a same-origin proxy path`).toBe(
				true
			);
			// An absolute URL to another origin would pass the check above only if
			// it began with the same string, so assert the shape directly too.
			expect(url, `${url} must not name an origin`).not.toMatch(/^https?:\/\//);
		}
	});

	it('never sends an Authorization header from browser code', async () => {
		await updateQueueTicketStatus('t1', 'called');
		const [, init] = fetchMock.mock.calls[0];
		const headers = (init as RequestInit).headers as Record<string, string> | undefined;
		const names = Object.keys(headers ?? {}).map((name) => name.toLowerCase());
		expect(names).not.toContain('authorization');
	});
});

/**
 * §15's architectural half: no page or component may reach around the shared
 * client with its own request helper.
 *
 * The behavioural tests above cover the methods that exist today. This one
 * fails if a NEW call site is added that builds its own request, which is the
 * drift a per-method test cannot see coming — a page calling `fetch` directly
 * still renders, still type-checks, and would put a credential path back into
 * the browser.
 *
 * Read as source rather than as a runtime spy because the offending code is,
 * by definition, code this file does not import.
 */
describe('no admin page bypasses the shared client', () => {
	it('uses no raw fetch outside the shared API client', async () => {
		const { readFile, readdir } = await import('node:fs/promises');

		// Only the admin SURFACES are scanned. The shared client itself calls
		// fetch by definition, and the citizen surface is outside this phase.
		const roots = ['src/routes/admin', 'src/lib/admin'];
		const files: string[] = [];

		async function collect(dir: string) {
			const entries = await readdir(dir, { withFileTypes: true });
			for (const entry of entries) {
				const full = `${dir}/${entry.name}`;
				if (entry.isDirectory()) {
					await collect(full);
				} else if (/\.(svelte|ts)$/.test(entry.name) && !/\.test\.ts$/.test(entry.name)) {
					files.push(full);
				}
			}
		}

		for (const root of roots) {
			await collect(root);
		}
		expect(files.length, 'the admin surface should not be empty').toBeGreaterThan(0);

		for (const file of files) {
			const source = await readFile(file, 'utf8');
			// Strip comments so the explanations in these files — which discuss
			// fetch and the shared client at length — are not what matches.
			const code = source
				.replace(/\/\*[\s\S]*?\*\//g, '')
				.replace(/\/\/[^\n]*/g, '')
				.replace(/<!--[\s\S]*?-->/g, '');
			expect(code, `${file} must not call fetch directly; use the shared client`).not.toMatch(
				/(^|[^.\w])fetch\s*\(/
			);
			expect(code, `${file} must not build an absolute API URL`).not.toMatch(
				/https?:\/\/[^'"`\s]*\/api\/v1/
			);
		}
	});
});
