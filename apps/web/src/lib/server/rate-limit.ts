// apps/web/src/lib/server/rate-limit.ts
//
// Minimal in-process brute-force throttle for the SIGAP form-auth path. Better
// Auth's own `rateLimit` covers its `/api/auth/*` HTTP endpoints via
// `auth.handler`, but a SvelteKit server action calling `auth.api.signInEmail`
// directly bypasses that limiter — so this module guards the form path with a
// second, independent fixed-window limiter keyed on the client IP.
//
// It is deliberately dumb and stateless-held-in-memory: a single web process
// in the local/dev topology. Production scale would key off a shared store, but
// the mission forbids adding new dependencies for a few lines, and the correct
// fail direction is to REFUSE on a saturated window rather than silently allow.

export interface RateLimiter {
	allow(key: string, now?: number): boolean;
}

const WINDOW_MS = 60_000;
const MAX_PER_WINDOW = 10;

function newInMemoryLimiter(windowMs: number, max: number): RateLimiter {
	const hits = new Map<string, number[]>();
	return {
		allow(key: string, now = Date.now()): boolean {
			const cutoff = now - windowMs;
			const arr = (hits.get(key) ?? []).filter((t) => t >= cutoff);
			if (arr.length >= max) {
				hits.set(key, arr);
				return false;
			}
			arr.push(now);
			hits.set(key, arr);
			return true;
		}
	};
}

const loginLimiter = newInMemoryLimiter(WINDOW_MS, MAX_PER_WINDOW);
const registerLimiter = newInMemoryLimiter(WINDOW_MS, MAX_PER_WINDOW);

/** Allowed when the client IP has not saturated the window (per-action bucket). */
export function allowLogin(ip: string): boolean {
	return loginLimiter.allow(`login:${ip}`);
}

export function allowRegister(ip: string): boolean {
	return registerLimiter.allow(`register:${ip}`);
}