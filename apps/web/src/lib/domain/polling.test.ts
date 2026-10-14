import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DEFAULT_POLL_INTERVAL_MS, createPolling } from './polling';

/**
 * Polling contract (Phase 3B1 section 17).
 *
 * Driven with fake timers so these are deterministic rather than timing-flaky.
 * Each case corresponds to a failure mode that a naive setInterval produces:
 * overlapping requests, a hidden tab still polling, a duplicated event
 * listener after restart, or an abort surfacing as a user-visible error.
 */

/** Controls document.visibilityState, which jsdom does not change on demand. */
function setVisibility(state: 'visible' | 'hidden') {
	Object.defineProperty(document, 'visibilityState', {
		configurable: true,
		get: () => state
	});
}

function fireVisibilityChange() {
	document.dispatchEvent(new Event('visibilitychange'));
}

/** A promise plus its resolver, for holding a request open deliberately. */
function deferred<T>() {
	let resolve!: (value: T) => void;
	let reject!: (reason: unknown) => void;
	const promise = new Promise<T>((res, rej) => {
		resolve = res;
		reject = rej;
	});
	return { promise, resolve, reject };
}

beforeEach(() => {
	vi.useFakeTimers();
	setVisibility('visible');
});

afterEach(() => {
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe('createPolling', () => {
	it('defaults to a 30 second interval for the queue board', () => {
		expect(DEFAULT_POLL_INTERVAL_MS).toBe(30_000);
	});

	it('polls on the interval', async () => {
		const onData = vi.fn();
		const polling = createPolling({
			fetch: async () => 'ok',
			onData,
			intervalMs: 1000
		});

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);
		expect(onData).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(2000);
		expect(onData).toHaveBeenCalledTimes(3);

		polling.stop();
	});

	it('can refresh immediately on start', async () => {
		const onData = vi.fn();
		const polling = createPolling({
			fetch: async () => 'ok',
			onData,
			intervalMs: 1000,
			immediate: true
		});

		polling.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(onData).toHaveBeenCalledTimes(1);

		polling.stop();
	});

	it('refuses to overlap a slow request', async () => {
		// A request slower than the interval must delay the next poll, not stack
		// a second concurrent request on top of it.
		const gate = deferred<string>();
		const fetcher = vi.fn(() => gate.promise);
		const onData = vi.fn();
		const polling = createPolling({ fetch: fetcher, onData, intervalMs: 1000 });

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);
		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(polling.isFetching()).toBe(true);

		// Two more intervals elapse while the first request is still open.
		await vi.advanceTimersByTimeAsync(2000);
		expect(fetcher).toHaveBeenCalledTimes(1);

		gate.resolve('late');
		await vi.advanceTimersByTimeAsync(0);
		expect(onData).toHaveBeenCalledTimes(1);
		expect(polling.isFetching()).toBe(false);

		polling.stop();
	});

	it('supports a manual refresh', async () => {
		const onData = vi.fn();
		const polling = createPolling({
			fetch: async () => 'ok',
			onData,
			intervalMs: 10_000
		});
		polling.start();

		await polling.refresh();
		expect(onData).toHaveBeenCalledTimes(1);

		polling.stop();
	});

	it('passes an AbortSignal to the fetcher', async () => {
		let seen: AbortSignal | undefined;
		const polling = createPolling({
			fetch: async (signal) => {
				seen = signal;
				return 'ok';
			},
			onData: vi.fn(),
			intervalMs: 1000
		});

		polling.start();
		await polling.refresh();
		expect(seen).toBeInstanceOf(AbortSignal);
		// A live request is never pre-aborted.
		expect(seen?.aborted).toBe(false);

		polling.stop();
	});

	it('aborts an in-flight request on stop, not just the timer', async () => {
		// Clearing the interval alone would leave a request running against a
		// controller the caller believes is gone.
		const gate = deferred<string>();
		let seen: AbortSignal | undefined;
		const polling = createPolling({
			fetch: (signal) => {
				seen = signal;
				return gate.promise;
			},
			onData: vi.fn(),
			intervalMs: 1000
		});

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);
		expect(polling.isFetching()).toBe(true);
		expect(seen?.aborted).toBe(false);

		polling.stop();
		expect(seen?.aborted).toBe(true);

		gate.resolve('ignored');
		await vi.advanceTimersByTimeAsync(0);
	});

	it('treats an aborted request as a no-op, not an error', async () => {
		// Navigating away mid-poll must not show a red error or call onError.
		const onError = vi.fn();
		const onData = vi.fn();
		const polling = createPolling({
			fetch: async () => {
				throw new DOMException('aborted', 'AbortError');
			},
			onData,
			onError,
			intervalMs: 1000
		});

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);

		expect(onError).not.toHaveBeenCalled();
		expect(onData).not.toHaveBeenCalled();

		polling.stop();
	});

	it('reports a genuine failure', async () => {
		const onError = vi.fn();
		const polling = createPolling({
			fetch: async () => {
				throw new Error('server exploded');
			},
			onData: vi.fn(),
			onError,
			intervalMs: 1000
		});

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);
		expect(onError).toHaveBeenCalledTimes(1);

		polling.stop();
	});

	it('pauses while the tab is hidden', async () => {
		const fetcher = vi.fn(async () => 'ok');
		const polling = createPolling({ fetch: fetcher, onData: vi.fn(), intervalMs: 1000 });
		polling.start();

		await vi.advanceTimersByTimeAsync(1000);
		expect(fetcher).toHaveBeenCalledTimes(1);

		setVisibility('hidden');
		fireVisibilityChange();
		expect(polling.isPolling()).toBe(false);

		// Intervals continue to elapse, but nothing is fetched.
		await vi.advanceTimersByTimeAsync(5000);
		expect(fetcher).toHaveBeenCalledTimes(1);

		polling.stop();
	});

	it('refreshes immediately when the tab becomes visible again', async () => {
		// Resuming the old cadence would leave the board showing what was true
		// when the user switched away.
		const fetcher = vi.fn(async () => 'ok');
		const polling = createPolling({ fetch: fetcher, onData: vi.fn(), intervalMs: 1000 });
		polling.start();

		await vi.advanceTimersByTimeAsync(1000);
		expect(fetcher).toHaveBeenCalledTimes(1);

		setVisibility('hidden');
		fireVisibilityChange();
		setVisibility('visible');
		fireVisibilityChange();
		await vi.advanceTimersByTimeAsync(0);

		expect(fetcher).toHaveBeenCalledTimes(2);
		polling.stop();
	});

	it('aborts an in-flight request when the tab is hidden', async () => {
		const gate = deferred<string>();
		let seen: AbortSignal | undefined;
		const polling = createPolling({
			fetch: (signal) => {
				seen = signal;
				return gate.promise;
			},
			onData: vi.fn(),
			intervalMs: 1000
		});

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);
		expect(seen?.aborted).toBe(false);

		setVisibility('hidden');
		fireVisibilityChange();
		// A hidden tab must not hold a connection open.
		expect(seen?.aborted).toBe(true);

		polling.stop();
	});

	it('stops cleanly on destroy and ignores later ticks', async () => {
		const fetcher = vi.fn(async () => 'ok');
		const onData = vi.fn();
		const polling = createPolling({ fetch: fetcher, onData, intervalMs: 1000 });

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);
		expect(fetcher).toHaveBeenCalledTimes(1);

		polling.stop();
		expect(polling.isStopped()).toBe(true);
		expect(polling.isPolling()).toBe(false);

		await vi.advanceTimersByTimeAsync(10_000);
		expect(fetcher).toHaveBeenCalledTimes(1);

		// A manual refresh after stop is also inert.
		await polling.refresh();
		expect(fetcher).toHaveBeenCalledTimes(1);
	});

	it('discards a response that arrives after stop', async () => {
		// A slow request must not repopulate a page the user already left.
		const gate = deferred<string>();
		const onData = vi.fn();
		const polling = createPolling({ fetch: () => gate.promise, onData, intervalMs: 1000 });

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);
		polling.stop();

		gate.resolve('stale data');
		await vi.advanceTimersByTimeAsync(0);
		expect(onData).not.toHaveBeenCalled();
	});

	it('does not double-register the visibility listener on restart', async () => {
		const addSpy = vi.spyOn(document, 'addEventListener');
		const removeSpy = vi.spyOn(document, 'removeEventListener');
		const polling = createPolling({ fetch: async () => 'ok', onData: vi.fn(), intervalMs: 1000 });

		polling.start();
		polling.stop();
		const addsAfterFirst = addSpy.mock.calls.filter((c) => c[0] === 'visibilitychange').length;
		const removesAfterFirst = removeSpy.mock.calls.filter((c) => c[0] === 'visibilitychange').length;

		polling.start();
		await vi.advanceTimersByTimeAsync(1000);
		setVisibility('hidden');
		fireVisibilityChange();
		polling.stop();

		// Balanced add/remove across the lifecycle: a leaked listener would
		// survive a stop and fire against a dead controller.
		expect(addsAfterFirst).toBe(1);
		expect(removesAfterFirst).toBe(1);
		expect(removeSpy.mock.calls.filter((c) => c[0] === 'visibilitychange')).toHaveLength(2);
	});
});
