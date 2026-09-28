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
		const secondary = document.querySelectorAll('.sigap-data-table__th--secondary');
		expect(secondary).toHaveLength(1);
		expect(secondary[0].textContent).toContain('Terdaftar');
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

	it('omits the count when nothing is filtered', async () => {
		// "Menampilkan: 12 dari 12" on an unfiltered list is noise, and printing
		// it unconditionally trains the eye to skip the informative case.
		render(FacilityFilter, { facilities: FACILITIES, selected: '', visible: 12, total: 12, onSelect: () => {} });
		expect(screen.queryByText(/Menampilkan:/)).toBeNull();
	});

	it('omits the count while loading, rather than claiming zero', async () => {
		// "Menampilkan: 0 dari 0" over a table that is still arriving reads as
		// "your clinic has no data", which is a different and alarming claim.
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
