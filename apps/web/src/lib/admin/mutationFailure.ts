import type { ApiError, ApiErrorKind } from '$lib/api/errors';

/**
 * How an admin mutation failure is presented.
 *
 * WHY THE BACKEND'S WORDS WIN. The server knows the specific rule that was
 * broken and has already written a message naming it — "Transisi status
 * 'checked_in' → 'completed' tidak diizinkan." Replacing that with a generic
 * "Permintaan tidak valid" would tell the operator nothing they can act on, and
 * worse, it would hide WHICH rule they hit. `describe()` in errors.ts is the
 * right fallback for a failure the backend did not describe, and the wrong one
 * for a 400 that arrived with a sentence attached.
 *
 * So: use the backend message whenever there is one, and fall back only when
 * there is not. That is the whole policy, and it is why this module exists
 * rather than each page doing `error.message || '...'` — four pages doing that
 * independently is how one of them ends up inventing its own wording.
 */
export interface MutationFailure {
	kind: ApiErrorKind;
	/** The message to show, already chosen. Never a generic substitute. */
	message: string;
	/**
	 * True when the backend supplied this text. Distinguishing matters for the
	 * inline presentation: a described failure is confidently displayable, while
	 * a bare transport failure should read as a retry invitation.
	 */
	described: boolean;
}

/**
 * Builds the presentation for a failed admin mutation.
 *
 * `hasSession` is required for the same reason `ErrorState` takes it: a 403
 * means "sign in" without a session and "not permitted" with one, and the two
 * are indistinguishable from the status code alone.
 */
export function mutationFailure(error: ApiError, hasSession: boolean): MutationFailure {
	// The backend's own words, when it sent any. `error.message` is populated
	// from the envelope's `error` field, so this is the real server text and not
	// a client-side string.
	if (error.message && error.message.trim().length > 0) {
		return { kind: error.kind, message: error.message, described: true };
	}
	return {
		kind: error.kind,
		message: FALLBACK_MESSAGE[error.kind] ?? FALLBACK_MESSAGE.unknown,
		described: false
	};
}

/**
 * Fallbacks for failures the backend did not describe.
 *
 * These are deliberately generic because they are the last resort. A 400 with
 * no body is a server bug, not a user error, so the wording says "something went
 * wrong" rather than blaming the operator's input.
 */
const FALLBACK_MESSAGE: Record<ApiErrorKind, string> = {
	bad_request: 'Permintaan tidak dapat diproses.',
	unauthorized: 'Sesi tidak valid. Silakan masuk kembali.',
	forbidden: 'Anda tidak memiliki izin untuk tindakan ini.',
	not_found: 'Data tidak ditemukan atau di luar lingkup akses Anda.',
	conflict: 'Data sudah berubah. Muat ulang halaman.',
	rate_limited: 'Terlalu banyak permintaan. Coba lagi sebentar lagi.',
	server: 'Layanan sedang bermasalah. Coba lagi nanti.',
	network: 'Tidak dapat terhubung ke server. Periksa koneksi Anda.',
	aborted: 'Permintaan dibatalkan.',
	unknown: 'Terjadi kesalahan. Coba lagi.'
};

/**
 * True when the failure is an authorization refusal rather than a validation
 * problem.
 *
 * Used to decide between the two panels. It deliberately does NOT distinguish
 * WHICH permission was missing — the browser was never told, and inventing that
 * distinction would mean the client had started guessing at the authorization
 * model, which is the thing Phase 3B5.0 exists to prevent.
 */
export function isAuthorizationFailure(failure: MutationFailure, hasSession: boolean): boolean {
	return failure.kind === 'forbidden' || (failure.kind === 'unauthorized' && !hasSession);
}
