import { describe, expect, it } from 'vitest';
import { ADMIN_PAGE_SIZE, clampPage, pageCount, pageRange, pageSlice } from './paginate';

/**
 * The paging math is shared by the appointments and notifications pages and by
 * the PaginationControls primitive, so an off-by-one here shows a row twice on
 * a boundary or an empty page after a filter shrink — and it does so identically
 * on every consumer. These cases are the boundaries, not the happy path.
 */

describe('pageCount', () => {
	it('is never less than 1', () => {
		expect(pageCount(0)).toBe(1);
		expect(pageCount(1)).toBe(1);
		expect(pageCount(ADMIN_PAGE_SIZE)).toBe(1);
	});

	it('counts an exact multiple without a phantom page', () => {
		expect(pageCount(ADMIN_PAGE_SIZE * 2)).toBe(2);
		expect(pageCount(ADMIN_PAGE_SIZE * 2 + 1)).toBe(3);
	});
});

describe('clampPage', () => {
	it('pulls a below-range page up to 1', () => {
		expect(clampPage(0, 100)).toBe(1);
		expect(clampPage(-3, 100)).toBe(1);
		expect(clampPage(NaN, 100)).toBe(1);
	});

	it('pulls a past-the-end page down to the last page', () => {
		expect(clampPage(99, ADMIN_PAGE_SIZE * 2)).toBe(2);
	});

	it('leaves an in-range page untouched', () => {
		expect(clampPage(2, ADMIN_PAGE_SIZE * 3)).toBe(2);
	});
});

describe('pageRange', () => {
	it('reports the empty range as 0–0', () => {
		expect(pageRange(1, 0)).toEqual({ from: 0, to: 0 });
	});

	it('reports a full first page and a partial last page', () => {
		expect(pageRange(1, ADMIN_PAGE_SIZE * 3)).toEqual({ from: 1, to: ADMIN_PAGE_SIZE });
		expect(pageRange(3, ADMIN_PAGE_SIZE * 2 + 7)).toEqual({
			from: ADMIN_PAGE_SIZE * 2 + 1,
			to: ADMIN_PAGE_SIZE * 2 + 7
		});
	});
});

describe('pageSlice', () => {
	const items = Array.from({ length: ADMIN_PAGE_SIZE + 3 }, (_, i) => i);

	it('slices the requested page', () => {
		expect(pageSlice(items, 2)).toEqual([ADMIN_PAGE_SIZE, ADMIN_PAGE_SIZE + 1, ADMIN_PAGE_SIZE + 2]);
	});

	it('clamps a below-range page to page 1 rather than returning nothing', () => {
		expect(pageSlice(items, 0)[0]).toBe(0);
	});

	it('clamps a past-the-end page to the last page rather than returning nothing', () => {
		expect(pageSlice(items, 999)).toEqual([
			ADMIN_PAGE_SIZE,
			ADMIN_PAGE_SIZE + 1,
			ADMIN_PAGE_SIZE + 2
		]);
	});
});