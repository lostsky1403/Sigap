import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { apiFetch, apiFetchList, buildUrl } from './client';
import {
	isPermissionDenied,
	isRetryable,
	isTransport,
	kindForStatus,
	requiresAuth
} from './errors';
import { wrongCheckInCode } from './endpoints/public';

/**
 * API layer contract (Phase 3B1 sections 12 and 13).
 *
 * The cases below are the ones that are easy to get quietly wrong and expensive
 * in production: a 403 that means two different things, a 401 that is not an
 * auth failure, a JSON body with a trailing newline, and an abort that must not
 * be shown to a user as an error.
 */

/** Builds a Response the way the backend does, newline and all. */
function jsonResponse(
	body: unknown,
	{ status = 200, contentType = 'application/json' } = {}
): Response {
	return new Response(
		// json.NewEncoder().Encode() appends a trailing newline. Reproducing it
		// here means a client that only works on trimmed bodies fails here too.
		typeof body === 'string' ? body : `${JSON.stringify(body)}\n`,
		{ status, headers: { 'content-type': `${contentType}; charset=utf-8` } }
	);
}

const originalFetch = globalThis.fetch;

beforeEach(() => {
	// Relative URLs need an origin to resolve against.
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

describe('buildUrl', () => {
	it('omits undefined and null query values entirely', () => {
		// An empty `?status=` is a different request to the backend than an
		// absent one, so absent keys must not serialize.
		const url = buildUrl('/api/v1/admin/notifications', {
			limit: 50,
			status: undefined,
			channel: null
		});
		expect(url).toBe('/api/v1/admin/notifications?limit=50');
	});

	it('encodes query values', () => {
		const url = buildUrl('/api/v1/patient/status', { code: 'AB 12/CD' });
		expect(url).toBe('/api/v1/patient/status?code=AB+12%2FCD');
	});

	it('stays same-origin', () => {
		expect(buildUrl('/api/v1/public/facilities').startsWith('/api/')).toBe(true);
	});
});

describe('apiFetch success', () => {
	it('unwraps the {success, data} envelope and tolerates a trailing newline', async () => {
		globalThis.fetch = vi.fn(async () =>
			jsonResponse({ success: true, data: { id: 'a', name: 'Pusat Kota' } })
		) as never;

		const result = await apiFetch<{ id: string }>('/api/v1/public/facilities');
		expect(result.ok).toBe(true);
		if (result.ok) expect(result.data.id).toBe('a');
	});

	it('sends credentials same-origin and never a bearer token', async () => {
		const calls: [string, RequestInit | undefined][] = [];
		globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
			calls.push([String(url), init]);
			return jsonResponse({ success: true, data: [] });
		}) as never;

		await apiFetch('/api/v1/admin/queues');

		const init = calls[0][1] as RequestInit & { headers: Record<string, string> };
		expect(init.credentials).toBe('same-origin');
		const headerNames = Object.keys(init.headers).map((h) => h.toLowerCase());
		// The session cookie is attached by the same-origin proxy server-side.
		// A browser-visible bearer token would be a credential-handling
		// regression, so its absence is asserted rather than assumed.
		expect(headerNames).not.toContain('authorization');
	});

	it('returns undefined data for an empty-bodied success', async () => {
		globalThis.fetch = vi.fn(async () => new Response(null, { status: 204 })) as never;
		const result = await apiFetch('/api/v1/admin/queues/1/status');
		expect(result.ok).toBe(true);
	});

	it('returns a bare payload untouched when it is not an envelope', async () => {
		globalThis.fetch = vi.fn(async () => jsonResponse({ status: 'ok', service: 'sigap-api' })) as never;
		const result = await apiFetch<{ status: string }>('/health');
		expect(result.ok).toBe(true);
		if (result.ok) expect(result.data.status).toBe('ok');
	});
});

describe('apiFetch error normalization', () => {
	const cases: [number, string][] = [
		[400, 'bad_request'],
		[401, 'unauthorized'],
		[403, 'forbidden'],
		[404, 'not_found'],
		[409, 'conflict'],
		[429, 'rate_limited'],
		[500, 'server'],
		[503, 'server']
	];

	for (const [status, kind] of cases) {
		it(`maps ${status} to ${kind} and keeps the backend message`, async () => {
			globalThis.fetch = vi.fn(async () =>
				jsonResponse({ success: false, error: 'Pesan dari backend' }, { status })
			) as never;

			const result = await apiFetch('/api/v1/admin/queues');
			expect(result.ok).toBe(false);
			if (!result.ok) {
				expect(result.error.kind).toBe(kind);
				expect(result.error.status).toBe(status);
				expect(result.error.message).toBe('Pesan dari backend');
			}
		});
	}

	it('does not throw on a bare 405 with no body', async () => {
		// Unlisted methods answer with a status and no body at all. A client
		// that called response.json() unconditionally would throw here.
		globalThis.fetch = vi.fn(async () => new Response(null, { status: 405 })) as never;

		const result = await apiFetch('/api/v1/admin/queues', { method: 'PUT' });
		expect(result.ok).toBe(false);
		if (!result.ok) expect(result.error.status).toBe(405);
	});

	it('survives a malformed JSON body', async () => {
		globalThis.fetch = vi.fn(async () => jsonResponse('{not json')) as never;
		const result = await apiFetch('/api/v1/public/facilities');
		expect(result.ok).toBe(false);
		if (!result.ok) expect(result.error.kind).toBe('unknown');
	});

	it('reports a network failure as network, not server', async () => {
		globalThis.fetch = vi.fn(async () => {
			throw new TypeError('Failed to fetch');
		}) as never;

		const result = await apiFetch('/api/v1/public/facilities');
		expect(result.ok).toBe(false);
		if (!result.ok) {
			expect(result.error.kind).toBe('network');
			expect(result.error.status).toBe(0);
		}
	});

	it('treats an abort as aborted, so no scary error is shown', async () => {
		const controller = new AbortController();
		globalThis.fetch = vi.fn(async () => {
			controller.abort();
			throw new DOMException('aborted', 'AbortError');
		}) as never;

		const result = await apiFetch('/api/v1/admin/queues', { signal: controller.signal });
		expect(result.ok).toBe(false);
		if (!result.ok) {
			expect(result.error.kind).toBe('aborted');
			expect(isTransport(result.error)).toBe(true);
			// An abort is not worth retrying: the caller navigated away.
			expect(isRetryable(result.error)).toBe(false);
		}
	});
});

describe('error union semantics', () => {
	it('maps every status the backend can return', () => {
		expect(kindForStatus(400)).toBe('bad_request');
		expect(kindForStatus(429)).toBe('rate_limited');
		expect(kindForStatus(503)).toBe('server');
		expect(kindForStatus(418)).toBe('unknown');
	});

	it('treats 403 with no session as needing authentication', () => {
		// The admin auth layer returns 403 both for a missing session and for
		// insufficient permission, so the session flag is the only signal.
		const error = { kind: 'forbidden' as const, status: 403, message: '' };
		expect(requiresAuth(error, false)).toBe(true);
		expect(isPermissionDenied(error, false)).toBe(false);
	});

	it('treats 403 with a session as a permission refusal', () => {
		const error = { kind: 'forbidden' as const, status: 403, message: '' };
		expect(requiresAuth(error, true)).toBe(false);
		expect(isPermissionDenied(error, true)).toBe(true);
	});

	it('always treats 401 as needing authentication', () => {
		const error = { kind: 'unauthorized' as const, status: 401, message: '' };
		expect(requiresAuth(error, true)).toBe(true);
		expect(requiresAuth(error, false)).toBe(true);
	});

	it('does not offer a retry that could never succeed', () => {
		expect(isRetryable({ kind: 'not_found', status: 404, message: '' })).toBe(false);
		expect(isRetryable({ kind: 'forbidden', status: 403, message: '' })).toBe(false);
		expect(isRetryable({ kind: 'network', status: 0, message: '' })).toBe(true);
	});
});

describe('check-in 401 is a wrong code, not an auth failure', () => {
	it('is recognised on the public check-in route', () => {
		// The backend answers 401 "Kode check-in tidak cocok." A citizen who
		// mistyped their code must not be shown a sign-in prompt.
		expect(wrongCheckInCode({ kind: 'unauthorized', status: 401 })).toBe(true);
	});

	it('does not fire for other failures', () => {
		expect(wrongCheckInCode({ kind: 'conflict', status: 409 })).toBe(false);
		expect(wrongCheckInCode({ kind: 'forbidden', status: 403 })).toBe(false);
		expect(wrongCheckInCode(null)).toBe(false);
	});
});

/**
 * The Go API encodes a nil slice as `data: null`, so an authorized request that
 * legitimately matches zero rows arrives as null rather than []. These cases pin
 * the normalization so a route can call rows.map() without a null check.
 */
describe('apiFetchList normalizes the Go null empty list', () => {
	it('turns data: null into an empty array', async () => {
		globalThis.fetch = vi.fn(async () =>
			jsonResponse({ success: true, data: null })
		) as never;

		const result = await apiFetchList('/api/v1/admin/queues');
		expect(result.ok).toBe(true);
		if (result.ok) {
			expect(Array.isArray(result.data)).toBe(true);
			expect(result.data).toEqual([]);
		}
	});

	it('leaves a real array untouched', async () => {
		globalThis.fetch = vi.fn(async () =>
			jsonResponse({ success: true, data: [{ id: 'q1' }] })
		) as never;

		const result = await apiFetchList<{ id: string }>('/api/v1/admin/queues');
		expect(result.ok).toBe(true);
		if (result.ok) expect(result.data).toHaveLength(1);
	});

	it('handles the fail-closed empty array the same way', async () => {
		// The zero-facility early return passes a composite literal, so the same
		// route can legitimately answer []. Both empty shapes must converge.
		globalThis.fetch = vi.fn(async () =>
			jsonResponse({ success: true, data: [] })
		) as never;

		const result = await apiFetchList('/api/v1/admin/queues');
		expect(result.ok).toBe(true);
		if (result.ok) expect(result.data).toEqual([]);
	});

	it('propagates a failure instead of inventing an empty list', async () => {
		// Normalizing must never swallow a 403 into a plausible-looking empty
		// table, which would read as "no data" instead of "no access".
		globalThis.fetch = vi.fn(async () =>
			jsonResponse({ success: false, error: 'Akses ditolak.' }, { status: 403 })
		) as never;

		const result = await apiFetchList('/api/v1/admin/queues');
		expect(result.ok).toBe(false);
		if (!result.ok) {
			expect(result.error.kind).toBe('forbidden');
			expect(result.error.message).toBe('Akses ditolak.');
		}
	});

	it('keeps an abort as an abort rather than an empty list', async () => {
		const controller = new AbortController();
		globalThis.fetch = vi.fn(async () => {
			controller.abort();
			throw new DOMException('The operation was aborted.', 'AbortError');
		}) as never;

		const result = await apiFetchList('/api/v1/admin/queues', {
			signal: controller.signal
		});
		expect(result.ok).toBe(false);
		if (!result.ok) expect(result.error.kind).toBe('aborted');
	});
});
