/**
 * Interval polling with the failure modes actually designed out.
 *
 * Polling is where a simple `setInterval(fetch, 30000)` quietly causes the three
 * problems this module exists to prevent:
 *
 * 1. Overlap. If a request takes longer than the interval, a naive loop piles
 *    up concurrent requests against a health service. An in-flight guard means a
 *    slow response delays the next poll instead of stacking on it.
 * 2. Wasted work in a hidden tab. Polling a tab nobody is looking at drains a
 *    phone battery for nothing, so polling pauses on visibilitychange and
 *    refreshes once on return rather than resuming the old cadence.
 * 3. Scary error states. An abort is a cancellation, not a failure. Without the
 *    abort handling here, navigating away mid-poll throws an unhandled rejection
 *    and can flash a red error at a user who did nothing wrong.
 */

/** The interval the admin queue board will use. */
export const DEFAULT_POLL_INTERVAL_MS = 30_000;

export interface PollingOptions<T> {
	/** Performs the work. Receives a signal so it can abandon the request. */
	fetch: (signal: AbortSignal) => Promise<T>;
	/** Called with each successful result. */
	onData: (data: T) => void;
	/** Called on a real failure. Never called for an abort. */
	onError?: (error: unknown) => void;
	/** Milliseconds between polls. */
	intervalMs?: number;
	/** Start polling immediately rather than after the first interval. */
	immediate?: boolean;
}

export interface PollingController {
	/** Runs the work now, subject to the overlap guard. */
	refresh: () => Promise<void>;
	/** Stops the timer and aborts any in-flight request. */
	stop: () => void;
	/** Resumes after stop(). Does not re-register listeners twice. */
	start: () => void;
	/** True while a request is in flight. */
	isFetching: () => boolean;
	/** False while the tab is hidden. */
	isPolling: () => boolean;
	/** True after stop(). */
	isStopped: () => boolean;
}

/**
 * Creates a polling controller.
 *
 * The caller owns the lifecycle: `stop()` must be called on destroy, which
 * `createPolling` does not do on its own because a store and a component have
 * different teardown conventions.
 */
export function createPolling<T>(options: PollingOptions<T>): PollingController {
	const { fetch: fetcher, onData, onError, intervalMs = DEFAULT_POLL_INTERVAL_MS, immediate = false } =
		options;

	let timer: ReturnType<typeof setInterval> | undefined;
	let controller: AbortController | undefined;
	let stopped = false;
	let inFlight = false;
	let paused = false;

	function isDocumentHidden(): boolean {
		// `document.visibilityState` is authoritative; `hidden` is the older
		// spelling and is not always populated in test environments.
		if (typeof document === 'undefined') return false;
		return document.visibilityState === 'hidden';
	}

	async function refresh(): Promise<void> {
		// The overlap guard. Without it a request slower than the interval
		// produces concurrent duplicate requests, which for a queue board means
		// rendering the same ticket twice and loading the API for nothing.
		if (inFlight || stopped) return;
		if (paused) return;

		inFlight = true;
		controller = new AbortController();
		try {
			const data = await fetcher(controller.signal);
			// A response that arrives after stop() is discarded, so a slow
			// in-flight request cannot repopulate a page the user has left.
			if (stopped) return;
			onData(data);
		} catch (error) {
			// An abort is our own doing. Surfacing it as an error trains people to
			// ignore real error states.
			if (isAbortError(error)) return;
			if (stopped) return;
			onError?.(error);
		} finally {
			inFlight = false;
			controller = undefined;
		}
	}

	function tick() {
		void refresh();
	}

	function handleVisibilityChange() {
		if (stopped) return;
		if (isDocumentHidden()) {
			paused = true;
			// Abort whatever is in flight: its result cannot be shown anyway, and
			// leaving it running keeps a connection open for a hidden tab.
			controller?.abort();
			return;
		}
		paused = false;
		// Refresh straight away on return. Resuming the old cadence would leave
		// the board showing whatever was true when the user switched away.
		void refresh();
	}

	function start() {
		if (timer) return;
		stopped = false;
		paused = isDocumentHidden();
		if (typeof document !== 'undefined') {
			document.addEventListener('visibilitychange', handleVisibilityChange);
		}
		timer = setInterval(tick, intervalMs);
		if (immediate) void refresh();
	}

	function stop() {
		stopped = true;
		paused = false;
		if (timer) {
			clearInterval(timer);
			timer = undefined;
		}
		controller?.abort();
		controller = undefined;
		if (typeof document !== 'undefined') {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		}
	}

	return {
		refresh,
		stop,
		start,
		isFetching: () => inFlight,
		isPolling: () => !stopped && !paused,
		isStopped: () => stopped
	};
}

function isAbortError(error: unknown): boolean {
	return (
		(error instanceof DOMException && error.name === 'AbortError') ||
		(error instanceof Error && error.name === 'AbortError')
	);
}
