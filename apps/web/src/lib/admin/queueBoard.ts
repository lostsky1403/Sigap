import { createPolling, type PollingController } from '$lib/domain/polling';
import type { ApiResult } from '$lib/api/client';
import { isAbort, type ApiError } from '$lib/api/errors';
import { formatUpdatedAt, type AdminLoad } from './readState';
import type { AdminQueueTicket } from '$lib/api/types/api';

/**
 * The queue board's polling state machine.
 *
 * The lifecycle mechanics already live in `createPolling` and are tested there.
 * What is NOT tested there, and what this module owns, is the POLICY:
 *
 *  1. A failed FIRST load replaces the page. There is nothing to show, so an
 *     ErrorState is the honest rendering.
 *  2. A failed REFRESH keeps the last good rows and marks them stale. This is
 *     the single most important behaviour on the page: blanking a queue board
 *     because one 30-second poll failed would destroy the one thing the operator
 *     is looking at, in order to display a tidier error.
 *  3. The freshness label advances only on a SUCCESSFUL load. Printing the
 *     current time on a failed poll would claim the data is fresh when it is
 *     not — the exact lie the label exists to prevent.
 *
 * Kept as a plain factory rather than inside the component so the policy is
 * testable with fake timers and no DOM, and so the component stays a
 * presentation concern.
 */

/** What the board renders. */
export interface QueueBoardState {
	load: AdminLoad<AdminQueueTicket>;
	/** When the last SUCCESSFUL load finished. Null until one does. */
	lastUpdated: Date | null;
	/** True when a refresh failed but rows are still on screen. */
	stale: boolean;
	refreshing: boolean;
}

export interface QueueBoardSource {
	/** Reads the scoped ticket list. Must honour the supplied signal. */
	read: (signal: AbortSignal) => Promise<ApiResult<AdminQueueTicket[]>>;
}

export interface QueueBoardController {
	state: QueueBoardState;
	subscribe: (listener: (next: QueueBoardState) => void) => () => void;
	start: () => void;
	stop: () => void;
	/** The manual refresh. Subject to the same overlap guard as a poll. */
	refresh: () => Promise<void>;
	polling: PollingController;
}

/**
 * Wires a polling controller to the board's state.
 *
 * `facilities` is loaded once, separately, and NOT polled. Facility scope changes
 * when an administrator changes a grant, which is rare; polling it every thirty
 * seconds would add a request per board for data that is also embedded in the
 * ticket rows' own facility references.
 */
export function createQueueBoard(source: QueueBoardSource): QueueBoardController {
	let state: QueueBoardState = {
		load: { loading: true, failed: false, error: null, rows: [] },
		lastUpdated: null,
		stale: false,
		refreshing: false
	};

	const listeners = new Set<(next: QueueBoardState) => void>();

	function update(patch: Partial<QueueBoardState>) {
		state = { ...state, ...patch };
		// Copied on the way out so a consumer cannot mutate module state, mirroring
		// the session store's own rule.
		const snapshot: QueueBoardState = { ...state, load: { ...state.load } };
		for (const listener of listeners) listener(snapshot);
	}

	const polling = createPolling<ApiResult<AdminQueueTicket[]>>({
		fetch: (signal) => source.read(signal),
		onData: (result) => {
			if (result.ok) {
				update({
					load: { loading: false, failed: false, error: null, rows: [...result.data] },
					lastUpdated: new Date(),
					// A successful load clears staleness. The rows on screen are now
					// the rows the server just sent.
					stale: false,
					refreshing: false
				});
				return;
			}
			handleFailure(result.error);
		},
		onError: () => {
			// The fetcher resolves with a normalized ApiResult rather than throwing,
			// so this path is only for an unexpected throw. Treated as a failure with
			// no ApiError, which still preserves rows.
			handleFailure({ kind: 'network', status: 0, message: '' });
		},
		intervalMs: 30_000,
		immediate: true
	});

	function handleFailure(error: ApiError) {
		// An abort is the app cancelling its own request, so it produces no state
		// change at all. Showing a failure to someone who navigated away trains
		// people to ignore error states.
		if (isAbort(error)) {
			update({ refreshing: false });
			return;
		}

		if (state.load.rows.length > 0) {
			// STALE, NOT BLANK. The rows stay, the error is remembered, and the
			// freshness label is deliberately NOT advanced.
			update({ stale: true, refreshing: false, load: { ...state.load, error } });
			return;
		}

		update({
			load: { loading: false, failed: true, error, rows: [] },
			stale: false,
			refreshing: false
		});
	}

	return {
		get state() {
			return state;
		},
		subscribe(listener) {
			listeners.add(listener);
			listener({ ...state, load: { ...state.load } });
			return () => listeners.delete(listener);
		},
		start: () => {
			update({ refreshing: true });
			polling.start();
		},
		stop: () => polling.stop(),
		refresh: async () => {
			update({ refreshing: true });
			await polling.refresh();
		},
		polling
	};
}

/**
 * The freshness string for the header, or an empty string before any success.
 *
 * Empty rather than a placeholder: a page that has never loaded has no update
 * time, and printing "00.00" would assert a freshness it cannot support.
 */
export function boardUpdatedLabel(state: QueueBoardState): string {
	return formatUpdatedAt(state.lastUpdated);
}

/** True when the board should show the stale banner. */
export function showsStaleWarning(state: QueueBoardState): boolean {
	return state.stale && state.load.rows.length > 0;
}
