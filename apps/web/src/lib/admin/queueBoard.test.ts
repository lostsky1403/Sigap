import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { boardUpdatedLabel, createQueueBoard, showsStaleWarning } from './queueBoard';
import type { ApiResult } from '$lib/api/client';
import type { ApiError } from '$lib/api/errors';
import type { AdminQueueTicket } from '$lib/api/types/api';

/**
 * T-3B4-04 §18: the queue board's polling lifecycle, deterministically.
 *
 * `domain/polling.test.ts` already proves the helper's mechanics: interval,
 * overlap guard, hidden-tab pause, visible-tab resume, abort on stop. What
 * these cases add is the BOARD POLICY, which the helper knows nothing about:
 *
 *   - a failed first load replaces the page
 *   - a failed refresh KEEPS the rows and marks them stale
 *   - the freshness label advances only on a success
 *
 * Those three are the claims the page is judged on, and each one is a way a
 * board can quietly lie to an operator.
 */

function setVisibility(state: 'visible' | 'hidden') {
	Object.defineProperty(document, 'visibilityState', {
		configurable: true,
		get: () => state
	});
}

function fireVisibilityChange() {
	document.dispatchEvent(new Event('visibilitychange'));
}

function ticket(id: string, number: number): AdminQueueTicket {
	return {
		id,
		facility_id: 'f1',
		queue_number: number,
		formatted_number: `RSK-${String(number).padStart(4, '0')}`,
		status: 'waiting',
		registered_at: '2026-09-28T10:00:00Z'
	};
}

function ok(rows: AdminQueueTicket[]): ApiResult<AdminQueueTicket[]> {
	return { ok: true, data: rows };
}

function fail(error: Partial<ApiError> = {}): ApiResult<AdminQueueTicket[]> {
	return {
		ok: false,
		error: { kind: 'server', status: 500, message: '', ...error }
	};
}

function deferred<T>() {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((res) => {
		resolve = res;
	});
	return { promise, resolve };
}

beforeEach(() => {
	vi.useFakeTimers();
	setVisibility('visible');
	vi.setSystemTime(new Date('2026-09-28T10:32:00Z'));
});

afterEach(() => {
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe('the queue board: first load', () => {
	it('loads immediately and marks the rows as fresh', async () => {
		const read = vi.fn(async () => ok([ticket('a', 1)]));
		const board = createQueueBoard({ read });

		board.start();
		await vi.advanceTimersByTimeAsync(0);

		expect(read).toHaveBeenCalledTimes(1);
		expect(board.state.load.rows).toHaveLength(1);
		expect(board.state.load.loading).toBe(false);
		expect(board.state.stale).toBe(false);
		board.stop();
	});

	it('passes a live AbortSignal to every read', async () => {
		const seen: AbortSignal[] = [];
		const board = createQueueBoard({
			read: async (signal) => {
				seen.push(signal);
				return ok([]);
			}
		});
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(seen[0]).toBeInstanceOf(AbortSignal);
		expect(seen[0].aborted).toBe(false);
		board.stop();
	});

	it('uses the first ErrorState when the FIRST load fails', async () => {
		// There is nothing to keep, so the honest rendering is a failure rather
		// than an empty board an operator would read as a quiet clinic.
		const board = createQueueBoard({ read: async () => fail() });
		board.start();
		await vi.advanceTimersByTimeAsync(0);

		expect(board.state.load.failed).toBe(true);
		expect(board.state.load.rows).toHaveLength(0);
		expect(board.state.stale).toBe(false);
		board.stop();
	});
});

describe('the queue board: 30s polling', () => {
	it('polls every 30 seconds', async () => {
		const read = vi.fn(async () => ok([ticket('a', 1)]));
		const board = createQueueBoard({ read });
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(read).toHaveBeenCalledTimes(1);

		await vi.advanceTimersByTimeAsync(30_000);
		expect(read).toHaveBeenCalledTimes(2);
		await vi.advanceTimersByTimeAsync(30_000);
		expect(read).toHaveBeenCalledTimes(3);
		board.stop();
	});

	it('never overlaps a slow request', async () => {
		const gate = deferred<ApiResult<AdminQueueTicket[]>>();
		const read = vi.fn(() => gate.promise);
		const board = createQueueBoard({ read });
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(read).toHaveBeenCalledTimes(1);

		// Two more intervals elapse while the first request is still open. A
		// stacked request would render the same ticket twice and load the API for
		// nothing.
		await vi.advanceTimersByTimeAsync(60_000);
		expect(read).toHaveBeenCalledTimes(1);

		gate.resolve(ok([ticket('a', 1)]));
		await vi.advanceTimersByTimeAsync(0);
		expect(board.state.load.rows).toHaveLength(1);
		board.stop();
	});
});

describe('the queue board: hidden tab', () => {
	it('pauses polling while hidden and resumes immediately when visible', async () => {
		const read = vi.fn(async () => ok([ticket('a', 1)]));
		const board = createQueueBoard({ read });
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(read).toHaveBeenCalledTimes(1);

		setVisibility('hidden');
		fireVisibilityChange();
		await vi.advanceTimersByTimeAsync(60_000);
		expect(read).toHaveBeenCalledTimes(1);

		setVisibility('visible');
		fireVisibilityChange();
		await vi.advanceTimersByTimeAsync(0);
		// Immediate, not "at the next interval": resuming the old cadence would
		// leave the board showing what was true when the operator looked away.
		expect(read).toHaveBeenCalledTimes(2);
		board.stop();
	});
});

describe('the queue board: manual refresh', () => {
	it('refreshes on demand', async () => {
		const read = vi.fn(async () => ok([ticket('a', 1)]));
		const board = createQueueBoard({ read });
		board.start();
		await vi.advanceTimersByTimeAsync(0);

		await board.refresh();
		expect(read).toHaveBeenCalledTimes(2);
		board.stop();
	});

	it('keeps the previous rows visible while a manual refresh is in flight', async () => {
		// Replacing visible rows with a skeleton on every refresh would make the
		// board flicker once a second for the operator watching it.
		const gate = deferred<ApiResult<AdminQueueTicket[]>>();
		let first = true;
		const board = createQueueBoard({
			read: () => {
				if (first) {
					first = false;
					return Promise.resolve(ok([ticket('a', 1)]));
				}
				return gate.promise;
			}
		});
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(board.state.load.rows).toHaveLength(1);

		const pending = board.refresh();
		expect(board.state.refreshing).toBe(true);
		expect(board.state.load.rows).toHaveLength(1);

		gate.resolve(ok([ticket('a', 1), ticket('b', 2)]));
		await pending;
		expect(board.state.load.rows).toHaveLength(2);
		expect(board.state.refreshing).toBe(false);
		board.stop();
	});
});

describe('the queue board: the freshness label', () => {
	it('advances only on a SUCCESSFUL load', async () => {
		let mode: 'ok' | 'fail' = 'ok';
		const board = createQueueBoard({
			read: async () => (mode === 'ok' ? ok([ticket('a', 1)]) : fail())
		});
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		const firstLabel = boardUpdatedLabel(board.state);
		expect(firstLabel).toMatch(/^Diperbarui pukul /);

		// Ten minutes later the poll fails.
		vi.setSystemTime(new Date('2026-09-28T10:42:00Z'));
		mode = 'fail';
		await vi.advanceTimersByTimeAsync(30_000);

		// The label must still read the time of the last SUCCESSFUL load.
		// Advancing it on a failure is the specific lie this label exists to
		// prevent: the operator would believe the data is current.
		expect(boardUpdatedLabel(board.state)).toBe(firstLabel);
		board.stop();
	});

	it('reads nothing at all before any load has succeeded', async () => {
		const board = createQueueBoard({ read: async () => ok([]) });
		// A page that has never loaded has no update time; printing a placeholder
		// would assert a freshness it cannot support.
		expect(boardUpdatedLabel(board.state)).toBe('');
	});
});

describe('the queue board: a failed refresh keeps the rows', () => {
	it('retains the last successful rows and raises the stale warning', async () => {
		let mode: 'ok' | 'fail' = 'ok';
		const board = createQueueBoard({
			read: async () => (mode === 'ok' ? ok([ticket('a', 1), ticket('b', 2)]) : fail())
		});
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(board.state.load.rows).toHaveLength(2);

		mode = 'fail';
		await vi.advanceTimersByTimeAsync(30_000);

		// The whole point. Blanking the board because one poll failed would
		// destroy the one thing the operator is looking at.
		expect(board.state.load.rows).toHaveLength(2);
		expect(board.state.load.failed).toBe(false);
		expect(showsStaleWarning(board.state)).toBe(true);
		// The error is remembered so the banner can explain it, without replacing
		// the data.
		expect(board.state.load.error).not.toBeNull();
		board.stop();
	});

	it('recovers and clears the stale warning on the next success', async () => {
		let mode: 'ok' | 'fail' = 'ok';
		const board = createQueueBoard({
			read: async () => (mode === 'ok' ? ok([ticket('a', 1)]) : fail())
		});
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		mode = 'fail';
		await vi.advanceTimersByTimeAsync(30_000);
		expect(showsStaleWarning(board.state)).toBe(true);

		mode = 'ok';
		await vi.advanceTimersByTimeAsync(30_000);
		expect(showsStaleWarning(board.state)).toBe(false);
		expect(board.state.stale).toBe(false);
		board.stop();
	});

	it('shows no stale warning when the very first load failed', async () => {
		// A page in ErrorState has nothing stale to warn about; a stale banner
		// there would claim there is data behind the error.
		const board = createQueueBoard({ read: async () => fail() });
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(showsStaleWarning(board.state)).toBe(false);
		expect(board.state.load.failed).toBe(true);
		board.stop();
	});
});

describe('the queue board: teardown', () => {
	it('aborts an in-flight request on stop and discards its response', async () => {
		const gate = deferred<ApiResult<AdminQueueTicket[]>>();
		let seen: AbortSignal | undefined;
		const board = createQueueBoard({
			read: (signal) => {
				seen = signal;
				return gate.promise;
			}
		});
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		expect(seen?.aborted).toBe(false);

		board.stop();
		// A hidden or navigated-away board must not hold a connection open.
		expect(seen?.aborted).toBe(true);

		gate.resolve(ok([ticket('a', 1)]));
		await vi.advanceTimersByTimeAsync(0);
		expect(board.state.load.rows).toHaveLength(0);
	});

	it('stops fetching after stop', async () => {
		const read = vi.fn(async () => ok([]));
		const board = createQueueBoard({ read });
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		board.stop();
		await vi.advanceTimersByTimeAsync(120_000);
		expect(read).toHaveBeenCalledTimes(1);
	});

	it('treats an abort as no state change at all', async () => {
		const board = createQueueBoard({
			read: async () => fail({ kind: 'aborted', status: 0 })
		});
		board.start();
		await vi.advanceTimersByTimeAsync(0);
		// An abort is the app cancelling its own request. Showing a failure to
		// someone who navigated away trains people to ignore error states.
		expect(board.state.load.failed).toBe(false);
		expect(board.state.load.loading).toBe(true);
		board.stop();
	});
});

describe('the queue board: subscription', () => {
	it('notifies subscribers and catches them up immediately', async () => {
		const board = createQueueBoard({ read: async () => ok([ticket('a', 1)]) });
		const seen: number[] = [];
		const unsubscribe = board.subscribe((next) => seen.push(next.load.rows.length));

		// Subscribe-and-catch-up: a subscriber registering after the load would
		// otherwise render one frame with the wrong value.
		expect(seen).toEqual([0]);
		board.start();
		await vi.advanceTimersByTimeAsync(0);

		// Asserted by last value and monotonic growth rather than an exact call
		// count: `start()` legitimately emits a `refreshing: true` frame before
		// the data lands, and pinning the count would make this test fail on a
		// harmless extra notification instead of on lost data.
		expect(seen[0]).toBe(0);
		expect(seen[seen.length - 1]).toBe(1);

		unsubscribe();
		const before = seen.length;
		await board.refresh();
		expect(seen).toHaveLength(before);
		board.stop();
	});
});
