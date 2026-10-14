import type { AdminQueueTicket } from '$lib/api/types/api';

/**
 * The queue board's source marker.
 *
 * WHY THIS EXISTS, and why it is not an `updated_at` field.
 *
 * `queue_tickets` has no `updated_at` column. The Go list query in
 * `apps/api/internal/handler/admin.go` selects exactly:
 *
 *   id, facility_id, queue_number, formatted_number, status,
 *   registered_at, called_at, completed_at
 *
 * and the migration schema has no further timestamp to select. So there is no
 * `updated_at` on a queue row to preserve. Rendering one would mean inventing a
 * value, which is exactly the failure mode Phase 3B4 exists to prevent — and it
 * is why the queue board and the appointment page deliberately show different
 * markers. `appointments` does have `updated_at`, and that page uses it.
 *
 * What the backend CAN answer is when each row last changed state, because
 * every queue status transition stamps one of the three columns above. The
 * newest stamp across a set of rows is therefore the honest equivalent of "when
 * did this data last change" — the question a per-row source marker exists to
 * answer.
 *
 * This lives in a module rather than inline in the component for one concrete
 * reason: it is pure, it encodes a fact about the schema, and a schema fact is
 * worth a test that can fail. A comment claiming which columns exist cannot be
 * verified by any test; this can.
 */

/** The columns a queue status transition can stamp, newest last. */
const QUEUE_CHANGE_STAMP_KEYS = ['registered_at', 'called_at', 'completed_at'] as const;

/**
 * Every stamp a single ticket carries, in a stable order.
 *
 * Missing stamps are dropped rather than rendered, because `called_at` and
 * `completed_at` are omitempty on the wire and a ticket that has never been
 * called genuinely has no call time.
 */
export function queueChangeStamps(ticket: AdminQueueTicket): string[] {
	return QUEUE_CHANGE_STAMP_KEYS.map((key) => ticket[key])
		.filter((value): value is string => typeof value === 'string' && value.length > 0);
}

/**
 * The most recent change recorded across a set of tickets, or null when none of
 * them carries a stamp.
 *
 * Invalid dates are skipped rather than compared: a malformed timestamp must not
 * make the whole board claim a freshness it cannot establish.
 */
export function latestQueueChange(tickets: readonly AdminQueueTicket[]): string | null {
	let latest: string | null = null;
	let latestMs = Number.NEGATIVE_INFINITY;

	for (const ticket of tickets) {
		for (const stamp of queueChangeStamps(ticket)) {
			const ms = new Date(stamp).getTime();
			if (Number.isNaN(ms)) continue;
			if (ms > latestMs) {
				latestMs = ms;
				latest = stamp;
			}
		}
	}

	return latest;
}
