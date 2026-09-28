import { formatNumber } from '$lib/domain/format';
import type { NotificationStatus, NotificationSummary } from '$lib/api/types/api';

/**
 * The notification outbox read layer's derived values.
 *
 * Kept out of the component so the parts that are easy to get wrong — the
 * `limit` ceiling and the summary fallback — are testable without a DOM, and so
 * a second consumer cannot quietly reintroduce a different limit.
 */

/** The server's default when no `limit` is sent. */
export const NOTIFICATION_DEFAULT_LIMIT = 100;

/** The server's hard maximum. Anything above this is clamped, not honoured. */
export const NOTIFICATION_MAX_LIMIT = 500;

/**
 * The limit this page requests.
 *
 * The server default is 100 and its maximum is 500. This page asks for neither:
 * it asks for the DEFAULT by omitting the parameter entirely, so the number of
 * rows an operator can see is whatever the backend decided rather than a figure
 * this client invented.
 *
 * There is deliberately no page-number control, no "load more", and no total
 * count anywhere on this page. The endpoint returns a bounded list with no
 * cursor and no total, so any pagination affordance would be a control that
 * cannot work — and a row count presented as "of N" would imply a total the
 * server never sent.
 */
export const NOTIFICATION_PAGE_LIMIT = NOTIFICATION_DEFAULT_LIMIT;

/** The footer's honest statement about what the list is. */
export const NOTIFICATION_LIMIT_NOTE =
	`Filter membatasi data yang dimuat dari sistem. Menampilkan hingga ${formatNumber(
		NOTIFICATION_PAGE_LIMIT
	)} notifikasi terbaru.`;

/**
 * The five summary cards, in the frozen order.
 *
 * Every key is always present — the server zero-fills all five — so the card set
 * renders in one pass and a status with a zero count still appears. A card that
 * vanishes when its count is zero is how a backlog becomes invisible.
 */
export const NOTIFICATION_STATUS_ORDER: readonly NotificationStatus[] = [
	'pending',
	'processing',
	'delivered',
	'failed',
	'cancelled'
] as const;

export const NOTIFICATION_STATUS_LABEL: Record<NotificationStatus, string> = {
	pending: 'Menunggu',
	processing: 'Diproses',
	delivered: 'Terkirim',
	failed: 'Gagal',
	cancelled: 'Dibatalkan'
};

/** Badge tone per status. Paired with the label, never used as the only signal. */
export const NOTIFICATION_STATUS_TONE: Record<
	NotificationStatus,
	'neutral' | 'info' | 'success' | 'warning' | 'danger'
> = {
	pending: 'warning',
	processing: 'info',
	delivered: 'success',
	failed: 'danger',
	cancelled: 'neutral'
};

/**
 * A zero-filled summary.
 *
 * Used ONLY when the summary endpoint has not answered yet, so the card set has
 * a stable shape. It is never used to paper over a summary FAILURE: a failed
 * summary shows its own error next to the list, because silently showing five
 * zeros next to a populated list reads as "nothing has ever been sent", which
 * is a materially different and much more alarming claim than "we could not load
 * the counts".
 *
 * Note what is NOT here: any fallback derived from the LIST. The list is
 * filtered and bounded, so counting it would produce a number scoped to the
 * current filter and present it as a facility-wide total.
 */
export function zeroSummary(): NotificationSummary {
	return { pending: 0, processing: 0, delivered: 0, failed: 0, cancelled: 0 };
}

/** The default filter state: nothing narrowed. */
export interface NotificationFilters {
	status: string;
	channel: string;
	templateKey: string;
	createdFrom: string;
	createdTo: string;
}

export function emptyFilters(): NotificationFilters {
	return { status: '', channel: '', templateKey: '', createdFrom: '', createdTo: '' };
}

export function hasActiveFilters(filters: NotificationFilters): boolean {
	return Boolean(
		filters.status ||
			filters.channel ||
			filters.templateKey ||
			filters.createdFrom ||
			filters.createdTo
	);
}

/**
 * Reads filters out of a URL's search params.
 *
 * Date filters are truncated to `YYYY-MM-DD` because the URL carries a full
 * day-boundary timestamp (`T00:00:00Z` / `T23:59:59Z`) while the controls are
 * date inputs. Reversing the expansion here is what makes a shared link
 * reproduce the operator's filter exactly.
 */
export function filtersFromParams(params: URLSearchParams): NotificationFilters {
	return {
		status: params.get('status') ?? '',
		channel: params.get('channel') ?? '',
		templateKey: params.get('template_key') ?? '',
		createdFrom: params.get('created_from')?.slice(0, 10) ?? '',
		createdTo: params.get('created_to')?.slice(0, 10) ?? ''
	};
}

/**
 * Writes the filters back into a URL.
 *
 * Unset filters are DELETED rather than set to an empty string, so a shared link
 * never carries `?status=` — which the backend would read as a filter matching
 * the empty status, returning nothing.
 */
export function applyFiltersToUrl(url: URL, filters: NotificationFilters): URL {
	const set = (key: string, value: string) => {
		if (value) url.searchParams.set(key, value);
		else url.searchParams.delete(key);
	};
	set('status', filters.status);
	set('channel', filters.channel);
	set('template_key', filters.templateKey);
	set('created_from', filters.createdFrom ? `${filters.createdFrom}T00:00:00Z` : '');
	set('created_to', filters.createdTo ? `${filters.createdTo}T23:59:59Z` : '');
	return url;
}

/**
 * The server query for the current filters.
 *
 * `limit` is the ONLY pagination-adjacent parameter sent, and it is the
 * backend's own default. There is no `offset` and no `page`, because the
 * endpoint has no way to honour either.
 */
export function buildNotificationQuery(filters: NotificationFilters): Record<string, string> {
	const query: Record<string, string> = {};
	if (filters.status) query.status = filters.status;
	if (filters.channel) query.channel = filters.channel;
	if (filters.templateKey) query.template_key = filters.templateKey;
	if (filters.createdFrom) query.created_from = `${filters.createdFrom}T00:00:00Z`;
	if (filters.createdTo) query.created_to = `${filters.createdTo}T23:59:59Z`;
	return query;
}
