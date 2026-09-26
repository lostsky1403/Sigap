/**
 * The single cross-cutting client store.
 *
 * What it holds is one boolean. That is not minimalism for its own sake — it is
 * the exact boundary of what the client is allowed to know.
 *
 * The server resolves identity, roles, permissions, facility scope, and the
 * super_admin flag from the database on every request (Phase 3B0). A client that
 * held any of those would be an authorization authority, and an authorization
 * authority is something a user can edit. Hiding a button is not access
 * control; the server still has to refuse. This store exists so the UI can
 * choose between "please sign in" and "you may not see this" for a 403 — and
 * that single distinction genuinely needs the session flag — while the actual
 * decision stays on the server.
 *
 * Explicitly never exposed here or by +layout.server.ts: the bearer token, the
 * user id, the role, the permission list, the facility scope, and the
 * super_admin flag. A test asserts the layout payload carries nothing beyond
 * hasSession and the pre-existing userEmail.
 */

/** A readable snapshot of session state. */
export interface SessionState {
	/**
	 * True when a session cookie is present and was valid server-side.
	 *
	 * It says nothing about privileges. `hasSession === true` with a 403 means
	 * the request was refused, not that the user should try again.
	 */
	hasSession: boolean;
}

/** Not exported as writable: a route may read the session, not grant one. */
let state: SessionState = { hasSession: false };

type Subscriber = (next: SessionState) => void;
const subscribers = new Set<Subscriber>();

function emit() {
	// Copied on the way out so a consumer cannot mutate the module state.
	const snapshot = { ...state };
	for (const subscriber of subscribers) subscriber(snapshot);
}

/**
 * Sets session state from server load data.
 *
 * Called from the root layout only. Deliberately takes the whole value rather
 * than a boolean, so an extra field would be a type error here instead of a
 * silent leak into the client bundle.
 */
export function setSession(next: SessionState): void {
	if (state.hasSession === next.hasSession) return;
	state = { hasSession: next.hasSession };
	emit();
}

/** Current snapshot. */
export function getSession(): SessionState {
	return { ...state };
}

/** Convenience accessor for the common case. */
export function hasSession(): boolean {
	return state.hasSession;
}

/**
 * Subscribes to changes and returns an unsubscribe function.
 *
 * The unsubscribe handle is required because a layout that mounts and unmounts
 * without unsubscribing would leave a listener holding a stale reference — the
 * same leak the polling helper is careful to avoid.
 */
export function subscribeToSession(subscriber: Subscriber): () => void {
	subscribers.add(subscriber);
	// Subscribe-and-catch-up: a subscriber that registers after the initial load
	// would otherwise render one frame with the wrong value.
	subscriber({ ...state });
	return () => {
		subscribers.delete(subscriber);
	};
}

/** Clears all subscribers. Test-only; a leaked listener is a bug, not state. */
export function resetSessionForTesting(): void {
	state = { hasSession: false };
	subscribers.clear();
}
