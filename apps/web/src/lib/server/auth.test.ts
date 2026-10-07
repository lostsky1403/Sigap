import { describe, expect, it, beforeEach, afterEach } from 'vitest';
import { localE2eActorHeader, proxyHeaders } from './auth';

/**
 * T-3B5 §2 — the local DB-backed actor selector, and why the browser cannot
 * become one by itself.
 *
 * The Phase 3B5 spec requires a proof BEFORE any mutation E2E: that the seeded
 * d993/d994 actors can be exercised through the real browser chain, and that a
 * browser cannot reach the same capability on its own.
 *
 * These cases are the half of that proof that belongs in the unit suite, and
 * they are security assertions, not plumbing assertions. Each one describes an
 * attack that would succeed if the code changed a particular way.
 */

const ORIGINAL_ENV = { ...process.env };

function setEnv(env: Record<string, string | undefined>) {
	for (const [key, value] of Object.entries(env)) {
		if (value === undefined) delete process.env[key];
		else process.env[key] = value;
	}
}

beforeEach(() => {
	setEnv({
		SIGAP_ENV: undefined,
		SIGAP_DEV_IDENTITY: undefined,
		SIGAP_LOCAL_E2E_ACTOR: undefined
	});
});

afterEach(() => {
	setEnv(ORIGINAL_ENV);
});

describe('proxyHeaders — the credential boundary', () => {
	it('injects nothing at all when dev identity is disabled', () => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_DEV_IDENTITY: 'false' });
		expect(proxyHeaders()).toEqual({});
	});

	it('injects nothing when dev identity is merely unset', () => {
		setEnv({ SIGAP_ENV: 'local' });
		expect(proxyHeaders()).toEqual({});
	});

	/**
	 * SECURITY (upstream main, vuln-0002 follow-up): the browser NEVER emits
	 * X-Sigap-Dev-User-ID, not even under SIGAP_ENV=local with dev identity
	 * enabled. A client-set header must carry no authentication meaning, so the
	 * web tier does not produce one at all.
	 */
	it('never injects a dev identity, even under SIGAP_ENV=local with it enabled', () => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_DEV_IDENTITY: 'true' });
		expect(proxyHeaders()).toEqual({});
	});

	it('never injects a dev identity outside local either', () => {
		setEnv({ SIGAP_ENV: 'staging', SIGAP_DEV_IDENTITY: 'true' });
		expect(proxyHeaders()).toEqual({});
	});

	/**
	 * The load-bearing property for §2.
	 *
	 * The signature takes no arguments, and that is the whole mechanism. There
	 * is no `event`, no `request`, and no way to pass a caller-supplied value
	 * in — so a request header arriving from the browser cannot reach the
	 * upstream identity, not because it is filtered, but because there is no
	 * code path that would read it.
	 *
	 * This is asserted as a signature fact rather than as a behaviour test
	 * because a behaviour test could only prove the CURRENT implementation
	 * ignores the header. The signature makes it impossible for a future
	 * implementation to start honouring it without also changing this test.
	 */
	it('takes no request argument, so no browser header can reach the upstream identity', () => {
		expect(proxyHeaders.length).toBe(0);
	});

	/**
	 * The dev identity is NEVER emitted. A browser cannot become an identity by
	 * naming one, and it cannot fall back to a fixed one either.
	 */
	it('always injects nothing, regardless of how many times it is called', () => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_DEV_IDENTITY: 'true' });
		const first = proxyHeaders();
		const second = proxyHeaders();
		expect(first).toEqual({});
		expect(second).toEqual({});
		expect(first).not.toHaveProperty('X-Sigap-Dev-User-ID');
	});

	/**
	 * The proxy must not grow a pass-through.
	 *
	 * A plausible future regression is "let the E2E pick an actor": someone adds
	 * a parameter that forwards a header, and local E2E starts working. That
	 * change would also make a browser-supplied `X-Sigap-*` header effective
	 * against any non-local deployment that has dev identity on. So the source
	 * is asserted directly — a behavioural test cannot see a parameter that is
	 * currently unused, and an unused parameter is precisely the risk.
	 */
	it('does not read any request header, so no X-Sigap-* pass-through exists', async () => {
		// Resolved from process.cwd() (apps/web) rather than import.meta.url:
		// under the jsdom environment `import.meta.url` is an http URL, which
		// node:fs rejects with "The URL must be of scheme file".
		//
		// COMMENTS ARE STRIPPED FIRST. This file documents the rejected
		// pass-through by name, so a raw regex over the source would match its
		// own explanation of the attack — and would then pass or fail for a
		// reason that has nothing to do with the code.
		const { readFile } = await import('node:fs/promises');
		const source = await readFile('src/lib/server/auth.ts', 'utf8');
		const code = source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');

		expect(code).not.toMatch(/event\.request\.headers/);
		expect(code).not.toMatch(/get\(\s*['"`]x-sigap/i);
		// The only X-Sigap-* literals in executable code may be headers the
		// server itself produces, never ones read from a caller. Since the
		// upstream security fix, the browser emits ONLY the local actor header.
		const emitted = [...code.matchAll(/['"`](X-Sigap-[A-Za-z-]+)['"`]/g)].map((m) => m[1]);
		expect(new Set(emitted)).toEqual(new Set(['X-Sigap-Local-Test-Subject']));
	});
});

/**
 * T-3B5 §2: the local DB-backed actor selector.
 *
 * These are the cases that decide whether local E2E can run as a real
 * RBAC-resolved actor at all, and — more importantly — the cases that keep that
 * capability from becoming a way for a browser to choose who it is.
 */
describe('localE2eActorHeader — local-only, process-fixed identity', () => {
	it('selects the configured subject under SIGAP_ENV=local', () => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_LOCAL_E2E_ACTOR: 'e2e-schedule-manager' });
		expect(localE2eActorHeader()).toEqual({
			'X-Sigap-Local-Test-Subject': 'e2e-schedule-manager'
		});
	});

	/**
	 * GATE 1. Outside local, the selector produces nothing at all — the value is
	 * not passed through "but ignored downstream", it is never emitted.
	 */
	it.each(['staging', 'production', 'dev', 'test', ''])(
		'refuses to select an actor when SIGAP_ENV=%o',
		(env) => {
			setEnv({ SIGAP_ENV: env, SIGAP_LOCAL_E2E_ACTOR: 'e2e-schedule-manager' });
			expect(localE2eActorHeader()).toEqual({});
		}
	);

	it('refuses when SIGAP_ENV is unset entirely', () => {
		setEnv({ SIGAP_ENV: undefined, SIGAP_LOCAL_E2E_ACTOR: 'e2e-schedule-manager' });
		expect(localE2eActorHeader()).toEqual({});
	});

	/**
	 * A deliberate asymmetry with the Go provider, pinned so it cannot change
	 * silently.
	 *
	 * `localE2eActorHeader` lowercases, so `SIGAP_ENV=LOCAL` passes gate 1 here
	 * and the header IS emitted — while the Go provider compares exactly
	 * (`os.Getenv("SIGAP_ENV") == "local"`) and treats it as NOT local, so it
	 * would ignore the header and answer with a zero actor.
	 *
	 * The end result is fail-closed: the stricter tier wins, and the outcome is
	 * an unauthenticated request rather than unintended access. That is the safe
	 * direction, so the looser web-side check is left alone rather than being
	 * tightened for symmetry — and this case exists so that a future change to
	 * EITHER side is a visible decision instead of an accident.
	 */
	it('emits the header for LOCAL, which Go then ignores — fail-closed, not open', () => {
		setEnv({ SIGAP_ENV: 'LOCAL', SIGAP_LOCAL_E2E_ACTOR: 'e2e-schedule-manager' });
		expect(localE2eActorHeader()).toEqual({
			'X-Sigap-Local-Test-Subject': 'e2e-schedule-manager'
		});
	});

	it.each(['', '   '])('emits nothing for a blank actor value %o', (actor) => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_LOCAL_E2E_ACTOR: actor });
		expect(localE2eActorHeader()).toEqual({});
	});

	it('emits nothing when the actor variable is unset', () => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_LOCAL_E2E_ACTOR: undefined });
		expect(localE2eActorHeader()).toEqual({});
	});

	it('trims surrounding whitespace off the subject', () => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_LOCAL_E2E_ACTOR: '  e2e-schedule-mixed  ' });
		expect(localE2eActorHeader()).toEqual({
			'X-Sigap-Local-Test-Subject': 'e2e-schedule-mixed'
		});
	});

	/**
	 * GATE 3, and the one that matters most.
	 *
	 * The header names an identity and grants nothing. Whatever subject is
	 * named, the Go API resolves that subject's permissions and facility grants
	 * from the database, and an unseeded subject resolves to zero permissions —
	 * so an unknown subject here produces a header that leads to a refused
	 * request, never to access. Asserting the pass-through makes that explicit:
	 * the selector does not validate, and deliberately so, because validation
	 * here would duplicate the RBAC resolver and could only ever be weaker.
	 */
	it('passes any subject through verbatim, granting nothing itself', () => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_LOCAL_E2E_ACTOR: 'not-a-seeded-subject' });
		expect(localE2eActorHeader()).toEqual({
			'X-Sigap-Local-Test-Subject': 'not-a-seeded-subject'
		});
	});
});

describe('proxyHeaders — local actor takes precedence over dev identity', () => {
	/**
	 * Mirrors `cmd/server/main.go`: the armed local provider REPLACES the dev
	 * provider, so a request carrying only the dev header would authenticate
	 * nobody. Asserting the web tier matches is what keeps the E2E stack from
	 * failing with an unexplained 401 on every admin read.
	 */
	it('sends only the local subject when one is configured', () => {
		setEnv({
			SIGAP_ENV: 'local',
			SIGAP_DEV_IDENTITY: 'true',
			SIGAP_LOCAL_E2E_ACTOR: 'e2e-schedule-manager'
		});
		expect(proxyHeaders()).toEqual({
			'X-Sigap-Local-Test-Subject': 'e2e-schedule-manager'
		});
	});

	it('never sends both headers, because Go honours only the subject one', () => {
		setEnv({
			SIGAP_ENV: 'local',
			SIGAP_DEV_IDENTITY: 'true',
			SIGAP_LOCAL_E2E_ACTOR: 'e2e-schedule-mixed'
		});
		const headers = proxyHeaders();
		expect(headers).not.toHaveProperty('X-Sigap-Dev-User-ID');
		expect(headers).toHaveProperty('X-Sigap-Local-Test-Subject');
	});

	/**
	 * SECURITY: with no local actor configured there is NO dev-identity
	 * fallback. The web tier emits nothing; a dev identity is a server-side
	 * tool only.
	 */
	it('does not fall back to a dev identity when no local actor is configured', () => {
		setEnv({ SIGAP_ENV: 'local', SIGAP_DEV_IDENTITY: 'true', SIGAP_LOCAL_E2E_ACTOR: undefined });
		expect(proxyHeaders()).toEqual({});
	});

	/**
	 * A local actor configured OUTSIDE local must not silently fall through to
	 * anything: it produces no header, and no dev header is emitted either.
	 */
	it('emits nothing when a local actor is configured outside local', () => {
		setEnv({
			SIGAP_ENV: 'production',
			SIGAP_DEV_IDENTITY: 'true',
			SIGAP_LOCAL_E2E_ACTOR: 'e2e-schedule-manager'
		});
		expect(proxyHeaders()).toEqual({});
	});
});
