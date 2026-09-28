import { isAbort, type ApiError } from '$lib/api/errors';
import type { AdminFacility } from '$lib/api/types/api';

/**
 * Shared decisions for the admin READ layer.
 *
 * Everything here is a presentation or derivation concern. None of it widens what
 * the operator can see: the backend already scoped every list it sent, and these
 * helpers only decide how to describe that data. A helper that "helpfully"
 * refetched a wider list, or that inferred a permission from a list's length,
 * would be an authorization bug wearing a convenience hat.
 */

/**
 * The class-1 empty state: no facility is in this operator's scope at all.
 *
 * This is categorically different from a module that simply has no rows, and
 * conflating the two is the defect this constant exists to prevent.
 *
 *   - Zero authorized facilities means the operator cannot work. The backend
 *     answered successfully with an empty facility list, and every other list
 *     will be empty too. That is a configuration or grant problem, and the UI
 *     must say so rather than implying a quiet day at the clinic.
 *   - A scope that EXISTS but whose queues are empty is an ordinary, healthy
 *     state. Rendering "you have no facilities" there would tell an operator
 *     their access was revoked moments after they watched a patient arrive.
 *
 * So the distinction is drawn from the authoritative scoped-facility read, never
 * from the emptiness of the module being displayed.
 */
export const EMPTY_SCOPE_TITLE = 'Anda belum memiliki fasilitas dalam cakupan';
export const EMPTY_SCOPE_DESCRIPTION =
	'Hanya fasilitas yang diberikan kepada akun Anda yang dapat dilihat. Hubungi administrator sistem untuk menambah cakupan akses.';

/**
 * True when the operator's authorized facility scope is empty.
 *
 * Takes the facilities the backend returned for this session, never a count of
 * some other list. `undefined` (still loading) is deliberately NOT scope-empty:
 * rendering the class-1 state before the request answers would flash "no
 * facilities" at an operator who has them.
 */
export function isEmptyScope(facilities: readonly AdminFacility[] | undefined): boolean {
	return Array.isArray(facilities) && facilities.length === 0;
}

/**
 * The ordinary empty state for a module that has a scope but no rows.
 *
 * Deliberately worded as a fact about the list rather than about the operator,
 * because the operator's access is fine — there is simply nothing in it yet.
 */
export const ORDINARY_EMPTY_DESCRIPTION =
	'Belum ada data dalam lingkup akses Anda saat ini.';

/**
 * A load state for an admin list.
 *
 * `error` is normalized to `null` when it was an abort, because an abort is the
 * app cancelling its own request — navigating away, or a poll superseded by a
 * manual refresh. Surfacing it would show a failure to someone who did nothing
 * wrong and who is no longer looking at the page.
 */
export interface AdminLoad<T> {
	loading: boolean;
	failed: boolean;
	error: ApiError | null;
	rows: T[];
}

/** The state before any request has been made. */
export function initialLoad<T>(): AdminLoad<T> {
	return { loading: true, failed: false, error: null, rows: [] };
}

/**
 * The state after a successful read.
 *
 * `rows` is copied rather than aliased so a later mutation of the caller's array
 * cannot retroactively change what the page rendered — a polling board holds onto
 * its last successful rows, and aliasing would let a filtered view quietly
 * rewrite them.
 */
export function loadedState<T>(rows: readonly T[]): AdminLoad<T> {
	return { loading: false, failed: false, error: null, rows: [...rows] };
}

/**
 * The state after a failed FIRST load, which replaces the page.
 *
 * Distinct from a failed refresh, which keeps the last good rows and marks them
 * stale. Collapsing the two would either blank a working board on one flaky
 * request or leave an operator staring at an empty page that is quietly broken.
 */
export function failedState<T>(error: ApiError): AdminLoad<T> {
	if (isAbort(error)) return initialLoad<T>();
	return { loading: false, failed: true, error, rows: [] };
}

/**
 * True when a refresh failed while rows are already on screen.
 *
 * The stale banner is shown on this condition, and the rows are kept. Blanking
 * a queue board because one 30-second poll failed would destroy the one thing
 * the operator is looking at, in order to display a tidier error.
 */
export function isStaleData(state: AdminLoad<unknown>, error: ApiError | null): boolean {
	return error !== null && !isAbort(error) && state.rows.length > 0;
}

/**
 * True when the page is waiting on a request that has never resolved.
 *
 * Distinct from "has rows and is refreshing": the first justifies a skeleton,
 * the second does not, because replacing visible rows with a skeleton on every
 * poll would make the board unreadable.
 */
export function isFirstLoad(state: AdminLoad<unknown>): boolean {
	return state.loading && state.rows.length === 0;
}

/** "Diperbarui pukul 14.05" — the freshness every scoped admin read must state. */
export function formatUpdatedAt(when: Date | null): string {
	if (!when) return '';
	const hours = String(when.getHours()).padStart(2, '0');
	const minutes = String(when.getMinutes()).padStart(2, '0');
	return `Diperbarui pukul ${hours}.${minutes}`;
}

/** The clock half of the freshness label, e.g. "10.15". */
export function formatClock(when: Date | null): string {
	if (!when) return '-';
	const hours = String(when.getHours()).padStart(2, '0');
	const minutes = String(when.getMinutes()).padStart(2, '0');
	return `${hours}.${minutes}`;
}
