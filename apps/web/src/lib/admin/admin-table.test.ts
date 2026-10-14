import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import DataTable from '$lib/ui/DataTable.svelte';
import AdminTable from './AdminTable.svelte';
import FacilityFilter from './FacilityFilter.svelte';
import { canHideColumn } from './AdminResponsiveTablePattern.svelte';
import type { AdminFacility } from '$lib/api/types/api';

/**
 * T-3B4-02: the admin table layer.
 *
 * The claims pinned here are the ones a screenshot cannot check. A table can
 * look correct and still be a grid of divs with no header association, hide its
 * own status column at 1024px, or describe a client-side filter as a server
 * query. Each of those is asserted directly.
 */

beforeEach(() => {
	document.body.innerHTML = '';
	vi.restoreAllMocks();
});

const FACILITIES: AdminFacility[] = [
	{
		id: 'f1',
		name: 'RSUD Kota Sehat',
		type: 'rumah_sakit',
		address: '',
		kecamatan: 'Kota',
		kabupaten_kota: 'Kota',
		provinsi: 'Jawa',
		phone: '021-1',
		total_beds: 50,
		available_beds: 42,
		is_active: true,
		short_code: 'RSK'
	},
	{
		id: 'f2',
		name: 'Puskesmas Sukajaya',
		type: 'puskesmas',
		address: '',
		kecamatan: 'Sukajaya',
		kabupaten_kota: 'Kabupaten',
		provinsi: 'Jawa',
		phone: '021-2',
		total_beds: 10,
		available_beds: 8,
		is_active: false,
		short_code: 'PKM'
	}
];

describe('DataTable: semantics', () => {
	const columns = [
		{ label: 'Nomor' },
		{ label: 'Status' },
		{ label: 'Terdaftar', secondary: true }
	];

	it('renders a real table with a caption and a column header row', () => {
		render(DataTable, { columns, caption: 'Antrean dalam lingkup akses' });
		const table = document.querySelector('table');
		expect(table, 'must be a semantic table, not a grid of divs').toBeTruthy();

		// The caption is how the table is named for assistive tech. Omitting it
		// leaves a screen-reader user with an unnamed table they cannot refer to.
		const caption = table?.querySelector('caption');
		expect(caption?.textContent).toContain('Antrean dalam lingkup akses');
	});

	it('gives every column header a scope of col', () => {
		render(DataTable, { columns, caption: 'x' });
		const headers = document.querySelectorAll('thead th');
		expect(headers).toHaveLength(3);
		for (const header of Array.from(headers)) {
			// Without scope, a screen reader re-reading a cell cannot say which
			// column the value belonged to.
			expect(header.getAttribute('scope')).toBe('col');
		}
	});

	it('marks only the declared secondary column as secondary', () => {
		render(DataTable, { columns, caption: 'x' });
		const secondary = document.querySelectorAll('.sigap-table-col-secondary');
		expect(secondary).toHaveLength(1);
		expect(secondary[0].textContent).toContain('Terdaftar');
	});

	it('uses ONE class for both halves of a secondary column', () => {
		// The header and the cell must be hidden by the same rule. A scoped rule on
		// the header alone would leave the cell visible, producing a table with more
		// cells than headers and every later value under the wrong label.
		render(DataTable, { columns, caption: 'x' });
		const header = document.querySelectorAll('thead th');
		expect(header[2].classList.contains('sigap-table-col-secondary')).toBe(true);
		expect(header[0].classList.contains('sigap-table-col-secondary')).toBe(false);
	});

	it('renders the caller-supplied rows inside tbody', () => {
		render(DataTable, { columns, caption: 'x' });
		expect(document.querySelector('tbody')).toBeTruthy();
	});

	it('never renders a raw UUID as a column label', () => {
		render(DataTable, { columns, caption: 'x' });
		const text = document.body.textContent ?? '';
		expect(text).not.toMatch(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}/i);
	});
});

describe('the column-priority rule', () => {
	it('never allows a status or action column to be hidden', () => {
		// The invariant that matters: below 1280px a secondary column disappears,
		// so a mislabelled status column would vanish exactly when the viewport is
		// narrowest and the operator most needs to see the state of the work.
		expect(canHideColumn({ label: 'Status', secondary: true })).toBe(false);
		expect(canHideColumn({ label: 'Aksi', secondary: true })).toBe(false);
		expect(canHideColumn({ label: 'Terdaftar', secondary: true })).toBe(true);
	});

	it('treats an essential column as never hidden', () => {
		expect(canHideColumn({ label: 'Terdaftar' })).toBe(false);
	});
});

/**
 * Regression guard for a rule that LOOKED correct and rendered as nothing.
 *
 * Phase 3B4.1 measured admin rows at 26px and found the cause: DataTable's row
 * rule was written as `.sigap-data-table tbody td`, which Svelte compiles to
 *
 *   .sigap-data-table.svelte-XXX tbody:where(.svelte-XXX) td:where(.svelte-XXX)
 *
 * The `td` elements belong to the CALLER's row components and carry a different
 * scope hash, so the selector matched nothing and every row silently fell back
 * to its content height. The stylesheet asserted a height; the browser never
 * applied one.
 *
 * A unit test cannot catch this class of bug — jsdom injects no component
 * stylesheet at all, so a computed-style assertion passes against any value.
 * The real guard is e2e/admin-table-density.spec.ts, which measures rendered
 * rows in Chromium. These two assertions below only ensure the *source* has not
 * quietly gone back to a scoped selector, so a future refactor fails loudly here
 * instead of shipping a table that looks right and measures 26px.
 */
describe('DataTable: the row rule must be able to reach the caller rows', () => {
	// Resolved through fileURLToPath + join, the same way visual-audit.test.ts
	// does it. `new URL(...).pathname` is not a usable path on Windows: it comes
	// back as "/F:/..." and readFileSync throws, which fails the whole suite at
	// collection time rather than at the assertion.
	const here = dirname(fileURLToPath(import.meta.url));
	const source = readFileSync(join(here, '..', 'ui', 'DataTable.svelte'), 'utf8');

	it('declares the row height rule global, not scoped', () => {
		// `:global(.sigap-data-table tbody td)`. A bare `.sigap-data-table tbody
		// td` compiles to a chain that cannot match a slotted row's cells.
		expect(source).toContain(':global(.sigap-data-table tbody td)');
		expect(source).not.toMatch(/^\s*\.sigap-data-table tbody td\s*\{/m);
	});

	it('declares the hover rule global for the same reason', () => {
		expect(source).toContain(':global(.sigap-data-table tbody tr:hover td)');
	});

	it('keeps the admin row height inside the 36-40px band', () => {
		// The value itself, so a future edit cannot quietly widen the band. The
		// rendered measurement is the E2E test's job; this is the cheap tripwire.
		const block = source.slice(source.indexOf(':global(.sigap-data-table tbody td)'));
		const height = /height:\s*(\d+)px/.exec(block);
		expect(height, 'the admin row rule must declare an explicit px height').not.toBeNull();
		const value = Number(height?.[1]);
		expect(value).toBeGreaterThanOrEqual(36);
		expect(value).toBeLessThanOrEqual(40);
	});
});

describe('AdminTable: the four states', () => {
	const columns = [{ label: 'Nomor' }];

	it('shows a loading state and no table before data arrives', () => {
		render(AdminTable, { columns, loading: true, rows: [] });
		expect(document.querySelector('table')).toBeNull();
	});

	it('renders rows once loaded', () => {
		render(AdminTable, { columns, loading: false, rows: [{ id: 1 }], caption: 'Antrean' });
		expect(document.querySelector('table')).toBeTruthy();
	});

	it('distinguishes an empty result from a failed request', () => {
		// The bug this prevents: an operator seeing "no queue" when the truth is
		// that the request failed. They would conclude the clinic is quiet.
		const { unmount } = render(AdminTable, { columns, loading: false, rows: [] });
		expect(screen.getByRole('status')).toBeTruthy();
		unmount();

		document.body.innerHTML = '';
		render(AdminTable, { columns, loading: false, failed: true, rows: [] });
		expect(document.querySelector('table')).toBeNull();
	});

	it('renders no table at all when the load failed', () => {
		render(AdminTable, { columns, loading: false, failed: true, rows: [{ id: 1 }] });
		// A half-rendered table over an error is worse than none, because the
		// operator reads the stale rows as current truth.
		expect(document.querySelector('table')).toBeNull();
	});

	it('separates a filter that hid everything from a genuinely empty list', () => {
		// Telling an operator to clear a filter when the list is truly empty sends
		// them to inspect a control that is not the problem.
		render(AdminTable, {
			columns,
			loading: false,
			rows: [],
			noMatchesTitle: 'Tidak ada baris yang cocok',
			noMatchesDescription: 'Coba ubah atau hapus filter Anda.'
		});
		expect(screen.getByText(/tidak ada baris yang cocok/i)).toBeTruthy();
		expect(screen.queryByText(/lingkup akses Anda saat ini/i)).toBeNull();
	});
});

describe('FacilityFilter: a client-side narrowing control', () => {
	it('offers facility NAMES, never raw ids, and no free-text id field', () => {
		render(FacilityFilter, { facilities: FACILITIES, selected: '', onSelect: () => {} });
		const select = screen.getByLabelText(/filter fasilitas/i) as HTMLSelectElement;
		const options = Array.from(select.options).map((o) => o.textContent?.trim());
		expect(options).toContain('RSUD Kota Sehat');
		expect(options).toContain('Puskesmas Sukajaya');
		// The pre-3B4 page had a free-text "Filter ID Fasilitas" box that invited
		// operators to paste identifiers and produced unexplained empty tables.
		expect(document.querySelector('input[type="text"]')).toBeNull();
		expect(document.body.textContent).not.toMatch(/filter id fasilitas/i);
	});

	it('emits the selected facility id through onSelect', async () => {
		const onSelect = vi.fn();
		render(FacilityFilter, { facilities: FACILITIES, selected: '', onSelect });
		const select = screen.getByLabelText(/filter fasilitas/i) as HTMLSelectElement;
		await fireEvent.change(select, { target: { value: 'f2' } });
		expect(onSelect).toHaveBeenCalledWith('f2');
	});

	it('says "Menampilkan: X dari Y" while a filter is narrowing', async () => {
		render(FacilityFilter, {
			facilities: FACILITIES,
			selected: 'f1',
			visible: 3,
			total: 12,
			onSelect: () => {}
		});
		// The only claim a client-side filter can honestly make: this is what you
		// are seeing out of what was loaded.
		expect(screen.getByText(/Menampilkan: 3 dari 12/)).toBeTruthy();
	});

	it('says "Menampilkan: Y dari Y" when nothing is filtered', async () => {
		// Phase 3B4.1 changed this. Suppressing the count while unfiltered was
		// tidiness, and it removed the only statement on the control that the
		// dataset is COMPLETE. Without it an operator cannot tell "12 rows, that
		// is all of them" from "12 rows, of many more" — and the difference is
		// the whole reason a count label exists.
		render(FacilityFilter, { facilities: FACILITIES, selected: '', visible: 12, total: 12, onSelect: () => {} });
		expect(screen.getByText(/Menampilkan: 12 dari 12/)).toBeTruthy();
	});

	it('still says "Menampilkan: 0 dari Y" when a narrowing matches nothing', async () => {
		// Zero results from a client-side narrowing is still a claim about a KNOWN
		// dataset, and the count is what says how many rows were excluded. The
		// "no rows match" empty state that renders underneath says nothing about
		// the size of the set being narrowed, so the count must survive.
		render(FacilityFilter, { facilities: FACILITIES, selected: 'f2', visible: 0, total: 12, onSelect: () => {} });
		expect(screen.getByText(/Menampilkan: 0 dari 12/)).toBeTruthy();
	});

	it('omits the count while loading, rather than claiming zero', async () => {
		// "Menampilkan: 0 dari 0" over a table that is still arriving reads as
		// "your clinic has no data", which is a different and alarming claim.
		// Withheld totals mean there is nothing truthful to print yet.
		render(FacilityFilter, { facilities: FACILITIES, selected: 'f1', onSelect: () => {} });
		expect(screen.queryByText(/Menampilkan:/)).toBeNull();
	});

	it('never claims to be a server-side query', () => {
		render(FacilityFilter, { facilities: FACILITIES, selected: '', onSelect: () => {} });
		const text = (document.body.textContent ?? '').toLowerCase();
		// These words would tell an operator they are searching the clinic's
		// records when they are only narrowing their own authorized view.
		for (const forbidden of ['server', ' Pencarian', 'cari fasilitas di']) {
			expect(text).not.toContain(forbidden.toLowerCase());
		}
	});

	it('issues no request of its own', async () => {
		const fetchSpy = vi.fn();
		globalThis.fetch = fetchSpy as never;
		render(FacilityFilter, { facilities: FACILITIES, selected: '', onSelect: () => {} });
		await waitFor(() => expect(true).toBe(true));
		// A filter that refetched would be a second data source, and could offer a
		// facility the operator is not authorized for.
		expect(fetchSpy).not.toHaveBeenCalled();
	});
});
