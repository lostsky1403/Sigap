import { describe, expect, it } from 'vitest';
import { latestQueueChange, queueChangeStamps } from './queueSource';
import type { AdminQueueTicket } from '$lib/api/types/api';

/**
 * The queue source marker.
 *
 * These tests exist because the schema fact this encodes is invisible to every
 * other kind of check. Nothing in the type system stops someone adding
 * `updated_at` to `AdminQueueTicket`, and a substring assertion on a component
 * cannot tell working logic from an explanatory comment. So the behaviour is
 * pinned here, against the exact columns `queue_tickets` actually has.
 */

const ticket = (over: Partial<AdminQueueTicket> = {}): AdminQueueTicket => ({
	id: 't-1',
	facility_id: 'f-1',
	queue_number: 1,
	formatted_number: 'RSK-0001',
	status: 'waiting',
	registered_at: '2026-01-01T08:00:00Z',
	...over
});

describe('queueChangeStamps', () => {
	it('returns the three columns a queue status transition can stamp', () => {
		expect(queueChangeStamps(ticket())).toEqual(['2026-01-01T08:00:00Z']);
		expect(
			queueChangeStamps(
				ticket({
					registered_at: '2026-01-01T08:00:00Z',
					called_at: '2026-01-01T08:05:00Z',
					completed_at: '2026-01-01T08:20:00Z'
				})
			)
		).toEqual(['2026-01-01T08:00:00Z', '2026-01-01T08:05:00Z', '2026-01-01T08:20:00Z']);
	});

	it('drops stamps a ticket does not have rather than rendering undefined', () => {
		// called_at and completed_at are omitempty on the wire, so a ticket that
		// has never been called genuinely carries no call time.
		expect(queueChangeStamps(ticket())).not.toContain(undefined);
		expect(queueChangeStamps(ticket())).toHaveLength(1);
	});

	it('treats an empty string as absent, not as a real time', () => {
		expect(queueChangeStamps(ticket({ registered_at: '' }))).toEqual([]);
	});
});

describe('latestQueueChange', () => {
	it('is null when no ticket carries a stamp', () => {
		expect(latestQueueChange([])).toBeNull();
		expect(latestQueueChange([ticket({ registered_at: '' })])).toBeNull();
	});

	it('finds the newest stamp on a single ticket', () => {
		const t = ticket({
			registered_at: '2026-01-01T08:00:00Z',
			called_at: '2026-01-01T08:30:00Z',
			completed_at: '2026-01-01T08:10:00Z'
		});
		// Newest, not last in column order: completed_at is older than called_at.
		expect(latestQueueChange([t])).toBe('2026-01-01T08:30:00Z');
	});

	it('finds the newest stamp across tickets, in any order', () => {
		const rows = [
			ticket({ id: 'a', registered_at: '2026-01-01T09:00:00Z' }),
			ticket({ id: 'b', registered_at: '2026-01-01T07:00:00Z' }),
			ticket({ id: 'c', registered_at: '2026-01-01T11:30:00Z' })
		];
		expect(latestQueueChange(rows)).toBe('2026-01-01T11:30:00Z');
	});

	it('skips a malformed stamp rather than claiming a freshness it cannot establish', () => {
		const rows = [
			ticket({ id: 'a', registered_at: 'not-a-date' }),
			ticket({ id: 'b', registered_at: '2026-01-01T07:00:00Z' })
		];
		expect(latestQueueChange(rows)).toBe('2026-01-01T07:00:00Z');
		expect(latestQueueChange([ticket({ registered_at: 'not-a-date' })])).toBeNull();
	});

	it('never invents an updated_at, because the schema has none', () => {
		// A ticket object carrying a bogus updated_at must not change the answer.
		// If the marker ever started reading that field, this would drift.
		const withBogus = { ...ticket({ registered_at: '2026-01-01T08:00:00Z' }), updated_at: '2099-12-31T00:00:00Z' };
		expect(latestQueueChange([withBogus as AdminQueueTicket])).toBe('2026-01-01T08:00:00Z');
	});
});
