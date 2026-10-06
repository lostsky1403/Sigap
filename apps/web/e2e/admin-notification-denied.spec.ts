import type { Page } from '@playwright/test';
import { expect, test } from './support/test';

/**
 * Phase 3B6, T-3B6-01 — the notification DENIAL, end to end.
 *
 * WHY THIS NEEDS ITS OWN FILE AND ITS OWN ACTOR
 *
 * The outbox distinguishes two states that the mutation suite cannot show at
 * once, and the whole point of the capability model is that they must never
 * collapse into each other:
 *
 *   1. ACTIONABLE. The actor holds `notification.manage` at the facility a row
 *      belongs to, and the row's status permits the action. The server answers
 *      `can_retry: true` / `can_cancel: true` and the table draws the control.
 *   2. DENIED. The actor can READ the outbox — the rows are genuinely in scope
 *      and visible — but holds `notification.manage` nowhere, so the server
 *      answers `false` for both booleans on every row.
 *
 * `admin-mutations.spec.ts` covers (1) for the managing actors. For (2) it has
 * only a read-only BRANCH inside a larger test, and that branch asserts the two
 * booleans and the absence of the controls — it does not assert the two things
 * that make a denial safe: that the recipient stays MASKED while being denied,
 * and that no authorization vocabulary leaks into the page. A denial that
 * disclosed a role, a permission name, or a facility scope would be an
 * existence oracle dressed as a refusal.
 *
 * So this file asserts the full denial contract under the one actor that can
 * reach it: `e2e-schedule-reader`, seeded as `operator` at the demo facility.
 * `operator` carries `notification.read` and NOT `notification.manage`, and
 * the demo facility is where the seeded outbox rows live — which is what makes
 * "readable, unactionable, and non-empty" observable at all. An actor with no
 * scope would render the class-1 empty state instead, and a denial beside an
 * empty table proves almost nothing.
 *
 * WHY THE REFUSAL MUST COME FROM THE SERVER
 *
 * The page holds `hasSession` and nothing else: no permission list, no role, no
 * facility scope. It cannot know whether the operator may retry a notification,
 * and it must not try — inferring capability from an email address or a token
 * claim is the authorization model Phase 3B0 removed. `can_retry` and
 * `can_cancel` are the server's answer, computed at each row's OWN stored
 * facility, and the controls follow them and nothing else.
 *
 * The mutation is refused server-side as well, which this file asserts
 * directly. That is the property that makes the UI half meaningful: a control
 * hidden in the browser while the endpoint would have accepted the write is not
 * a capability boundary, it is a suggestion.
 */

const DESKTOP = { width: 1440, height: 900 };

/** Real round trips against a local API; the default timeout is too tight. */
const SETTLE = { timeout: 15_000 };

/** Any UUID. Used to prove a machine identifier never reaches the UI. */
const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

/**
 * The one subject that reaches the notification-denial state.
 *
 * Seeded as `operator` at the demo facility: `operator` carries
 * `notification.read` and NOT `notification.manage`, so the outbox is readable
 * while every row is unactionable. See packages/db/seed/rbac.sql for the role
 * grant and packages/db/seed/demo.sql for the facility assignment.
 *
 * The gate is on the TEST PROCESS's copy of the pinned subject
 * (`SIGAP_E2E_ACTOR`), never on anything the page renders. Asking the page
 * would mean inferring the actor's grants from its own output, which is the
 * inference this whole matrix exists to avoid.
 */
const DENIED_ACTOR = 'e2e-schedule-reader';

const ACTOR = (process.env.SIGAP_E2E_ACTOR ?? '').trim();

async function gotoAdmin(page: Page, path: string) {
	await page.goto(path);
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
}

/** One outbox row as the wire carries it, including the two affordances. */
interface WireRow {
	id: string;
	status: string;
	recipient_contact_masked: string;
	can_retry: boolean;
	can_cancel: boolean;
}

test.describe('T-3B5-05 notification denial: readable outbox, no notification.manage', () => {
	test.use({ viewport: DESKTOP });

	// Skipped, not failed, for every other actor — and skipped BEFORE any
	// assertion, because each of them is in a genuinely different state:
	// managing, mixed-provenance, or zero-scope. Asserting any of this file's
	// claims under those actors would be asserting something false, not merely
	// something out of scope.
	test.skip(
		ACTOR !== DENIED_ACTOR,
		`the notification-denial state is only reachable as ${DENIED_ACTOR}; this run is ${ACTOR || '(unset)'}`
	);

	/**
	 * The whole criterion, in one test, because the claims are only meaningful
	 * together.
	 *
	 * Asserting "no Retry button" alone would pass against a page that rendered
	 * no table at all, or against the zero-scope empty state, or against a
	 * failed read — all of which are the specific wrong answers this state must
	 * be distinguished from. So the test first establishes, from the API, that
	 * there ARE rows to deny, and only then asserts what the page does with
	 * them.
	 */
	test('offers no retry or cancel, keeps the recipient masked, and leaks no authorization vocabulary', async ({
		page
	}) => {
		// ---- SERVER-AUTHORITATIVE PRECONDITIONS -------------------------------

		const listed = await page.request.get('/api/v1/admin/notifications');
		expect(listed.status(), 'the outbox read must succeed, not be refused').toBe(200);
		const rows = (await listed.json()).data as WireRow[];

		expect(
			rows.length,
			'this actor must be able to READ outbox rows. An empty list beside a denial is ' +
				'indistinguishable from the ordinary no-rows state, so the test would prove far ' +
				'less than it claims.'
		).toBeGreaterThan(0);

		for (const row of rows) {
			expect(
				row.can_retry,
				`row ${row.id} must not be retryable without notification.manage`
			).toBe(false);
			expect(
				row.can_cancel,
				`row ${row.id} must not be cancellable without notification.manage`
			).toBe(false);
		}

		// The scope is genuinely non-empty, which is what separates this state
		// from the zero-scope actor's class-1 empty panel.
		const readable = await page.request.get('/api/v1/admin/facilities');
		expect(readable.status()).toBe(200);
		const readableFacilities = (await readable.json()).data as Array<{ name: string }>;
		expect(
			readableFacilities.length,
			'this actor must hold a READABLE facility scope, or it is the zero-scope actor and ' +
				'the empty state would be the correct rendering instead'
		).toBeGreaterThan(0);

		// ---- THE BROWSER ASSERTIONS -------------------------------------------

		await gotoAdmin(page, '/admin/notifications');

		const table = page.locator('table');
		await expect(table).toBeVisible(SETTLE);
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		// 1. NOT the zero-scope empty state. The class-1 panel is a different
		//    claim ("you have no facilities"), so its own words are asserted
		//    absent.
		const body = (await page.locator('body').textContent()) ?? '';
		expect(
			body,
			'a readable-scope actor must not be told they have no facilities in scope'
		).not.toMatch(/tidak memiliki fasilitas dalam lingkup|no facilities in scope/i);

		// 2. NO ACTION CONTROLS AT ALL. Both halves of the pair, counted across
		//    the whole table rather than per row, so a single leaked control
		//    anywhere fails this.
		await expect(
			table.getByRole('button', { name: 'Kirim ulang' }),
			'a denied actor must not be offered a retry control'
		).toHaveCount(0);
		await expect(
			table.getByRole('button', { name: 'Batalkan' }),
			'a denied actor must not be offered a cancel control'
		).toHaveCount(0);
		// And no other action-shaped control crept in to replace them.
		expect(
			await table.getByRole('button').count(),
			'a fully unactionable outbox must render no buttons in the table at all'
		).toBe(0);

		// 3. The row says so in words. An empty action cell reads as a failed
		//    load, so the page states the absence deliberately.
		await expect(table).toContainText('Tidak ada aksi');

		// 4. THE RECIPIENT STAYS MASKED WHILE BEING DENIED. This is the claim
		//    the mutation suite's read-only branch does not make. A denial that
		//    disclosed the real contact would be worse than the action it
		//    withheld.
		const tableText = (await table.textContent()) ?? '';
		expect(
			tableText,
			'the masked recipient must still be rendered for a denied actor'
		).toMatch(/•/);
		expect(
			tableText,
			'no raw phone number may be shown'
		).not.toMatch(/\+62\d{9,}/);
		expect(
			tableText,
			'no raw email may be shown'
		).not.toMatch(/[\w.+-]+@[\w-]+\.[\w.]+/);
		expect(tableText.toLowerCase(), 'no internal dedup hash may be shown').not.toContain('hash');
		expect(tableText, 'no raw UUID may be shown').not.toMatch(UUID);

		// 5. NO AUTHORIZATION VOCABULARY. The two booleans are a wire contract,
		//    not UI copy, and the page must not name the permission, the grant
		//    model, or the role identifiers it was refused by. Telling a user
		//    precisely what they lack is information the server withholds on
		//    purpose.
		//
		//    The bare word "operator" is deliberately NOT in this list. It is
		//    ordinary Indonesian for "the staff member using this screen" and
		//    the page legitimately says "lingkup operator saat ini" in its
		//    empty-state copy. Banning the word would forbid correct UI text and
		//    would fail for a reason that has nothing to do with a leak. The
		//    role is instead covered by its unambiguous identifier forms below.
		const pageText = ((await page.locator('body').textContent()) ?? '').toLowerCase();
		for (const forbidden of [
			'can_retry',
			'can_cancel',
			'notification.manage',
			'notification_manage',
			'notificationmanage',
			'super_admin',
			'super admin',
			'facility_admin',
			'facilityadmin',
			'unrestricted',
			'facilitygrant',
			'facility_grant',
			'role_permissions',
			'permission'
		]) {
			expect(
				pageText,
				`the page must not name "${forbidden}" — the denial must not become an oracle`
			).not.toContain(forbidden);
		}
	});

	/**
	 * The refusal is enforced by the SERVER, not merely by the browser.
	 *
	 * This is the half that makes the hidden button a capability boundary rather
	 * than a UI suggestion. A client that hid the control while the endpoint
	 * accepted the write would satisfy every assertion above, and the operator
	 * would discover the real boundary only by ignoring the UI.
	 *
	 * The request is issued directly rather than through a control, precisely
	 * because no control exists for this actor — so this is the only way to
	 * reach the server's own answer.
	 *
	 * TWO LAYERS, AND THIS ACTOR MEETS THE OUTER ONE.
	 *
	 * The admin surface authorizes twice, in a fixed order:
	 *
	 *   1. The ROUTE GATE (`identity.RequirePermission`, driven by
	 *      `router.Registry`) requires the declared policy for the path —
	 *      `notification.manage` for a POST under
	 *      `/api/v1/admin/notifications/`. It consults the actor's permission
	 *      union and answers 403 when it is absent.
	 *   2. The HANDLER then re-authorizes per row, requiring
	 *      `notification.manage` at the row's OWN stored facility, and answers
	 *      404 (not 403) so the refusal cannot be used to enumerate which rows
	 *      are actionable.
	 *
	 * This actor holds `notification.manage` NOWHERE, so the gate refuses first
	 * and the handler never runs. 403 is therefore the correct expectation, and
	 * asserting 404 here would be asserting a layer this actor cannot reach.
	 * The handler's 404 is exercised by the managing actors, whose union
	 * satisfies the gate and whose per-facility grants are then checked.
	 */
	test('the server refuses retry and cancel on every visible row for this actor', async ({
		page
	}) => {
		const listed = await page.request.get('/api/v1/admin/notifications');
		const rows = (await listed.json()).data as WireRow[];
		expect(rows.length, 'there must be rows to attempt against').toBeGreaterThan(0);

		/*
		 * EVERY row, and both actions, rather than one sampled row. The gate is
		 * path-based so it cannot vary per row, but issuing the request against
		 * each row proves the refusal does not depend on the row's status —
		 * which is what rules out a state-based explanation for the denial.
		 */
		for (const row of rows) {
			for (const action of ['retry', 'cancel'] as const) {
				const response = await page.request.post(
					`/api/v1/admin/notifications/${row.id}/${action}`
				);
				expect(
					response.status(),
					`${action} on row ${row.id} (status ${row.status}) must be refused for an actor ` +
						`without notification.manage`
				).toBe(403);

				// The gate's refusal must not name the permission it wanted.
				// A body that said "requires notification.manage" would turn
				// every refusal into a description of the authorization model,
				// which §12 forbids the browser from receiving.
				const body = await response.text();
				for (const leak of ['notification.manage', 'notification_manage', 'super_admin']) {
					expect(
						body.toLowerCase(),
						`the refusal body must not name "${leak}"`
					).not.toContain(leak);
				}
			}
		}

		// And nothing changed. A refusal that still wrote would be a worse
		// defect than one that returned the wrong status.
		//
		// Checked PER ROW rather than by comparing totals: other spec files
		// append to this outbox while this one runs (a booking or a check-in
		// enqueues a confirmation), so the total is not a stable number. Each
		// row this test attempted against must still exist with the status it
		// had, which is the property the refusal has to preserve.
		const after = await page.request.get('/api/v1/admin/notifications');
		const afterRows = (await after.json()).data as WireRow[];
		for (const row of rows) {
			const unchanged = afterRows.find((candidate) => candidate.id === row.id);
			expect(unchanged, `row ${row.id} must still exist`).toBeTruthy();
			expect(unchanged!.status, `row ${row.id} must be unchanged`).toBe(row.status);
		}
	});
});
