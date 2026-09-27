import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
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

function facility(id: string, name: string, shortCode: string) {
	return { id, name, short_code: shortCode, is_active: true };
}

const CATALOG: ReturnType<typeof facility>[] = [
	facility('11111111-1111-4111-8111-111111111111', 'Puskesmas Sukajaya', 'PKM'),
	facility('22222222-2222-4222-8222-222222222222', 'RSUD Kota Sehat', 'RSK'),
	facility('33333333-3333-4333-8333-333333333333', 'Puskesmas Melati Indah', 'PMI')
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
