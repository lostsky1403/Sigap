import type { ApiError } from '$lib/api/errors';
import { mutationFailure, type MutationFailure } from './mutationFailure';
import { announce } from './announcer';
import { pushToast } from './toastStore';

/**
 * The shared shape of an admin mutation, and the one place its result is
 * interpreted.
 *
 * WHY A CONTROLLER AND NOT FOUR INLINE HANDLERS. Every admin mutation has the
 * same four obligations, and getting any of them wrong in one place while three
 * others are correct is the normal way this goes wrong:
 *
 *   1. Send the request through the shared client. Never a bare `fetch` — that
 *      is the Phase 3B1 anti-pattern, and reintroducing it would put a
 *      credential path back into page code.
 *   2. On success: reload the affected data, announce it politely, and raise a
 *      toast. All three, because they serve three different consumers — a
 *      sighted operator, a screen-reader user, and someone who looked away.
 *   3. On failure: show the BACKEND's message verbatim. Never a generic
 *      substitute, and never a "conflict" rewording when the server said
 *      something specific.
 *   4. Never blank the table because a mutation failed. The rows are the last
 *      thing that was true.
 *
 * Centralising those four is what makes them consistent. The per-surface code
 * supplies only what genuinely differs: the request, and the wording of the
 * success message.
 */
export interface MutationOutcome {
	ok: boolean;
	/** The backend's message on failure. Empty on success. */
	message: string;
	/** Populated on failure only. */
	failure: MutationFailure | null;
	/** The raw error, for callers that need the status. */
	error: ApiError | null;
}

export interface MutationContext {
	/**
	 * The subject named in the success message — a queue number, a patient name.
	 * Used by the caller's own success wording, never for authorization.
	 */
	subject: string;
	/** Human label for the state being left, e.g. "Menunggu". */
	fromLabel: string;
	/** Human label for the state being entered, e.g. "Dipanggil". */
	toLabel: string;
	/** What the operator is acting on, e.g. "Antrean" or "Janji temu". */
	noun: string;
	/**
	 * Re-reads the affected data after a successful mutation. Required, not
	 * optional: an optimistic local patch would drift from the server's
	 * timestamps (`called_at`, `updated_at`), and the next poll would snap the
	 * row back, which reads as the mutation having failed.
	 */
	reload: () => void | Promise<void>;
}

/**
 * Runs one admin mutation and reports the outcome through every channel.
 *
 * Returns the outcome so the caller can react to the specific case it cares
 * about — a 403 needs a panel, a 409 needs its message shown inline. The
 * generic channels (toast, announcement) are fired here because they are
 * identical for every mutation and are what makes a mutation feel acknowledged
 * when it succeeds.
 */
export async function runAdminMutation(
	perform: () => Promise<{ ok: true } | { ok: false; error: ApiError }>,
	context: MutationContext,
	hasSession: boolean
): Promise<MutationOutcome> {
	const result = await perform();

	if (result.ok) {
		// Reload FIRST. If the reload fails, the operator should see stale rows
		// with a toast claiming success, rather than a fresh error page that
		// makes a successful mutation look like a failed one.
		await context.reload();

		const message = `${context.noun} ${context.subject} dipindahkan dari ${context.fromLabel} ke ${context.toLabel}.`;
		// The announcement is what a screen-reader user receives; the toast is
		// what everyone else sees. Emitting only one leaves somebody uninformed.
		announce(message);
		pushToast(message, { title: 'Perubahan tersimpan' });
		return { ok: true, message, failure: null, error: null };
	}

	// A failure is NOT toasted. It is rendered inline next to the control that
	// failed, where the operator can read it and do something about it. A toast
	// that disappears takes the explanation with it, and the backend's message
	// is specific enough to be worth keeping on screen.
	return {
		ok: false,
		message: '',
		failure: mutationFailure(result.error, hasSession),
		error: result.error
	};
}
