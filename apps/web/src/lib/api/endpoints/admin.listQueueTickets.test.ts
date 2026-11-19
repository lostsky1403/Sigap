import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { listQueueTickets } from './admin';

/**
 * T-3B4-04 service-day client contract: `date` is the only lever that changes
 * the queue list's day scope, and omitting it MUST leave the server's WIB today
 * default untouched. Sending `?date=` always is subtle: the server default and
 * an explicit today are the same rows now, but an explicit today pins the
 * request to a calendar date that a later operator (or a WIB/WITA midnight
 * boundary) can silently disagree with. Omitting is the contract.
 */

const fetchMock = vi.fn();

beforeEach(() => {
	fetchMock.mockReset();
	fetchMock.mockImplementation(async () =>
		new Response(JSON.stringify({ data: [] }), {
			status: 200,
			headers: { 'Content-Type': 'application/json' }
		})
	);
	vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
	vi.unstubAllGlobals();
});

function lastUrl(): string {
	const [input] = fetchMock.mock.calls[fetchMock.mock.calls.length - 1];
	return String(input);
}

describe('listQueueTickets service-day query contract', () => {
	it('omits ?date when no date is given, so the server today default applies', async () => {
		await listQueueTickets({});
		expect(lastUrl()).toBe('/api/v1/admin/queues');
	});

	it('sends ?date=YYYY-MM-DD for an explicit service day', async () => {
		await listQueueTickets({ date: '2026-11-07' });
		expect(lastUrl()).toBe('/api/v1/admin/queues?date=2026-11-07');
	});
});