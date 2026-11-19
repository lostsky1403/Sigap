/**
 * Client-side paging for the admin lists.
 *
 * WHY THIS EXISTS: the admin endpoints take no paging parameters (admin.go
 * parses none), so the pages page what they have already loaded. Two pages do
 * it — appointments and notifications — and the arithmetic is exactly the kind
 * that drifts when each page writes its own: an off-by-one in the offset shows
 * a row twice on a boundary, and a stale page number after a filter change
 * shows an empty table that looks like "no data".
 *
 * Both of those are the reason `clampPage` exists rather than a bare slice:
 * when a filter shrinks the list below the current page, the page number has to
 * come back into range, or the operator sees nothing and concludes the filter
 * matched nothing.
 */

/** The default page size. 25 rows keeps the appointments tab order ~75 stops. */
export const ADMIN_PAGE_SIZE = 25;

/** Total number of pages for `total` items, never less than 1. */
export function pageCount(total: number, pageSize: number = ADMIN_PAGE_SIZE): number {
	if (pageSize <= 0) return 1;
	return Math.max(1, Math.ceil(total / pageSize));
}

/**
 * The page number to actually render, given a requested page and the current
 * item count. A page below 1 becomes 1; a page past the end becomes the last
 * page. Without this a shrink (filter, mutation) leaves the operator on a page
 * that no longer exists.
 */
export function clampPage(page: number, total: number, pageSize: number = ADMIN_PAGE_SIZE): number {
	const last = pageCount(total, pageSize);
	if (!Number.isFinite(page) || page < 1) return 1;
	return Math.min(Math.floor(page), last);
}

/** The slice of `items` belonging to `page`, with the page clamped into range. */
export function pageSlice<T>(
	items: readonly T[],
	page: number,
	pageSize: number = ADMIN_PAGE_SIZE
): T[] {
	const safe = clampPage(page, items.length, pageSize);
	const start = (safe - 1) * pageSize;
	return items.slice(start, start + pageSize);
}

/** The 1-based inclusive range shown on `page`, for the "Menampilkan X–Y" line. */
export function pageRange(
	page: number,
	total: number,
	pageSize: number = ADMIN_PAGE_SIZE
): { from: number; to: number } {
	if (total === 0) return { from: 0, to: 0 };
	const safe = clampPage(page, total, pageSize);
	return { from: (safe - 1) * pageSize + 1, to: Math.min(safe * pageSize, total) };
}
