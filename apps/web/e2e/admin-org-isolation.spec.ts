import type { Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { expect, test } from './support/test';

/**
 * T-regional pilot: the running ORGANIZATION surface, end to end.
 *
 * These four assertions are about the directory the backend actually returns
 * and the per-organization overview it actually aggregates. Every name and id
 * asserted here is read from `GET /api/v1/admin/organizations` through the
 * same-origin proxy — nothing is intercepted and no response is faked — so
 * "the table shows two organizations" means the scoped read really returned
 * two, not that a fixture happened to be found.
 *
 * THE ACTOR. This spec runs pinned to the super-admin preview actor exported by
 * Start-LocalE2E.ps1 (`SIGAP_LOCAL_E2E_ACTOR`): super_admin holds both seeded
 * organizations, so it alone can assert a POPULATED directory and a successful
 * overview. That is the whole point of running the browser against
 * super-admin rather than against the org-A / org-B / regional actors.
 *
 * WHAT IS DELIBERATELY OUT OF SCOPE HERE. Per-actor scoping — that org-A's
 * admin sees only org A, org-B's admin sees only org B, and a zero-scope actor
 * sees the fail-closed empty state — is a server-side decision in
 * `ListOrganizations` / `GetOrganizationOverview`, and it is NOT browser-
 * switchable: the subject is pinned by the `SIGAP_LOCAL_E2E_ACTOR` launch
 * variable, not by any control the client can reach. Re-seeding a browser run
 * with a different actor would re-prove a server property against one
 * hand-picked id at a time, which is strictly weaker than the exhaustive
 * coverage already owned by the Go tenant tests
 * (apps/api/internal/handler/org_tenancy_test.go and regional_scenarios_test.go)
 * plus the direct API-level evidence in the facilities read. The browser spec
 * therefore covers only what the browser alone can prove: super-admin reads,
 * the 404 denial render, and the responsive/density surface.
 */

const DESKTOP = { width: 1440, height: 900 };

/** Real round trips against a local API; the default timeout is too tight. */
const SETTLE = { timeout: 15_000 };

/** Any UUID. Used to prove a machine identifier never reaches the UI. */
const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

/** The four scoped aggregate labels on the per-organization overview. */
const COUNT_LABELS = ['Fasilitas', 'Unit layanan', 'Antrean aktif', 'Janji temu'] as const;

/**
 * Waits for the page's own heading, exactly as the other admin specs do: the
 * shell renders immediately, so waiting for the sidebar would let these tests
 * start asserting table content before the read had resolved.
 */
async function gotoAdmin(page: Page, path: string) {
	await page.goto(path);
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
}

/** The admin nav landmark, by its accessible name. */
function adminNav(page: Page) {
	return page.getByRole('navigation', { name: 'Menu operasi' });
}

/** Fails if the document scrolls sideways at the current width. */
async function expectNoHorizontalOverflow(page: Page) {
	const overflow = await page.evaluate(() => ({
		scrollWidth: document.documentElement.scrollWidth,
		clientWidth: document.documentElement.clientWidth
	}));
	expect(
		overflow.scrollWidth,
		`content is ${overflow.scrollWidth}px wide in a ${overflow.clientWidth}px viewport`
	).toBeLessThanOrEqual(overflow.clientWidth + 1);
}

/**
 * The scope the running actor actually holds, read from the backend rather than
 * assumed. This is what makes the two seeded org assertions survive a future
 * seed edit: they assert the table renders every row the API returned, not a
 * pair hardcoded against a specific snapshot.
 */
async function fetchScopedOrgs(page: Page): Promise<Array<{ id: string; name: string }>> {
	const response = await page.request.get('/api/v1/admin/organizations');
	expect(response.ok(), 'the scoped organization read must succeed').toBe(true);
	return ((await response.json()).data as Array<{ id: string; name: string }>);
}

test.describe('admin organization isolation', () => {
	test.use({ viewport: DESKTOP });

	test('the directory renders the two seeded organizations, nothing else', async ({ page }) => {
		await gotoAdmin(page, '/admin/organizations');

		// The frozen h1. This is the page's own heading, not a string lifted from
		// the nav, so it asserts the destination rendered rather than the shell.
		await expect(page.getByRole('heading', { level: 1 })).toHaveText('Organisasi');

		const orgs = await fetchScopedOrgs(page);
		// super_admin holds both seeded orgs; anything less and this spec is no
		// longer asserting a populated directory.
		expect(orgs.length, 'super-admin must hold both seeded organizations').toBe(2);

		const table = page.locator('table.sigap-data-table');
		await expect(table).toBeVisible(SETTLE);

		// Exactly two data rows, matching what the scoped read returned.
		const rows = table.locator('tbody tr');
		await expect(rows).toHaveCount(orgs.length);

		// Every scoped name reaches the UI, so this is the backend's directory and
		// not a client-side reconstruction.
		const text = (await table.textContent()) ?? '';
		for (const org of orgs) {
			expect(text, `the directory must show ${org.name}`).toContain(org.name);
		}

		// A machine identifier in a cell meant for an operator is a defect: the
		// name — not the id — is what a person reads and what a link labels.
		expect(text, 'the organizations table must not show a raw UUID').not.toMatch(UUID);

		// Long org names ("Puskesmas Prima Sejahtera Network") must not widen the
		// page; the table scrolls inside its own region instead.
		await expectNoHorizontalOverflow(page);
	});

	test('every row deep-links to a scoped overview with the four count labels', async ({
		page
	}) => {
		const orgs = await fetchScopedOrgs(page);

		for (const org of orgs) {
			await gotoAdmin(page, '/admin/organizations');

			// The row's name is the link's accessible label, and it points at THIS
			// organization — never a raw id spliced into the page as text.
			const link = page.getByRole('link', { name: org.name });
			await expect(link).toHaveAttribute('href', `/admin/organizations/${org.id}`);

			await link.click();
			await expect(page).toHaveURL(new RegExp(`/admin/organizations/${org.id}$`));

			// The frozen h1 for the overview destination.
			await expect(page.getByRole('heading', { level: 1 })).toHaveText(
				'Rincian Organisasi'
			);

			// The one labelled-lines block, never a set of comparison KPI tiles.
			const counts = page.locator('dl.sigap-org-overview__counts');
			await expect(counts).toBeVisible(SETTLE);
			await expect(counts.locator('dt')).toHaveCount(COUNT_LABELS.length);
			for (const label of COUNT_LABELS) {
				await expect(counts.getByText(label, { exact: true })).toBeVisible();
			}

			// Each label is paired with a numeric value (formatNumber) — a zero is
			// a number, an empty cell is a broken aggregate.
			const values = await counts.locator('dd').allTextContents();
			expect(values, 'every count label pairs with a value').toHaveLength(COUNT_LABELS.length);
			for (const value of values) {
				expect(value, `a count must render a number, got ${JSON.stringify(value)}`).toMatch(
					/\d/
				);
			}
		}
	});

	test('a forged organization id renders the denied state and leaks nothing', async ({
		page
	}) => {
		const orgs = await fetchScopedOrgs(page);

		// Any id that is not a seed proves the 404 wall; a freshly random one is
		// guaranteed to collide with nothing, including a future seed.
		await gotoAdmin(page, `/admin/organizations/${randomUUID()}`);

		// No crash: the page still renders its own heading, then the missing state.
		await expect(page.getByRole('heading', { level: 1 })).toHaveText('Rincian Organisasi');

		const alert = page.getByRole('alert');
		await expect(alert).toBeVisible(SETTLE);
		await expect(alert).toContainText('Organisasi tidak ditemukan');
		await expect(alert).toContainText('tidak dapat diakses oleh akun Anda');

		// The 404 must not distinguish a forged id from a sibling org, so the page
		// may not leak a machine identifier NOR any organization this actor can
		// actually see. Leaking the super-admin's own directory here would turn an
		// "org existence is private" door into an oracle.
		const body = ((await page.locator('body').textContent()) ?? '');
		expect(body, 'the denied page must not show a raw UUID').not.toMatch(UUID);
		for (const org of orgs) {
			expect(body, `the denied page must not leak ${org.name}`).not.toContain(org.name);
		}
	});

	test('the regional surface still exposes all seven nav destinations', async ({ page }) => {
		await gotoAdmin(page, '/admin/organizations');
		// Exactly seven links, matching the frozen sidebar: the new organizations
		// destination joined the six existing ones without displacing any.
		await expect(adminNav(page).getByRole('link')).toHaveCount(7);
	});
});