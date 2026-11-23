import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { RequestEvent } from '@sveltejs/kit';
import { GET } from './organizations/+server';

/**
 * Finding D regression: every proxy must use the graceful resolveApiBase()
 * (503 envelope when SIGAP_API_INTERNAL is unset), never the throwing apiBase().
 * A misconfigured deploy must fail closed with the documented response shape,
 * not an unhandled 500 / dev stack trace — and must never turn an auth/config
 * failure into a success.
 */

const here = dirname(fileURLToPath(import.meta.url));
const apiV1Root = resolve(here, '..'); // src/routes/api/v1

function walk(dir: string, out: string[] = []): string[] {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) walk(p, out);
		else if (name === '+server.ts') out.push(p);
	}
	return out;
}

describe('apiBase() migration contract', () => {
	it('no proxy imports or calls the throwing apiBase()', () => {
		for (const file of walk(apiV1Root)) {
			const src = readFileSync(file, 'utf8');
			const rel = relative(apiV1Root, file).replace(/\\/g, '/');
			expect(src, `${rel} must not import apiBase`).not.toMatch(/import[^;]*\bapiBase\b[^;]*from/);
			expect(src, `${rel} must not call apiBase()`).not.toMatch(/\bapiBase\(\)/);
		}
	});

	it('every proxy that fetches an upstream uses resolveApiBase', () => {
		for (const file of walk(apiV1Root)) {
			const src = readFileSync(file, 'utf8');
			if (!src.includes('fetch(')) continue;
			const rel = relative(apiV1Root, file).replace(/\\/g, '/');
			expect(src, `${rel} must import resolveApiBase`).toMatch(
				/import \{ resolveApiBase \} from '\$lib\/server\/upstream';/
			);
			expect(src, `${rel} must guard on 'unavailable'`).toMatch(
				/if \('unavailable' in resolved\) return resolved\.unavailable;/
			);
		}
	});
});

// ---- Behavioral checks on a representative protected admin proxy ----

const ORIGINAL_API_INTERNAL = process.env.SIGAP_API_INTERNAL;
const fetchMock = vi.fn();

function eventFor(url: string, jwt?: string): RequestEvent {
	return {
		request: new Request(url),
		locals: jwt ? { jwt } : {}
	} as unknown as RequestEvent;
}

beforeEach(() => {
	process.env.SIGAP_API_INTERNAL = 'http://127.0.0.1:18080';
	fetchMock.mockReset();
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

describe('admin proxy misconfiguration fails closed', () => {
	it('unset base returns a 503 envelope and issues no upstream request', async () => {
		delete process.env.SIGAP_API_INTERNAL;
		const res = await GET(eventFor('http://origin/api/v1/admin/organizations'));
		expect(res.status).toBe(503);
		const body = await res.json();
		expect(body).toMatchObject({ success: false });
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it('configured base forwards the Bearer JWT from the trusted local session', async () => {
		const res = await GET(
			eventFor('http://origin/api/v1/admin/organizations', 'jwt-from-session')
		);
		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0];
		expect(String(url)).toContain('/api/v1/admin/organizations');
		expect(init.headers['Authorization']).toBe('Bearer jwt-from-session');
		expect(res.status).toBe(200);
	});

	it('an upstream 401 is passed through, never converted to success', async () => {
		fetchMock.mockImplementation(async () =>
			new Response(JSON.stringify({ success: false, error: 'unauthorized' }), { status: 401 })
		);
		const res = await GET(eventFor('http://origin/api/v1/admin/organizations'));
		expect(res.status).toBe(401);
		expect((await res.json()).success).toBe(false);
	});

	it('an upstream 500 is passed through with its status preserved', async () => {
		fetchMock.mockImplementation(async () =>
			new Response(JSON.stringify({ success: false, error: 'upstream' }), { status: 500 })
		);
		const res = await GET(eventFor('http://origin/api/v1/admin/organizations'));
		expect(res.status).toBe(500);
		expect((await res.json()).success).toBe(false);
	});
});
