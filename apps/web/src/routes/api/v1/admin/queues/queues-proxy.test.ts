import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import type { RequestEvent } from '@sveltejs/kit';
import { GET } from './+server';

/**
 * T-3B5 — the queue proxy's service-day scope rides on `?date=YYYY-MM-DD`.
 *
 * The Go API defaults `registered_at` to "today (WIB)" when the query string is
 * missing, so dropping `?date=` silently renders a historical day as if it were
 * today — a wrong-result regression with no error to look for. This pins both
 * directions of the forwarding: a present date survives the `URL().search`
 * round-trip, and an absent query stays absent (no stray `?date=undefined`).
 */

const ORIGINAL_API_INTERNAL = process.env.SIGAP_API_INTERNAL;

const fetchMock = vi.fn();

/** The URL of the last request the proxy issued to the Go API. */
function lastUrl(): string {
	const [input] = fetchMock.mock.calls[fetchMock.mock.calls.length - 1];
	return String(input);
}

/**
 * A minimal RequestEvent carrying only what this proxy reads: `request.url`
 * (parsed by GET), `request.method`/`request.headers` (read by `proxy`), and
 * `locals` (read by `authHeaders`, which needs `locals.session` to exist).
 */
function eventFor(url: string): RequestEvent {
	return {
		request: new Request(url),
		locals: {}
	} as unknown as RequestEvent;
}

beforeEach(() => {
	process.env.SIGAP_API_INTERNAL = 'http://127.0.0.1:18080';
	fetchMock.mockReset();
	// A FACTORY, not one shared Response: `proxy` reads the body once per call,
	// and a reused instance would throw "Body is unusable" on the second read.
	fetchMock.mockImplementation(async () =>
		new Response(JSON.stringify({ success: true, data: [] }), {
			status: 200,
			headers: { 'Content-Type': 'application/json' }
		})
	);
	vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
	vi.unstubAllGlobals();
	if (ORIGINAL_API_INTERNAL === undefined) delete process.env.SIGAP_API_INTERNAL;
	else process.env.SIGAP_API_INTERNAL = ORIGINAL_API_INTERNAL;
});

describe('GET /api/v1/admin/queues — ?date= service-day forwarding', () => {
	it('forwards ?date=YYYY-MM-DD through to the upstream Go API', async () => {
		await GET(eventFor('http://origin/api/v1/admin/queues?date=2026-11-08'));

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const url = lastUrl();
		expect(url).toContain('/api/v1/admin/queues?date=2026-11-08');
		expect(url).toContain('date=2026-11-08');
	});

	it('forwards no ?date= when the request has no query string', async () => {
		await GET(eventFor('http://origin/api/v1/admin/queues'));

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const url = lastUrl();
		expect(url).toContain('/api/v1/admin/queues');
		expect(url).not.toContain('?date=');
		expect(url).not.toContain('date=undefined');
	});
});