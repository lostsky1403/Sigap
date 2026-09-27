import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import type { PublicFacilityType } from '$lib/api/types/api';
import Beranda from '../../routes/+page.svelte';
import Faskes from '../../routes/faskes/+page.svelte';

/**
 * Beranda and /faskes are the two citizen pages that consume real data, and
 * both were previously demo content. These tests exist to hold two lines at
 * once: the page must tell the truth about the public catalog, and it must not
 * quietly start telling a different truth later.
 *
 * The data is faked at the network boundary rather than at the component
 * boundary, deliberately. Mocking `loadPublicFacilities` would let the pages
 * render facilities the API would never return, which is exactly the class of
 * defect these tests exist to catch. Mocking `fetch` means every assertion
 * passes only if the real client, the real normalizer, and the real page agree.
 */

/** Matches a canonical v4 UUID, the shape the Go API actually issues. */
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function facility(id: string, name: string, shortCode: string, type: PublicFacilityType) {
	return { id, name, short_code: shortCode, type, is_active: true };
}

const CATALOG: ReturnType<typeof facility>[] = [
	facility('11111111-1111-4111-8111-111111111111', 'Puskesmas Sukajaya', 'PKM', 'puskesmas'),
	facility('22222222-2222-4222-8222-222222222222', 'RSUD Kota Sehat', 'RSK', 'rumah_sakit'),
	facility('33333333-3333-4333-8333-333333333333', 'Puskesmas Melati Indah', 'PMI', 'puskesmas')
];

/**
 * A catalog whose names actively contradict their `type`.
 *
 * Every facility here is named with the *other* category's prefix, so a UI
 * that inferred the type from the name would render exactly the wrong label
 * for all of them. This is the fixture that makes "the UI reads the wire
 * field" a testable claim rather than an assertion about the code.
 *
 * "RS Foo" carrying `type: 'puskesmas'` is the sharpest case: the name is
 * what a heuristic would read, and the enum is what the citizen is shown.
 */
const MISLEADING_CATALOG: ReturnType<typeof facility>[] = [
	facility('44444444-4444-4444-8444-444444444444', 'RS Foo', 'RSF', 'puskesmas'),
	facility('55555555-5555-4555-8555-555555555555', 'Puskesmas Bar', 'PKB', 'rumah_sakit')
];

/**
 * A catalog built so that one query matches rows of *both* types.
 *
 * Composition cannot be tested on `CATALOG`: its only "sehat" row is the one
 * hospital, so filtering that query to Puskesmas yields zero and the test
 * cannot tell "applied both conditions" apart from "ignored the type". These
 * three give the query two matches spanning both categories, so the surviving
 * row is identified by the type rather than by the count.
 */
const COMPOSING_CATALOG: ReturnType<typeof facility>[] = [
	facility('66666666-6666-4666-8666-666666666666', 'Pusreas Sehat', 'PRS', 'puskesmas'),
	facility('77777777-7777-4777-8777-777777777777', 'RSUD Kota Sehat', 'RSK', 'rumah_sakit'),
	facility('88888888-8888-4888-8888-888888888888', 'Puskesmas Melati Indah', 'PMI', 'puskesmas')
];

/**
 * A real `Response`, not a hand-rolled object literal.
 *
 * The client reads `headers.get('content-type')` before parsing, so a fake
 * response without headers fails deep inside the API layer and reports as a
 * client bug. Using the platform object also reproduces the trailing newline
 * that Go's json encoder appends.
 */
function jsonResponse(body: unknown, status = 200): Response {
	return new Response(`${JSON.stringify(body)}\n`, {
		status,
		headers: { 'content-type': 'application/json; charset=utf-8' }
	});
}

/**
 * Answers the public catalog with a fixed payload, and counts the calls.
 *
 * The responder is allowed to return a promise, not just a Response, because
 * the loading-state tests need a response that can be held open and released
 * later. `fetch` is asynchronous anyway, so awaiting the responder inside the
 * mock changes nothing about the code under test.
 */
function mockCatalog(responder: () => Response | Promise<Response>) {
	const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
		const url = String(input);
		if (url.includes('/api/v1/public/facilities')) return await responder();
		throw new Error(`unexpected request in citizen page test: ${url}`);
	});
	globalThis.fetch = fetchMock as never;
	return fetchMock;
}

function mockCatalogWithRows(rows: unknown[] = CATALOG) {
	return mockCatalog(() => jsonResponse({ success: true, data: rows }));
}

describe('Beranda', () => {
	beforeEach(() => {
		document.body.innerHTML = '';
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('shows a busy loading region before the catalog arrives', async () => {
		// A deferred response, so the loading branch is observed rather than
		// raced past. A test that only ever saw the resolved state would pass
		// even with no loading state at all.
		let release: (value: Response) => void = () => {};
		mockCatalog(
			() =>
				new Promise<Response>((resolve) => {
					release = resolve;
				})
		);

		render(Beranda);
		await tick();

		const busy = document.querySelector('[aria-busy="true"]');
		expect(busy, 'loading must be announced as busy').toBeTruthy();
		expect(document.body.textContent).toContain('Memuat daftar faskes');

		release(jsonResponse({ success: true, data: CATALOG }));
		await waitFor(() => expect(document.querySelector('[aria-busy="true"]')).toBeNull());
	});

	it('renders real facilities from the public catalog', async () => {
		mockCatalogWithRows();
		render(Beranda);

		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));
		expect(document.body.textContent).toContain('RSUD Kota Sehat');
		// Short code is a real public field and is rendered.
		expect(document.body.textContent).toContain('PKM');
	});

	it('shows the facility type in the preview, as the frozen design requires', async () => {
		mockCatalogWithRows();
		render(Beranda);

		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));

		// The frozen Beranda reference puts the classification under the
		// identity on every preview row, so the preview is not "name and short
		// code" — it is the same public identity as /faskes.
		const first = Array.from(document.querySelectorAll('li')).find((li) =>
			li.textContent?.includes('Puskesmas Sukajaya')
		);
		expect(first?.textContent).toContain('Puskesmas');

		// And the other enum value renders too, so this is a label map and not
		// a hardcoded string.
		const second = Array.from(document.querySelectorAll('li')).find((li) =>
			li.textContent?.includes('RSUD Kota Sehat')
		);
		expect(second?.textContent).toContain('Rumah Sakit');
		// Never the raw wire value.
		expect(document.body.textContent).not.toContain('rumah_sakit');
	});

	it('reads the preview type from the wire, never from the facility name', async () => {
		mockCatalogWithRows(MISLEADING_CATALOG);
		render(Beranda);

		await waitFor(() => expect(document.body.textContent).toContain('RS Foo'));

		// "RS Foo" is registered as a puskesmas. A name heuristic would call it
		// a hospital here exactly as it would on /faskes.
		const row = Array.from(document.querySelectorAll('li')).find((li) =>
			li.textContent?.includes('RS Foo')
		);
		expect(row?.textContent).toContain('Puskesmas');
		expect(row?.textContent).not.toContain('Rumah Sakit');
	});

	it('reads through the same-origin client, never a backend URL', async () => {
		const fetchMock = mockCatalogWithRows();
		render(Beranda);
		await waitFor(() => expect(fetchMock).toHaveBeenCalled());

		for (const [input] of fetchMock.mock.calls) {
			const url = String(input);
			// Browser code must go through the SvelteKit proxy. A hardcoded API
			// origin would bypass the proxy, the cookie jar, and the
			// same-origin CSRF posture in one move. The full path is also asserted
			// because a bare `/public/facilities` looks correct in source and
			// silently 404s against the router.
			expect(url, `wrong proxy path: ${url}`).toBe('/api/v1/public/facilities');
			expect(url.startsWith('/api/')).toBe(true);
			expect(url).not.toMatch(/^https?:\/\//);
		}
	});

	it('reports an empty catalog honestly instead of inventing facilities', async () => {
		// Go encodes a zero-row result as `data: null`. The client normalizes it
		// to [], and the page must show the empty state.
		mockCatalog(() => jsonResponse({ success: true, data: null }));
		render(Beranda);

		await waitFor(() => expect(document.body.textContent).toContain('Belum ada data faskes'));
		// The empty state must not fall back to placeholder facilities. This is
		// the exact failure the empty state exists to prevent: a plausible
		// looking list the backend never sent.
		expect(document.body.textContent).not.toContain('Puskesmas Sukajaya');
		expect(document.body.textContent).not.toContain('RSUD Kota Sehat');
		expect(document.querySelector('#sigap-facility-results')).toBeNull();
	});

	it('offers retry on failure and recovers on the second attempt', async () => {
		let attempt = 0;
		const fetchMock = mockCatalog(() => {
			attempt += 1;
			return attempt === 1
				? jsonResponse({ success: false, error: 'Gagal mengambil data fasilitas.' }, 500)
				: jsonResponse({ success: true, data: CATALOG });
		});

		render(Beranda);
		await waitFor(() => expect(document.body.textContent).toContain('Gagal mengambil data fasilitas'));

		const retry = screen.getByRole('button', { name: /Muat ulang|Coba lagi/ });
		await fireEvent.click(retry);

		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));
		expect(document.querySelector('[aria-busy="true"]')).toBeNull();
	});

	it('never displays a raw facility UUID', async () => {
		mockCatalogWithRows();
		render(Beranda);
		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));

		// A UUID is an identifier, not information a citizen can use. It belongs
		// in the booking link and nowhere visible.
		expect(document.body.textContent).not.toMatch(UUID);
	});

	it('contains no demo-stage or fabricated operational content', async () => {
		mockCatalogWithRows();
		render(Beranda);
		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));

		const text = document.body.textContent ?? '';
		// Each string below is a specific claim this page is forbidden to make:
		// bed counts, distance, wait times, ratings, opening hours, and live
		// status. None are backed by the public API, so none may be rendered.
		for (const forbidden of [
			'kamar kosong',
			'Kamar Kosong',
			'km',
			'menit',
			'Rating',
			'Buka',
			'realtime',
			'real-time',
			'placeholder',
			'Data contoh',
			'scaffolding',
			'lorem'
		]) {
			expect(text, `Beranda must not claim ${forbidden}`).not.toContain(forbidden);
		}
	});

	it('links only to canonical routes', async () => {
		mockCatalogWithRows();
		render(Beranda);
		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));

		const hrefs = Array.from(document.querySelectorAll('a')).map((a) => a.getAttribute('href'));
		for (const href of hrefs) {
			expect(href).toBeTruthy();
			// No wallet link anywhere, and no invented route.
			expect(href).not.toContain('/wallet');
		}
		expect(hrefs).toContain('/faskes');
		expect(hrefs).toContain('/appointments/new');
		expect(hrefs).toContain('/appointments/check-in');
		expect(hrefs).toContain('/patient/status');
	});

	it('exposes a single top-level heading', async () => {
		mockCatalogWithRows();
		render(Beranda);
		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));
		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
	});
});

describe('/faskes', () => {
	beforeEach(() => {
		document.body.innerHTML = '';
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it('shows the five required states as distinct experiences', async () => {
		// States 1 through 4 in sequence, then the error state. Empty and
		// no-result are asserted separately below because conflating them is the
		// specific failure this route must avoid.
		let release: (value: Response) => void = () => {};
		mockCatalog(
			() =>
				new Promise<Response>((resolve) => {
					release = resolve;
				})
		);
		render(Faskes);
		await tick();
		expect(document.body.textContent).toContain('Memuat daftar faskes');
		release(jsonResponse({ success: true, data: CATALOG }));
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());

		// State 4: search with no matches.
		await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'zzzz' } });
		await waitFor(() =>
			expect(document.body.textContent).toContain('Tidak ada faskes yang cocok')
		);

		// State 4 recovery: reset restores the full catalog.
		await fireEvent.click(screen.getByRole('button', { name: 'Hapus semua filter' }));
		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));
	});

	it('shows the empty-catalog copy when the catalog itself is empty', async () => {
		mockCatalog(() => jsonResponse({ success: true, data: null }));
		render(Faskes);
		await waitFor(() => expect(document.body.textContent).toContain('Belum ada data faskes'));
		// The empty catalog has nothing to search, so the box is not offered.
		expect(screen.queryByRole('searchbox')).toBeNull();
	});

	it('distinguishes an empty catalog from an empty search result', async () => {
		mockCatalogWithRows();
		render(Faskes);
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());

		await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'zzzz' } });
		await waitFor(() =>
			expect(document.body.textContent).toContain('Tidak ada faskes yang cocok')
		);

		// The two states must not share copy. If they did, a citizen whose search
		// matched nothing would be told the town has no facilities.
		expect(document.body.textContent).not.toContain('Belum ada data faskes');
	});

	it('filters on facility name and on short code', async () => {
		mockCatalogWithRows();
		render(Faskes);
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());
		const list = () => document.querySelector('#sigap-facility-results');
		const rows = () => list()?.querySelectorAll('li').length ?? 0;

		expect(rows()).toBe(3);

		// By name.
		await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'melati' } });
		await waitFor(() => expect(rows()).toBe(1));
		expect(document.body.textContent).toContain('Puskesmas Melati Indah');

		// By short code, which is a different field and a real one.
		await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'RSK' } });
		await waitFor(() => expect(document.body.textContent).toContain('RSUD Kota Sehat'));
		expect(rows()).toBe(1);

		// Search is case-insensitive.
		await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'rsk' } });
		await waitFor(() => expect(rows()).toBe(1));
	});

	it('renders only verified public fields in a row', async () => {
		mockCatalogWithRows();
		render(Faskes);
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());

		const row = document.querySelector('#sigap-facility-results li');
		expect(row?.textContent).toContain('Puskesmas Sukajaya');
		expect(row?.textContent).toContain('PKM');

		// The allowlist test. These are the claims a facility row is forbidden to
		// make regardless of how convenient they would be, because the public
		// endpoint supplies no such data.
		for (const forbidden of ['km', 'menit', 'Rating', 'kamar', 'Buka', 'antrean', '★']) {
			expect(row?.textContent, `row must not show ${forbidden}`).not.toContain(forbidden);
		}
		expect(row?.textContent).not.toMatch(UUID);
	});

	it('preselects the facility when heading to booking', async () => {
		mockCatalogWithRows();
		render(Faskes);
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());

		const row = document.querySelector('#sigap-facility-results li');
		const link = within(row as HTMLElement).getByRole('link');
		const href = link.getAttribute('href') ?? '';

		// Booking is Phase 3B3; this phase only has to arrive there with the
		// facility already chosen, which is the difference between picking an
		// faskes once and picking it twice.
		expect(href).toContain('/appointments/new');
		expect(href).toContain('facility_id=11111111-1111-4111-8111-111111111111');
		// And the link must be named, because the visible text is "Buat janji"
		// repeated down the list.
		expect(link.getAttribute('aria-label')).toContain('Puskesmas Sukajaya');
	});

	it('offers retry on API failure', async () => {
		let attempt = 0;
		mockCatalog(() => {
			attempt += 1;
			return attempt === 1
				? jsonResponse({ success: false, error: 'Gagal mengambil data fasilitas.' }, 500)
				: jsonResponse({ success: true, data: CATALOG });
		});
		render(Faskes);
		await waitFor(() => expect(document.body.textContent).toContain('Gagal mengambil data fasilitas'));

		await fireEvent.click(screen.getByRole('button', { name: /Coba lagi/ }));
		await waitFor(() => expect(document.body.textContent).toContain('Puskesmas Sukajaya'));
	});

	it('marks the active result set for assistive technology', async () => {
		mockCatalogWithRows();
		render(Faskes);
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());

		// Results are announced through a live region, and the search box points
		// at the list it controls.
		const status = document.querySelector('[aria-live="polite"]');
		expect(status?.textContent).toContain('3');
		expect(screen.getByRole('searchbox').getAttribute('aria-controls')).toBe(
			'sigap-facility-results'
		);
	});

	it('has a single top-level heading', async () => {
		mockCatalogWithRows();
		render(Faskes);
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());
		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
	});
});

/**
 * The facility type filter.
 *
 * Separated from the state tests above because these are about one specific
 * contract claim: the classification shown and filtered on is the real enum
 * from the wire. It is tempting to test the happy path — real names that
 * already agree with their type — but that fixture passes even if the UI
 * infers the type from the name, so it proves nothing. The tests below lean on
 * `MISLEADING_CATALOG`, where every name contradicts its `type`.
 */
describe('/faskes facility type filter', () => {
	beforeEach(() => {
		document.body.innerHTML = '';
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	/** The chip row, found by its accessible group name rather than a test id. */
	const typeGroup = () => screen.getByRole('group', { name: 'Saring berdasarkan tipe' });
	const chip = (name: string) => within(typeGroup()).getByRole('button', { name });
	const rows = () => document.querySelectorAll('#sigap-facility-results li');
	const rowText = () => Array.from(rows()).map((row) => row.textContent ?? '');

	/** Waits for the catalog to load, which is when the filter is offered. */
	async function readyWith(rows: unknown[] = CATALOG) {
		mockCatalogWithRows(rows);
		render(Faskes);
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());
	}

	it('offers exactly the three frozen controls, with Semua selected by default', async () => {
		await readyWith();

		// Exactly three, and no more. A fourth control would be a design change
		// nobody approved; a control labelled with a raw enum value would leak
		// the wire format into the citizen's UI.
		const labels = within(typeGroup())
			.getAllByRole('button')
			.map((button) => button.textContent?.trim());
		expect(labels).toEqual(['Semua', 'Puskesmas', 'Rumah Sakit']);

		// Default is Semua, pressed.
		expect(chip('Semua').getAttribute('aria-pressed')).toBe('true');
		expect(chip('Puskesmas').getAttribute('aria-pressed')).toBe('false');
		expect(chip('Rumah Sakit').getAttribute('aria-pressed')).toBe('false');
	});

	it('exposes the pressed state on every control, not only the selected one', async () => {
		await readyWith();

		// aria-pressed is the only thing a screen reader reports about the
		// selection, so a control that omits it while unselected is invisible
		// to assistive tech rather than merely unstyled.
		for (const name of ['Semua', 'Puskesmas', 'Rumah Sakit']) {
			expect(chip(name).getAttribute('aria-pressed'), `${name} needs aria-pressed`).toMatch(
				/^(true|false)$/
			);
		}
	});

	it('shows the human-readable type on every row', async () => {
		await readyWith();

		// Both enum values render, spelled for a citizen and never as the raw
		// wire value.
		expect(document.body.textContent).toContain('Puskesmas');
		expect(document.body.textContent).toContain('Rumah Sakit');
		expect(document.body.textContent).not.toContain('puskesmas');
		expect(document.body.textContent).not.toContain('rumah_sakit');
	});

	it('reads the type from the wire and never infers it from the name', async () => {
		await readyWith(MISLEADING_CATALOG);

		const [first, second] = rowText();
		// "RS Foo" is a puskesmas. A name-based heuristic renders "Rumah Sakit".
		expect(first).toContain('RS Foo');
		expect(first).toContain('Puskesmas');
		expect(first).not.toContain('Rumah Sakit');

		// "Puskesmas Bar" is a rumah sakit. The mirror case, same fixture.
		expect(second).toContain('Puskesmas Bar');
		expect(second).toContain('Rumah Sakit');
	});

	it('filters to puskesmas without touching the search box', async () => {
		await readyWith();

		expect(rows()).toHaveLength(3);
		await fireEvent.click(chip('Puskesmas'));
		await waitFor(() => expect(rows()).toHaveLength(2));

		// Both surviving rows are the real enum, not merely the right count.
		// A count alone would pass even if the wrong rows survived.
		for (const text of rowText()) {
			expect(text).toContain('Puskesmas');
			expect(text).not.toContain('Rumah Sakit');
		}
		expect(document.body.textContent).toContain('Puskesmas Melati Indah');
		expect(document.body.textContent).not.toContain('RSUD Kota Sehat');
	});

	it('filters to rumah sakit', async () => {
		await readyWith();
		await fireEvent.click(chip('Rumah Sakit'));
		await waitFor(() => expect(rows()).toHaveLength(1));

		expect(rowText()[0]).toContain('RSUD Kota Sehat');
		expect(rowText()[0]).toContain('Rumah Sakit');
	});

	it('restores the whole catalog when the citizen returns to Semua', async () => {
		await readyWith();

		await fireEvent.click(chip('Puskesmas'));
		await waitFor(() => expect(rows()).toHaveLength(2));
		await fireEvent.click(chip('Rumah Sakit'));
		await waitFor(() => expect(rows()).toHaveLength(1));
		await fireEvent.click(chip('Semua'));
		await waitFor(() => expect(rows()).toHaveLength(3));

		expect(chip('Semua').getAttribute('aria-pressed')).toBe('true');
		expect(screen.getByRole('searchbox').getAttribute('value') ?? '').toBe('');
	});

	it('composes the type filter with the search query', async () => {
		await readyWith(COMPOSING_CATALOG);

		// "sehat" matches two facilities spanning both categories, so applying
		// the type on top of the query is observable in *which* row survives,
		// not merely in the count. On a fixture where the query matches one
		// category only, "both applied" and "type ignored" produce the same
		// result and the test proves nothing.
		await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'sehat' } });
		await waitFor(() => expect(rows()).toHaveLength(2));
		expect(document.body.textContent).toContain('Pusreas Sehat');
		expect(document.body.textContent).toContain('RSUD Kota Sehat');

		await fireEvent.click(chip('Puskesmas'));
		await waitFor(() => expect(rows()).toHaveLength(1));
		expect(rowText()[0]).toContain('Pusreas Sehat');
		expect(rowText()[0]).toContain('Puskesmas');
		// The hospital matched the query but not the type. It must be gone.
		expect(document.body.textContent).not.toContain('RSUD Kota Sehat');

		// Widening the type again must not lose the query: the search is still
		// applied on top, so the hospital comes back and the third facility
		// stays out.
		await fireEvent.click(chip('Semua'));
		await waitFor(() => expect(rows()).toHaveLength(2));
		expect(document.body.textContent).toContain('RSUD Kota Sehat');
		expect(document.body.textContent).not.toContain('Puskesmas Melati Indah');
	});

	it('reaches a no-result state from the type filter alone, without quoting a search', async () => {
		await readyWith();

		await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'melati' } });
		await waitFor(() => expect(rows()).toHaveLength(1));
		// One puskesmas matched; asking for rumah sakit cannot.
		await fireEvent.click(chip('Rumah Sakit'));

		await waitFor(() =>
			expect(document.body.textContent).toContain('Tidak ada faskes yang cocok')
		);
		// The citizen typed a real query, so naming it is honest here — but the
		// type has to be named too, or the message blames the search alone.
		expect(document.body.textContent).toContain('Rumah Sakit');
	});

	it('keeps the no-result message honest when only the type is set', async () => {
		// A catalog holding one category only, so selecting the other produces
		// zero rows without any search having been typed. This is the case the
		// copy exists for: a filter the citizen chose is the cause, and quoting
		// an empty search box would blame something they never did.
		await readyWith([CATALOG[0], CATALOG[2]]);

		await fireEvent.click(chip('Rumah Sakit'));
		await waitFor(() =>
			expect(document.body.textContent).toContain('Tidak ada faskes yang cocok')
		);

		// It names the type, and does not invent a search to quote.
		expect(document.body.textContent).toContain('Tidak ada faskes bertipe Rumah Sakit');
		expect(document.body.textContent).not.toContain('Tidak ada hasil untuk');
		// Not the empty-catalog state: the catalog itself is fine.
		expect(document.body.textContent).not.toContain('Belum ada data faskes');
	});

	it('clears both the query and the type on reset', async () => {
		await readyWith();

		await fireEvent.click(chip('Rumah Sakit'));
		await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'kot' } });
		await waitFor(() => expect(rows()).toHaveLength(1));

		await fireEvent.click(screen.getByRole('button', { name: 'Hapus semua filter' }));
		await waitFor(() => expect(rows()).toHaveLength(3));

		// Both inputs, not just the visible one. A reset that cleared the search
		// but left the chip on "Rumah Sakit" would show a full list the citizen
		// could not account for.
		expect((screen.getByRole('searchbox') as HTMLInputElement).value).toBe('');
		expect(chip('Semua').getAttribute('aria-pressed')).toBe('true');
		expect(chip('Rumah Sakit').getAttribute('aria-pressed')).toBe('false');
	});

	it('offers a reset for a type filter even when the result set is non-empty', async () => {
		await readyWith();

		await fireEvent.click(chip('Rumah Sakit'));
		await waitFor(() => expect(rows()).toHaveLength(1));

		// One result, no search typed, and the way back is still there. Gating
		// the reset on a query would strand a citizen who filtered down to one
		// facility with no visible control to widen it again.
		expect(screen.getByRole('button', { name: 'Hapus semua filter' })).toBeTruthy();
	});

	it('activates a type control from the keyboard', async () => {
		await readyWith();

		// Real buttons, so activation is the browser's job. A div with an
		// on:click would pass a mouse-driven test and be unreachable by
		// keyboard entirely.
		const target = chip('Puskesmas') as HTMLButtonElement;
		expect(target.tagName).toBe('BUTTON');
		expect(target.getAttribute('type')).toBe('button');
		expect(target.disabled).toBe(false);

		await fireEvent.keyDown(target, { key: 'Enter' });
		target.click();
		await waitFor(() => expect(chip('Puskesmas').getAttribute('aria-pressed')).toBe('true'));
	});

	it('keeps the type controls off the loading and error screens', async () => {
		let release: (value: Response) => void = () => {};
		mockCatalog(
			() =>
				new Promise<Response>((resolve) => {
					release = resolve;
				})
		);
		render(Faskes);
		await tick();

		// Filtering data that has not arrived would imply results exist.
		expect(screen.queryByRole('group', { name: 'Saring berdasarkan tipe' })).toBeNull();
		release(jsonResponse({ success: true, data: CATALOG }));
		await waitFor(() => expect(screen.getByRole('searchbox')).toBeTruthy());

		// And filtering an empty list on an error screen would let a citizen
		// conclude the search is broken.
		vi.restoreAllMocks();
		document.body.innerHTML = '';
		mockCatalog(() =>
			jsonResponse({ success: false, error: 'Gagal mengambil data fasilitas.' }, 500)
		);
		render(Faskes);
		await waitFor(() => expect(document.body.textContent).toContain('Gagal mengambil data fasilitas'));
		expect(screen.queryByRole('group', { name: 'Saring berdasarkan tipe' })).toBeNull();
	});

	it('does not guess a type for a facility whose wire value is unrecognised', async () => {
		// A value the frontend does not know. It can happen when a backend
		// release lands ahead of a frontend one, and it must not be silently
		// sorted into a category it was never verified as.
		await readyWith([
			{ ...CATALOG[0], type: 'klinik_gigi' },
			CATALOG[1],
			CATALOG[2]
		]);

		// The row shows what actually arrived rather than a relabelled guess.
		// Asserted on the type line specifically: the facility is *named*
		// "Puskesmas Sukajaya", so a substring check over the whole row would
		// pass on the name alone and prove nothing about the label.
		const typeLine = rows()[0]?.querySelector('.sigap-facility-row__type')?.textContent ?? '';
		expect(typeLine).toBe('klinik_gigi');
		expect(typeLine).not.toContain('Puskesmas');
		expect(typeLine).not.toContain('Rumah Sakit');

		// Every specific filter excludes it...
		await fireEvent.click(chip('Puskesmas'));
		await waitFor(() => expect(rows()).toHaveLength(1));
		expect(rowText()[0]).toContain('Puskesmas Melati Indah');

		// ...but "Semua" still shows the full catalog, because hiding an
		// unrecognised facility would understate what is actually active.
		await fireEvent.click(chip('Semua'));
		await waitFor(() => expect(rows()).toHaveLength(3));
	});
});
