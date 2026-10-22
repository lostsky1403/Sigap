import { describe, expect, it, afterEach } from 'vitest';
import { resolveApiBase } from './upstream';

/**
 * The upstream base is resolved in exactly one place.
 *
 * These cases exist because the five `/api/v1/*` proxies each used to carry
 * `process.env.SIGAP_API_INTERNAL || 'http://api:8080'`. That fallback turned a
 * missing environment variable into a transport failure, which the citizen sees
 * as "Layanan sedang bermasalah" — the same sentence a dead database produces.
 * A misconfiguration was therefore indistinguishable from an outage, and the
 * operator had nothing to go on.
 *
 * The assertion is not "it returns a URL". It is "it never invents one".
 */

const ORIGINAL = process.env.SIGAP_API_INTERNAL;

afterEach(() => {
	if (ORIGINAL === undefined) delete process.env.SIGAP_API_INTERNAL;
	else process.env.SIGAP_API_INTERNAL = ORIGINAL;
});

describe('resolveApiBase', () => {
	it('returns the configured base when it is set', () => {
		process.env.SIGAP_API_INTERNAL = 'http://127.0.0.1:18080';
		const resolved = resolveApiBase();
		expect('base' in resolved && resolved.base).toBe('http://127.0.0.1:18080');
	});

	it('refuses to guess a host when it is unset', () => {
		delete process.env.SIGAP_API_INTERNAL;
		const resolved = resolveApiBase();

		// Not a base. Nothing to connect to, and nothing invented.
		expect('base' in resolved).toBe(false);
		if (!('unavailable' in resolved)) throw new Error('expected an unavailable response');
		expect(resolved.unavailable.status).toBe(503);
	});

	it('answers with the JSON envelope the client error model already handles', async () => {
		delete process.env.SIGAP_API_INTERNAL;
		const resolved = resolveApiBase();
		if (!('unavailable' in resolved)) throw new Error('expected an unavailable response');

		// A 503 maps to `kind: 'server'` in $lib/api/errors, which the status page
		// renders as a retryable outage rather than as "Kode tidak ditemukan".
		const body = await resolved.unavailable.json();
		expect(body).toMatchObject({ success: false });
		expect(typeof body.error).toBe('string');
		expect(resolved.unavailable.headers.get('content-type')).toContain('application/json');
	});

	it('treats an empty value as unset rather than as a base', () => {
		process.env.SIGAP_API_INTERNAL = '';
		const resolved = resolveApiBase();
		expect('unavailable' in resolved).toBe(true);
	});
});
