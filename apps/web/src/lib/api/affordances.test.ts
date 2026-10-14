import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	getScheduleMutationOptions,
	listNotifications,
	retryNotification,
	cancelNotification
} from './endpoints/admin';
import type {
	NotificationOutboxRow,
	ScheduleMutationOptions,
	ScheduleOptionFacility
} from './types/api';

/**
 * Frontend affordance contract (Phase 3B5.0, sections 15 and 16).
 *
 * The client may hold affordance data. It may NOT hold authorization data.
 * The difference is not pedantic: a client that knows it is a super_admin, or
 * that holds `schedule.manage`, or that its facility scope is {A, B}, has
 * become an authorization authority, and an authorization authority is
 * something a user can edit with devtools. The server must stay the only place
 * that decides.
 *
 * So the tests below check the SHAPE of what the client can learn, not just
 * that the endpoints work. An affordance is a domain answer — "these are the
 * options you may offer" — and it must arrive without a reason, a role, a
 * permission name, or a scope echo attached.
 *
 * Nothing here wires UI. The ScheduleEditor and the retry/cancel buttons are
 * deliberately untouched in this pass; these tests only fix the contract the
 * wiring will consume later.
 */

/** The §2/§16 denial set: never in browser or session state. */
const FORBIDDEN_KEYS = [
	'role',
	'roles',
	'user_roles',
	'permission',
	'permissions',
	'permission_keys',
	'role_permissions',
	'grant',
	'grants',
	'facility_grant',
	'facility_scope',
	'unrestricted',
	'super_admin',
	'is_super_admin',
	'is_dev',
	'user_id',
	'app_user_id',
	'capabilities',
	'capability',
	'actions',
	'can_manage',
	'can_edit',
	'practitioner',
	'practitioner_id',
	'actor'
] as const;

/**
 * Recursively collects every key in a decoded payload, at any depth.
 *
 * Depth matters here: a leak nested inside `data.facilities[0]` is exactly as
 * bad as one at the top level, and a shallow check would miss it.
 */
function collectKeys(value: unknown, into: string[] = []): string[] {
	if (Array.isArray(value)) {
		for (const item of value) collectKeys(item, into);
		return into;
	}
	if (value !== null && typeof value === 'object') {
		for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
			into.push(key);
			collectKeys(child, into);
		}
	}
	return into;
}

/** Lowercased key, so `Unrestricted` and `facilityScope` are both caught. */
function assertNoForbiddenKeys(payload: unknown, label: string): void {
	const keys = collectKeys(payload).map((k) => k.toLowerCase());
	for (const forbidden of FORBIDDEN_KEYS) {
		expect(
			keys.includes(forbidden),
			`${label} must not expose "${forbidden}" (saw: ${[...new Set(keys)].join(', ')})`
		).toBe(false);
	}
}

/**
 * A well-formed options payload, used as the fixture the decode is exercised
 * against. Built by hand rather than by a fetch mock so the test is asserting
 * about the CONTRACT, not about whatever a mock happens to return.
 */
const OPTIONS_FIXTURE: ScheduleMutationOptions = {
	facilities: [
		{
			id: 'fac-b',
			name: 'RS Mitra Sehat',
			service_units: [
				{ id: 'su-1', facility_id: 'fac-b', name: 'Poli Umum' },
				{ id: 'su-2', facility_id: 'fac-b', name: 'Poli Gigi' }
			]
		},
		{
			// A manageable facility that has not onboarded a service unit yet.
			// Present with an empty list, NOT omitted.
			id: 'fac-c',
			name: 'Puskesmas Baru',
			service_units: []
		}
	]
};

const ROW_FIXTURE: NotificationOutboxRow = {
	id: 'n-1',
	facility_id: 'fac-b',
	channel: 'dev',
	template_key: 'queue.called',
	subject: 'Nomor antrean Anda',
	body_template: 'Nomor antrean {{queue_number}}',
	recipient_type: 'patient',
	recipient_contact_masked: '+62••••0001',
	status: 'failed',
	attempt_count: 2,
	next_attempt_at: '2026-01-02T03:04:05Z',
	created_at: '2026-01-01T00:00:00Z',
	updated_at: '2026-01-01T00:00:00Z',
	can_retry: true,
	can_cancel: true
};

describe('schedule mutation options: domain choices only', () => {
	it('carries ids, names, and the parent relationship — nothing else', () => {
		assertNoForbiddenKeys(OPTIONS_FIXTURE, 'schedule options');

		const facility = OPTIONS_FIXTURE.facilities[0] as ScheduleOptionFacility;
		expect(Object.keys(facility).sort()).toEqual(['id', 'name', 'service_units']);
		expect(Object.keys(facility.service_units[0]).sort()).toEqual([
			'facility_id',
			'id',
			'name'
		]);
	});

	it('gives the editor what it needs to build the form', () => {
		// Every facility has a human-readable name, or the form shows a UUID.
		for (const facility of OPTIONS_FIXTURE.facilities) {
			expect(facility.id).toBeTruthy();
			expect(facility.name).toBeTruthy();
			// service_units is always an array. A null here would force every
			// consumer into a null check, and `null` is indistinguishable from
			// "the server did not answer" — which is exactly the ambiguity the
			// explicit empty array removes.
			expect(Array.isArray(facility.service_units)).toBe(true);
			// Each unit points back at its parent, so the editor can bind the
			// nesting without a second request.
			for (const unit of facility.service_units) {
				expect(unit.facility_id).toBe(facility.id);
			}
		}
	});

	it('represents "manageable nowhere" as an empty list, not as an error', () => {
		// The zero-manageable case is a successful answer. A client that treated
		// it as a failure would show a connection error where the truth is a
		// capability refusal, and the operator would have no way to tell the
		// difference.
		const empty: ScheduleMutationOptions = { facilities: [] };
		expect(empty.facilities).toEqual([]);
		expect(empty.facilities).toHaveLength(0);
	});

	it('exposes no practitioner data of any kind', () => {
		// Schedules are practitioner-bound, so this is the most tempting place
		// for a convenience field to leak in.
		const keys = collectKeys(OPTIONS_FIXTURE).map((k) => k.toLowerCase());
		expect(keys.some((k) => k.includes('practitioner'))).toBe(false);
	});
});

describe('notification actionability: booleans only', () => {
	it('adds exactly can_retry and can_cancel to the existing row', () => {
		assertNoForbiddenKeys(ROW_FIXTURE, 'notification row');

		const keys = Object.keys(ROW_FIXTURE);
		expect(keys).toContain('can_retry');
		expect(keys).toContain('can_cancel');
		// Both are booleans. A string or a status enum here would re-expose the
		// state machine to the client, which is the thing the server owns.
		expect(typeof ROW_FIXTURE.can_retry).toBe('boolean');
		expect(typeof ROW_FIXTURE.can_cancel).toBe('boolean');
	});

	it('keeps the existing row shape intact', () => {
		// The projection is additive. Removing or renaming an existing field
		// would break the admin client for no security benefit, and the server
		// side is deliberately byte-for-byte what it was.
		for (const key of [
			'id',
			'facility_id',
			'channel',
			'template_key',
			'subject',
			'body_template',
			'recipient_type',
			'recipient_contact_masked',
			'status',
			'attempt_count',
			'next_attempt_at',
			'created_at',
			'updated_at'
		]) {
			expect(ROW_FIXTURE).toHaveProperty(key);
		}
	});

	it('carries no reason code alongside the booleans', () => {
		// `false` is ambiguous on purpose: ineligible by status, by permission
		// at this row's facility, or both. Shipping a reason would tell the
		// browser enough to start reconstructing the authorization model, and
		// "reconstruct the authorization model in the browser" is the failure
		// this whole phase exists to prevent.
		const keys = Object.keys(ROW_FIXTURE).map((k) => k.toLowerCase());
		for (const leaky of ['reason', 'reason_code', 'denied_because', 'cause', 'why']) {
			expect(keys).not.toContain(leaky);
		}
	});

	it('stays masked: no raw contact and no dedup hash', () => {
		expect(ROW_FIXTURE.recipient_contact_masked).toBe('+62••••0001');
		const keys = Object.keys(ROW_FIXTURE).map((k) => k.toLowerCase());
		expect(keys.some((k) => k.includes('hash'))).toBe(false);
		expect(keys.some((k) => k.includes('recipient_contact') && !k.includes('masked'))).toBe(
			false
		);
	});
});

describe('client surface: the affordance contract is readable and read-only', () => {
	it('exposes a typed options reader', () => {
		// The function exists and is callable. Phase 3B5.0 deliberately does NOT
		// wire it to any page, so this asserts availability, not usage.
		expect(typeof getScheduleMutationOptions).toBe('function');
	});

	it('keeps the schedule mutations themselves untouched', () => {
		// The affordance is advisory. The mutation endpoints must remain the
		// authority, and they must remain POSTs to their own paths — not folded
		// into the options read.
		expect(typeof retryNotification).toBe('function');
		expect(typeof cancelNotification).toBe('function');
		expect(typeof listNotifications).toBe('function');
	});
});

/* ------------------------------ wire contract ----------------------------- */

const originalFetch = globalThis.fetch;

beforeEach(() => {
	Object.defineProperty(window, 'location', {
		value: { origin: 'http://127.0.0.1:4173' },
		writable: true,
		configurable: true
	});
});

afterEach(() => {
	globalThis.fetch = originalFetch;
	vi.restoreAllMocks();
});

/** Mimics the Go encoder, trailing newline included. */
function upstream(body: unknown, status = 200): Response {
	return new Response(`${JSON.stringify(body)}\n`, {
		status,
		headers: { 'content-type': 'application/json; charset=utf-8' }
	});
}

describe('options request', () => {
	it('reads the literal /options sub-path, not a schedule id', async () => {
		const fetchMock = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit) =>
			upstream({ success: true, data: OPTIONS_FIXTURE })
		);
		globalThis.fetch = fetchMock as unknown as typeof fetch;

		const result = await getScheduleMutationOptions();

		expect(result.ok).toBe(true);
		// The path is the whole point. `/options` must not be built as
		// `/schedules/options` against an [id] route, and must not be confused
		// with the read list at `/schedules`.
		expect(fetchMock).toHaveBeenCalledTimes(1);
		const url = String(fetchMock.mock.calls[0][0]);
		expect(url).toBe('/api/v1/admin/schedules/options');
	});

	it('decodes the nested payload through the envelope', async () => {
		globalThis.fetch = vi.fn(async () =>
			upstream({ success: true, data: OPTIONS_FIXTURE })
		) as unknown as typeof fetch;

		const result = await getScheduleMutationOptions();
		if (!result.ok) throw new Error('expected ok');
		// `data` here is `{facilities: [...]}` — the envelope unwraps exactly one
		// level, so the client must not expect a bare array.
		expect(Array.isArray(result.data)).toBe(false);
		expect(result.data.facilities).toHaveLength(2);
		expect(result.data.facilities[0].service_units[0].facility_id).toBe('fac-b');
	});

	it('surfaces an empty facility list as success, not as an error', async () => {
		globalThis.fetch = vi.fn(async () =>
			upstream({ success: true, data: { facilities: [] } })
		) as unknown as typeof fetch;

		const result = await getScheduleMutationOptions();
		// This is the behaviour the UI depends on: a reader with no manageable
		// facility gets a capability refusal, never a failure banner.
		expect(result.ok).toBe(true);
		if (!result.ok) return;
		expect(result.data.facilities).toEqual([]);
	});

	it('does not send any authorization header from the browser', async () => {
		const fetchMock = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit) =>
			upstream({ success: true, data: OPTIONS_FIXTURE })
		);
		globalThis.fetch = fetchMock as unknown as typeof fetch;

		await getScheduleMutationOptions();

		const init = fetchMock.mock.calls[0][1] as
			| { headers?: Record<string, string> }
			| undefined;
		const headers = init?.headers ?? {};
		// The same-origin proxy attaches the session cookie server-side. A
		// bearer token here would put a credential in browser code, where it can
		// be read and replayed.
		for (const header of Object.keys(headers)) {
			expect(header.toLowerCase()).not.toBe('authorization');
		}
	});
});

describe('notification actionability on the wire', () => {
	it('reads can_retry and can_cancel off each listed row', async () => {
		globalThis.fetch = vi.fn(async () =>
			upstream({ success: true, data: [ROW_FIXTURE] })
		) as unknown as typeof fetch;

		const result = await listNotifications();
		if (!result.ok) throw new Error('expected ok');
		expect(result.data).toHaveLength(1);
		// The booleans are TOP-LEVEL keys on the row, siblings of `status`. If
		// the server nested them under an object, or omitted them, this fails —
		// which is the check that keeps the Go embedding decision honest.
		expect(result.data[0].can_retry).toBe(true);
		expect(result.data[0].can_cancel).toBe(true);
		assertNoForbiddenKeys(result.data[0], 'decoded notification row');
	});

	it('leaves them undefined when the server does not send them', async () => {
		// An older server, or a cached response, must not crash a consumer.
		// `undefined` is falsy, so a `can_retry &&` guard hides the control —
		// which is the safe direction.
		const { can_retry, can_cancel, ...withoutActionability } = ROW_FIXTURE;
		globalThis.fetch = vi.fn(async () =>
			upstream({ success: true, data: [withoutActionability] })
		) as unknown as typeof fetch;

		const result = await listNotifications();
		if (!result.ok) throw new Error('expected ok');
		expect(result.data[0].can_retry).toBeUndefined();
		expect(result.data[0].can_cancel).toBeUndefined();
		expect(Boolean(result.data[0].can_retry)).toBe(false);
	});

	it('still exposes the mutations as independent POSTs', async () => {
		// The mock's parameters are typed, not inferred away: a zero-arg
		// `vi.fn(async () => ...)` produces a `[]` tuple, and indexing element 1
		// of that is a type error rather than a runtime surprise. Declaring the
		// parameters keeps both the compiler and the assertion honest.
		const fetchMock = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit) =>
			upstream({ success: true, data: ROW_FIXTURE })
		);
		globalThis.fetch = fetchMock as unknown as typeof fetch;

		await retryNotification('n-1');
		await cancelNotification('n-1');

		// The affordance must not have absorbed the mutation into the read path.
		// Each stays its own request to its own endpoint, which re-derives
		// authorization and state server-side regardless of what the list said.
		const [retryCall, cancelCall] = fetchMock.mock.calls;
		expect(String(retryCall[0])).toBe('/api/v1/admin/notifications/n-1/retry');
		expect(retryCall[1]?.method).toBe('POST');
		expect(String(cancelCall[0])).toBe('/api/v1/admin/notifications/n-1/cancel');
		expect(cancelCall[1]?.method).toBe('POST');
	});
});
