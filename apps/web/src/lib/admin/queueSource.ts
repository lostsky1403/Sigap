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

/**
 * A local calendar day key, e.g. "2026-10-23".
 *
 * Built from LOCAL date parts, never from `toISOString()`. The board is read by
 * a clerk in their own timezone and the backend stamps UTC, so a visit
 * completed at 00.30 WIB is 17.30 UTC on the previous day: slicing a UTC string
 * would file it under yesterday and drop it out of today's band. Returns null
 * for anything that is not a real instant, so a caller can distinguish "not
 * today" from "no usable stamp".
 */
export function localDayKey(value: string | number | Date): string | null {
	const date = value instanceof Date ? value : new Date(value);
	if (Number.isNaN(date.getTime())) return null;
	const month = `${date.getMonth() + 1}`.padStart(2, '0');
	const day = `${date.getDate()}`.padStart(2, '0');
	return `${date.getFullYear()}-${month}-${day}`;
}

/**
 * The band whose day scope is being applied. Only the bands that make a claim
 * about WHEN their rows happened need one; `waiting` deliberately has none,
 * because a patient registered yesterday is still waiting today.
 */
export type QueueDayBand = 'called' | 'finished';

/**
 * The stamp that decides which day a ticket joined its band.
 *
 * `UpdateQueueStatus` in apps/api/internal/handler/admin.go stamps
 * `completed_at` for `completed` and `cancelled` ONLY. A `skipped` ticket
 * therefore carries no completion stamp at all — its newest stamp is
 * `called_at`, the moment it was called and then passed over. So `skipped` is
 * the one finished status whose day is read from the call time; for the other
 * two, `completed_at` is the completion and is used as-is.
 *
 * Deliberately NO fallback from a missing `completed_at` to `called_at` for
 * completed/cancelled rows: that would claim the ticket was finished at the
 * moment it was called, which the data does not say. Such a row is UNKNOWN and
 * stays in the live band instead.
 */
export function queueBandStamp(
	ticket: AdminQueueTicket,
	band: QueueDayBand
): string | undefined {
	if (band === 'called') return ticket.called_at;
	if (ticket.status === 'skipped') return ticket.called_at;
	return ticket.completed_at;
}

/** A band's rows split by the local calendar day their band stamp falls on. */
export interface QueueDayPartition {
	/**
	 * Rows that belong in the live band, in their original order: those stamped
	 * today, plus those whose stamp cannot be placed on any day (see
	 * `unknownCount`). Unknown rows are kept visible rather than folded away,
	 * because a missing timestamp is not evidence of an old row — hiding a
	 * patient from the board is a worse error than showing one without a time.
	 */
	live: AdminQueueTicket[];
	/** Rows provably stamped on an earlier local day. */
	older: AdminQueueTicket[];
	/** How many of `live` are stamped today. */
	todayCount: number;
	/** How many of `live` have no readable stamp (missing, malformed, or ahead of today). */
	unknownCount: number;
}

/**
 * Splits a band's tickets into today's live rows and provably older ones.
 *
 * A stamp on a FUTURE local day is counted as unknown rather than "older": it
 * can only come from clock skew across midnight, and filing it under "sebelum
 * hari ini" would state something the data does not support.
 */
export function partitionQueueDay(
	tickets: readonly AdminQueueTicket[],
	band: QueueDayBand,
	now: Date = new Date()
): QueueDayPartition {
	const todayKey = localDayKey(now);
	const partition: QueueDayPartition = { live: [], older: [], todayCount: 0, unknownCount: 0 };

	for (const ticket of tickets) {
		const stamp = queueBandStamp(ticket, band);
		const dayKey = typeof stamp === 'string' && stamp !== '' ? localDayKey(stamp) : null;

		if (todayKey !== null && dayKey === todayKey) {
			partition.live.push(ticket);
			partition.todayCount += 1;
			continue;
		}

		if (todayKey !== null && dayKey !== null && dayKey < todayKey) {
			partition.older.push(ticket);
			continue;
		}

		partition.live.push(ticket);
		partition.unknownCount += 1;
	}

	return partition;
}
