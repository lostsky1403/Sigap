/**
 * id-ID formatting helpers.
 *
 * `Intl` carries the Indonesian locale, so grouping and month names come from
 * the platform rather than a hand-maintained table of Indonesian words.
 */

const ID_LOCALE = 'id-ID';

/** Placeholder for anything absent. Same contract as joins.UNRESOLVED. */
export const EMPTY = '-';

/** Coerces a backend value to a Date, or null when it is not a real timestamp. */
export function toDate(value: string | number | Date | null | undefined): Date | null {
	if (value === null || value === undefined || value === '') return null;
	const date = value instanceof Date ? value : new Date(value);
	return Number.isNaN(date.getTime()) ? null : date;
}

/** Full date and time, e.g. "26 Sep 2026, 14.05". */
export function formatDateTime(value: string | number | Date | null | undefined): string {
	const date = toDate(value);
	if (!date) return EMPTY;
	return new Intl.DateTimeFormat(ID_LOCALE, {
		dateStyle: 'medium',
		timeStyle: 'short'
	}).format(date);
}

/** Date only, e.g. "26 Sep 2026". */
export function formatDate(value: string | number | Date | null | undefined): string {
	const date = toDate(value);
	if (!date) return EMPTY;
	return new Intl.DateTimeFormat(ID_LOCALE, { dateStyle: 'medium' }).format(date);
}

/**
 * Time only, e.g. "14.05".
 *
 * Written with an explicit pattern rather than timeStyle so a citizen reads
 * "14.05" rather than "14:05"; the rest of the product is Indonesian.
 */
export function formatTime(value: string | number | Date | null | undefined): string {
	const date = toDate(value);
	if (!date) return EMPTY;
	return new Intl.DateTimeFormat(ID_LOCALE, {
		hour: '2-digit',
		minute: '2-digit',
		hour12: false
	}).format(date);
}

/**
 * A plain number, with Indonesian digit grouping.
 *
 * Queue and bed counts read badly with a thousands separator typed by hand, and
 * this is the only place that conversion should live.
 */
export function formatNumber(value: number | null | undefined): string {
	if (value === null || value === undefined || Number.isNaN(value)) return EMPTY;
	return new Intl.NumberFormat(ID_LOCALE).format(value);
}

/**
 * Relative time, e.g. "5 menit lalu".
 *
 * The unit is chosen from the LARGEST magnitude that applies, and the tests
 * cover the boundaries explicitly. The previous implementation compared
 * day-minutes against an hour threshold first, so an event two days old could
 * render as "48 jam lalu" or an event 90 minutes old as "1 jam lalu" depending
 * on the order of the checks. Getting the ordering wrong here is what makes a
 * queue board look broken: "2 jam lalu" next to a call that just happened.
 *
 * Anything older than a week is rendered as an absolute date. A relative
 * string for a three-month-old row is noise, and a stale timestamp must never
 * be presented as if it were current.
 */
export function formatRelativeTime(
	value: string | number | Date | null | undefined,
	now: Date = new Date()
): string {
	const date = toDate(value);
	if (!date) return EMPTY;

	const diffMs = now.getTime() - date.getTime();
	// A timestamp in the future means clock skew, not a negative age. Rendering
	// "0 menit lalu" is the least alarming truthful reading.
	if (diffMs < 0) return 'Baru saja';

	const seconds = Math.floor(diffMs / 1000);
	if (seconds < 45) return 'Baru saja';

	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes} menit lalu`;

	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours} jam lalu`;

	const days = Math.floor(hours / 24);
	// Seven days is the last day described relatively. `days <= 7` keeps
	// "7 hari lalu" reading as recent; at 8 days the absolute date is clearer.
	if (days <= 7) return `${days} hari lalu`;

	// Beyond a week an absolute date is clearer than "47 hari lalu", and it
	// cannot be misread as current.
	return formatDate(date);
}

/**
 * True when a timestamp is too old to present as current.
 *
 * The queue board uses this to decide whether to keep showing a relative age or
 * to say plainly that the data is stale, so a frozen screen is visibly stale
 * rather than quietly lying.
 */
export function isStale(
	value: string | number | Date | null | undefined,
	thresholdMs: number,
	now: Date = new Date()
): boolean {
	const date = toDate(value);
	if (!date) return true;
	return now.getTime() - date.getTime() > thresholdMs;
}

/** Uppercase first letter, for building a sentence from a raw status token. */
export function sentenceCase(value: string | null | undefined): string {
	if (typeof value !== 'string') return EMPTY;
	const trimmed = value.trim();
	if (trimmed === '') return EMPTY;
	return trimmed.charAt(0).toUpperCase() + trimmed.slice(1);
}

/** Truncates to `max` characters, appending an ellipsis only when it cut. */
export function truncate(value: string | null | undefined, max: number): string {
	if (typeof value !== 'string') return EMPTY;
	if (max <= 0) return '';
	if (value.length <= max) return value;
	return `${value.slice(0, Math.max(max - 1, 0))}…`;
}
