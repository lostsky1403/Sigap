/**
 * Normalized API error model.
 *
 * Every failure the frontend can experience collapses into one of these kinds,
 * so a route never has to branch on a status code or guess at a message. Two
 * rules make this model worth having:
 *
 * 1. The kind is derived from the HTTP status plus transport facts, never from
 *    message text. Matching on strings is how "something went wrong" quietly
 *    becomes a hardcoded Indonesian sentence in three places.
 *
 * 2. `kind` describes the *transport* outcome. Whether a 403 means "log in" or
 *    "you may not do this" is a presentation decision, and it is decided by the
 *    caller using `hasSession` — never by the client inferring a role.
 */

export type ApiErrorKind =
	| 'bad_request'
	| 'unauthorized'
	| 'forbidden'
	| 'not_found'
	| 'conflict'
	| 'rate_limited'
	| 'server'
	| 'network'
	| 'aborted'
	| 'unknown';

/**
 * A failure, always returned rather than thrown.
 *
 * Returning instead of throwing is a deliberate ergonomic choice: a route that
 * renders a data table has to handle "the request failed" on every path, and a
 * result type makes that path impossible to forget. It also keeps abort from
 * turning into an unhandled rejection during navigation.
 */
export interface ApiError {
	kind: ApiErrorKind;
	/** HTTP status, or 0 for a transport failure that never reached the server. */
	status: number;
	/** Message from the backend when it sent one, otherwise a neutral fallback. */
	message: string;
	/** Field-level validation detail, when the backend supplied it. */
	fields?: Record<string, string>;
	/**
	 * Raw response body, kept for logging and for the rare case a route needs a
	 * code the normalized model does not carry. Never rendered directly.
	 */
	body?: unknown;
}

/** True when the failure was an abort, which callers should treat as a no-op. */
export function isAbort(error: ApiError): boolean {
	return error.kind === 'aborted';
}

/** True for failures caused by the network or transport, not by the server. */
export function isTransport(error: ApiError): boolean {
	return error.kind === 'network' || error.kind === 'aborted';
}

/**
 * True when the user has to authenticate.
 *
 * This is the 403-with-no-session case from the admin auth layer, where missing
 * authentication and insufficient permission deliberately share a status code.
 * The distinction is only available to the frontend through the session flag,
 * which is why presentation is decided here and nowhere else.
 */
export function requiresAuth(error: ApiError, hasSession: boolean): boolean {
	if (error.kind === 'unauthorized') return true;
	// No session plus 403 means the server would not tell us more: present the
	// sign-in path. A session plus 403 is a real permission refusal.
	return error.kind === 'forbidden' && !hasSession;
}

/**
 * True when the session exists but the action is not permitted.
 *
 * Deliberately requires `hasSession`. The client has no role, no permission
 * list, and no facility scope, so it cannot and must not decide this on its own.
 */
export function isPermissionDenied(error: ApiError, hasSession: boolean): boolean {
	return error.kind === 'forbidden' && hasSession;
}

/** True when the request never completed, so retrying is reasonable. */
export function isRetryable(error: ApiError): boolean {
	return error.kind === 'network' || error.kind === 'server' || error.kind === 'rate_limited';
}

/** Human-readable text for the given session state, used by the state panels. */
export function describe(error: ApiError, hasSession: boolean): string {
	if (error.message) return error.message;
	switch (error.kind) {
		case 'bad_request':
			return 'Permintaan tidak valid.';
		case 'conflict':
			return 'Data sudah berubah. Muat ulang halaman.';
		case 'rate_limited':
			return 'Terlalu banyak permintaan. Coba lagi sebentar lagi.';
		case 'server':
			return 'Layanan sedang bermasalah. Coba lagi nanti.';
		case 'network':
			return 'Tidak dapat terhubung ke server. Periksa koneksi Anda.';
		default:
			return 'Terjadi kesalahan. Coba lagi.';
	}
}

/** Maps an HTTP status onto the error kind. Extracted so it can be tested alone. */
export function kindForStatus(status: number): ApiErrorKind {
	if (status === 400) return 'bad_request';
	if (status === 401) return 'unauthorized';
	if (status === 403) return 'forbidden';
	if (status === 404) return 'not_found';
	if (status === 409) return 'conflict';
	if (status === 429) return 'rate_limited';
	if (status >= 500) return 'server';
	return 'unknown';
}
