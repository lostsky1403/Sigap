import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
	getSession,
	hasSession,
	resetSessionForTesting,
	setSession,
	subscribeToSession
} from './session';

/**
 * Session exposure contract (Phase 3B1 section 18).
 *
 * These assertions are the security boundary of this phase, not a convenience
 * test. The failure they prevent is subtle and serious: a client that learns it
 * is, say, a super_admin or holds a facility scope becomes an authorization
 * authority, and an authorization authority is something a user can edit with
 * devtools. The server must remain the only place that decides.
 *
 * That is why the source of +layout.server.ts is parsed directly. A test that
 * only called the load function would pass even if a future edit added a
 * forbidden field that the test's fake `locals` happened not to populate.
 */

const here = dirname(fileURLToPath(import.meta.url));
const layoutServerSource = readFileSync(
	join(here, '..', '..', 'routes', '+layout.server.ts'),
	'utf8'
);

/**
 * Strips comments before scanning.
 *
 * The layout file documents the very fields it must never expose, so a naive
 * substring scan would flag its own warning as a violation. Stripping comments
 * keeps the check honest: it still fails on any real property, and stops
 * failing on the note that explains why the property is absent.
 */
const layoutServerCode = layoutServerSource
	.replace(/\/\*[\s\S]*?\*\//g, '')
	.replace(/^\s*\/\/.*$/gm, '');

beforeEach(() => {
	resetSessionForTesting();
});

describe('layout server exposure', () => {
	it('exposes only hasSession and the pre-existing userEmail', () => {
		// Every key defined anywhere in the module's returned object, not just
		// the first `return {`. Matching a single block would silently pass if a
		// later field were added, which is exactly the regression this guards.
		// Verified by negative control: injecting `permissions: []` turns this test
		// and the forbidden-key test below red.
		//
		// The value class is non-greedy on purpose. A greedy one would let the
		// first property match run past the following ones and capture them all
		// under a single name, which silently hides every later key.
		const keys = [
			...layoutServerCode.matchAll(
				/(?:^|[;{,\s])([A-Za-z_][A-Za-z0-9_]*)\s*:\s*[^;{}]*?[,}]/g
			)
		].map((m) => m[1]);

		const allowed = new Set(['hasSession', 'userEmail']);
		const unexpected = keys.filter((k) => !allowed.has(k));
		expect(unexpected).toEqual([]);
		expect(keys).toContain('hasSession');
		expect(keys).toContain('userEmail');
	});

	it('exposes no token, role, permission, or facility scope', () => {
		// The forbidden set, spelled out. Each of these has been a real class of
		// frontend authorization bug.
		const forbidden = [
			'access_token',
			'accessToken',
			'refresh_token',
			'refreshToken',
			'bearer',
			'authorization',
			'role',
			'roles',
			'permissions',
			'permission_keys',
			'facility_scope',
			'facilityScope',
			'facility_ids',
			'facilityIds',
			'super_admin',
			'superAdmin',
			'is_super_admin',
			'user_id',
			'userId',
			'app_user_id'
		];

		for (const key of forbidden) {
			// Matched as an object key or a shorthand property, case-insensitively.
			const pattern = new RegExp(`(^|[\\s{,])${key}\\s*[:,}]`, 'im');
			expect(
				pattern.test(layoutServerCode),
				`+layout.server.ts must not expose ${key}`
			).toBe(false);
		}
	});

	it('derives hasSession from the session rather than from anything client-supplied', () => {
		expect(layoutServerCode).toMatch(/hasSession:\s*Boolean\(locals\.session/);
		// A hasSession that could be overridden by a query parameter would let a
		// caller claim a session they do not have and reach the wrong 403 branch.
		expect(layoutServerCode).not.toMatch(/url\.searchParams/);
	});
});

describe('session store', () => {
	it('defaults to no session', () => {
		expect(hasSession()).toBe(false);
		expect(getSession()).toEqual({ hasSession: false });
	});

	it('holds exactly one boolean', () => {
		setSession({ hasSession: true });
		expect(hasSession()).toBe(true);
		// Nothing else is derivable, so nothing else can leak.
		expect(Object.keys(getSession()).sort()).toEqual(['hasSession']);
	});

	it('returns a copy so a consumer cannot mutate module state', () => {
		setSession({ hasSession: true });
		const snapshot = getSession();
		snapshot.hasSession = false;
		expect(hasSession()).toBe(true);
	});

	it('notifies subscribers on change', () => {
		const seen: boolean[] = [];
		const unsubscribe = subscribeToSession((state) => seen.push(state.hasSession));

		setSession({ hasSession: true });
		setSession({ hasSession: false });
		unsubscribe();

		// Catch-up on subscribe, then one call per real change.
		expect(seen).toEqual([false, true, false]);
	});

	it('does not notify when nothing changed', () => {
		setSession({ hasSession: true });
		const subscriber = vi.fn();
		const unsubscribe = subscribeToSession(subscriber);
		expect(subscriber).toHaveBeenCalledTimes(1); // catch-up

		setSession({ hasSession: true });
		expect(subscriber).toHaveBeenCalledTimes(1);

		unsubscribe();
	});

	it('stops notifying after unsubscribe', () => {
		const subscriber = vi.fn();
		subscribeToSession(subscriber)();
		setSession({ hasSession: true });
		// A leaked listener would keep a stale reference alive and re-render
		// components that no longer exist.
		expect(subscriber).toHaveBeenCalledTimes(1);
	});
});
