import { expect, test, type Page } from '@playwright/test';

/**
 * Phase 3B5.1 — T-3B5-03's capability refusal, end to end.
 *
 * WHY THIS NEEDS ITS OWN FILE AND ITS OWN ACTOR
 *
 * The schedules page distinguishes two states that a single actor cannot produce
 * at once, and the whole point of Phase 3B5.1 is that they must never collapse
 * into each other:
 *
 *   1. ZERO SCOPE. The actor has no facility grant at all. `listFacilities()`
 *      returns `[]`, `isEmptyScope` is true, and the page renders the class-1
 *      empty state: "you have no facilities in your scope".
 *   2. READABLE BUT NO MANAGE. The actor CAN read schedules at a facility, but
 *      holds `schedule.manage` nowhere. `listFacilities()` returns that facility,
 *      so `scopeEmpty` is FALSE — and `/api/v1/admin/schedules/options` answers
 *      200 with `facilities: []`, which the page renders as a ForbiddenPanel.
 *
 * Every pre-existing 3B5 actor lands in one of the OTHER three states:
 * `e2e-schedule-manager` manages; `e2e-schedule-mixed` manages at B;
 * `local-global-super-admin` manages everywhere. So state (2) was unobservable,
 * and the `capabilityDenied` branch in schedules/+page.svelte had no test at all.
 * `e2e-schedule-reader` exists solely to reach it, and this file runs only under
 * that subject.
 *
 * WHY THE REFUSAL MUST NOT BE A CLIENT-SIDE GUESS
 *
 * The page holds `hasSession` and nothing else — no permission list, no role, no
 * facility scope. It therefore cannot know whether the operator may manage
 * schedules, and it must not try: inferring capability from an email address or a
 * token claim is the exact authorization model Phase 3B0 removed. The only
 * honest source is the server's own answer to a DOMAIN question ("which
 * facilities may I offer as choices?"), so the assertion here is that the panel
 * appears exactly when the server says the list is empty AND scope is non-empty.
 *
 * The mutation is refused server-side as well (403), which this file asserts
 * directly. That is the property that makes the UI half meaningful: a control
 * hidden in the browser while the endpoint would have accepted the write is not a
 * capability boundary, it is a suggestion.
 */

const DESKTOP = { width: 1440, height: 900 };

/** Real round trips against a local API; the default timeout is too tight. */
const SETTLE = { timeout: 15_000 };

/** Any UUID. Used to prove a machine identifier never reaches the UI. */
const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

/**
 * The one subject that reaches the capability-refusal state.
 *
 * Seeded as `operator` at the demo facility: `operator` carries `schedule.read`
 * and NOT `schedule.manage`, so the schedules list is readable while the options
 * list is empty. See packages/db/seed/demo.sql for the self-verification that
 * fails the seed if `operator` ever gains `schedule.manage`.
 *
 * The gate is on the TEST PROCESS's copy of the pinned subject
 * (`SIGAP_E2E_ACTOR`), never on anything the page renders. Asking the page would
 * mean inferring the actor's grants from its own output, which is the inference
 * this whole matrix exists to avoid.
 */
const CAPABILITY_REFUSAL_ACTOR = 'e2e-schedule-reader';

const ACTOR = (process.env.SIGAP_E2E_ACTOR ?? '').trim();

async function gotoAdmin(page: Page, path: string) {
	await page.goto(path);
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
}

test.describe('T-3B5-03 capability refusal: readable schedules, no schedule.manage', () => {
	test.use({ viewport: DESKTOP });

	// Skipped, not failed, for every other actor — and skipped BEFORE any
	// assertion, because each of them is in a genuinely different state:
	// authorized, mixed-provenance, or zero-scope. Asserting any of this file's
	// claims under those actors would be asserting something false, not merely
	// something out of scope.
	test.skip(
		ACTOR !== CAPABILITY_REFUSAL_ACTOR,
		`the capability-refusal state is only reachable as ${CAPABILITY_REFUSAL_ACTOR}; this run is ${ACTOR || '(unset)'}`
	);

	/**
	 * The whole criterion, in one test, because the claims are only meaningful
	 * together. Asserting "ForbiddenPanel is visible" alone would pass against a
	 * page that renders it unconditionally, or against an ErrorState, or against
	 * the zero-scope empty panel — all of which are the specific wrong answers
	 * this state must be distinguished from.
	 */
	test('renders ForbiddenPanel for readable scope with zero manageable options', async ({
		page
	}) => {
		// ---- SERVER-AUTHORITATIVE PRECONDITIONS -------------------------------
		//
		// Read from the API through the same origin the page uses, so the
		// assertions below are about what the SERVER said rather than about what
		// the page chose to render. Each precondition gets its own message so a
		// failure names which of the three broke.

		const options = await page.request.get('/api/v1/admin/schedules/options');
		expect(options.status(), 'the options read must succeed, not 403').toBe(200);
		const optionFacilities = (
			(await options.json()).data as { facilities: unknown[] }
		).facilities;
		expect(
			Array.isArray(optionFacilities),
			'the options payload must carry a facilities array'
		).toBe(true);
		expect(
			optionFacilities,
			'this actor holds schedule.manage nowhere, so the list must be EMPTY. A non-empty ' +
				'list means the seed granted manage and the rest of this file would be testing ' +
				'the authorized state under a misleading name.'
		).toHaveLength(0);

		const readable = await page.request.get('/api/v1/admin/facilities');
		expect(readable.status()).toBe(200);
		const readableFacilities = (await readable.json()).data as Array<{ name: string }>;
		expect(
			readableFacilities.length,
			'this actor must hold a READABLE facility scope, or it is the zero-scope actor ' +
				'and the class-1 empty state would be correct instead'
		).toBeGreaterThan(0);

		const schedules = await page.request.get('/api/v1/admin/schedules');
		expect(schedules.status()).toBe(200);
		const scheduleRows = (await schedules.json()).data as unknown[];
		expect(
			scheduleRows.length,
			'this actor must be able to READ schedule rows. An empty list beside a refusal ' +
				'panel is indistinguishable from the ordinary no-rows state, so the test would ' +
				'prove much less than it claims.'
		).toBeGreaterThan(0);

		// ---- THE BROWSER ASSERTIONS -------------------------------------------

		await gotoAdmin(page, '/admin/schedules');

		// 1. Readable schedule context/scope exists, rendered.
		const table = page.locator('table');
		await expect(table).toBeVisible(SETTLE);
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		// The facility is named in a rendered ROW, which is only possible because
		// scope resolved.
		//
		// Scoped to the table rather than the whole page: the facility name also
		// appears in the filter's <option> list, and an <option> inside a closed
		// <select> computes as hidden, so a page-wide `.first()` would resolve to
		// an invisible node and fail against a perfectly correct page.
		await expect(
			table.getByText(readableFacilities[0].name).first(),
			'the rendered schedule rows must name the readable facility'
		).toBeVisible(SETTLE);

		// 2. The facility filter offers the readable facility, so the operator can
		//    see they DO have scope. A zero-scope actor gets no such option, which
		//    is what makes this different from state (1).
		const facilityFilter = page.getByLabel('Filter fasilitas');
		await expect(facilityFilter).toBeVisible(SETTLE);
		const filterOptions = await facilityFilter.locator('option').allTextContents();
		expect(
			filterOptions,
			'the readable facility must be offered by the filter'
		).toContain(readableFacilities[0].name);

		// 3. NOT the zero-scope empty state. The class-1 empty panel and the
		//    capability panel are different claims, so each one's own words are
		//    asserted absent.
		const body = ((await page.locator('body').textContent()) ?? '');
		expect(
			body,
			'the class-1 zero-scope empty state must NOT be rendered for a readable scope'
		).not.toContain('Belum ada jadwal');
		// `AdminReadState`'s scope-empty wording is the one that would appear if
		// `isEmptyScope(facilities)` were true, which it must not be here.
		expect(
			body,
			'a readable-scope actor must not be told they have no facilities in scope'
		).not.toMatch(/tidak memiliki fasilitas dalam lingkup|no facilities in scope/i);

		// 4. ForbiddenPanel IS rendered, with the copy the page supplies.
		const forbidden = page.locator('.sigap-forbidden');
		await expect(forbidden).toHaveCount(1);
		await expect(forbidden).toBeVisible(SETTLE);
		await expect(forbidden).toContainText('Tidak dapat mengelola jadwal', SETTLE);
		// The description explains the read/change split in the operator's terms.
		await expect(forbidden).toContainText('tidak dapat menambah atau mengubahnya', SETTLE);
		// It is an alert region, so it is announced rather than only displayed.
		await expect(forbidden).toHaveAttribute('role', 'alert');

		// 5. The mutation affordance cannot be used: no create control at all.
		await expect(
			page.getByRole('button', { name: 'Jadwal baru' }),
			'a refused actor must not be offered the create affordance'
		).toHaveCount(0);
		// And no per-row edit control either, which is the per-row half of the
		// same rule.
		await expect(
			table.getByRole('button', { name: 'Ubah' }),
			'a refused actor must not be offered a per-row edit affordance'
		).toHaveCount(0);

		// 6. No editor can be opened, so no form and no save control exist.
		await expect(page.getByRole('dialog')).toHaveCount(0);
		await expect(page.getByLabel('Durasi slot (menit)')).toHaveCount(0);
		await expect(page.getByRole('button', { name: /Simpan jadwal|Simpan perubahan/ })).toHaveCount(0);

		// 7. No raw permission name is exposed anywhere on the page.
		//
		// The panel deliberately does not name `schedule.manage`: the client is
		// not an authorization authority, and telling a user precisely what they
		// lack is information the server withholds on purpose. Asserting the
		// ABSENCE of the literal name is what proves the page is not leaking the
		// affordance read into its copy.
		for (const forbidden of ['schedule.manage', 'schedule_manage', 'scheduleManage']) {
			expect(
				body,
				`the page must not name the permission "${forbidden}"`
			).not.toContain(forbidden);
		}

		// 8. No role, scope, or super_admin data is exposed. Same reasoning: the
		//    options endpoint answers a DOMAIN question and carries no
		//    authorization vocabulary, and the page must not reintroduce any.
		for (const secret of ['super_admin', 'super admin', 'facility_admin', 'operator']) {
			expect(body, `the page must not name the role "${secret}"`).not.toContain(secret);
		}
		expect(
			body,
			'the page must not expose the authorization payload shape'
		).not.toMatch(/can_retry|can_cancel|can_manage|role_permissions/);
		expect(body, 'no raw UUID may be shown').not.toMatch(UUID);
	});

	/**
	 * The refusal is enforced by the SERVER, not merely by the browser.
	 *
	 * This is the half that makes the panel a capability boundary rather than a
	 * UI suggestion. A client that hid the button while the endpoint accepted the
	 * write would satisfy every assertion in the test above, and the operator
	 * would discover the real capability boundary only by ignoring the UI.
	 *
	 * The request is issued directly rather than through the form precisely
	 * because the form does not exist for this actor — so this is the only way to
	 * reach the server's own answer.
	 */
	test('the server refuses a schedule create for this actor, not just the UI', async ({
		page
	}) => {
		const readable = await page.request.get('/api/v1/admin/facilities');
		const facilities = (await readable.json()).data as Array<{ id: string; name: string }>;
		const units = await page.request.get('/api/v1/admin/service-units');
		expect(units.status()).toBe(200);
		const unitList = (await units.json()).data as Array<{ id: string; facility_id: string }>;
		const unit = unitList.find((candidate) => candidate.facility_id === facilities[0].id);
		expect(
			unit,
			`the readable facility must expose a service unit (saw ${unitList.length} units)`
		).toBeTruthy();

		// A body the backend itself considers well formed: every required field
		// present and valid. A rejection of THIS body is a policy refusal, not a
		// validation error, which is what makes it evidence.
		const response = await page.request.post('/api/v1/admin/schedules', {
			data: {
				facility_id: facilities[0].id,
				service_unit_id: unit!.id,
				schedule_date: '2026-12-24',
				start_time: '09:00:00',
				end_time: '12:00:00',
				slot_minutes: 60,
				capacity_per_slot: 2
			}
		});

		// 403 is the authorization answer. 400 would mean the body was rejected
		// first, which would prove nothing about permissions.
		expect(
			response.status(),
			'a well-formed create must be refused on AUTHORIZATION, not validation'
		).toBe(403);

		// And nothing was persisted: a refusal that still wrote a row would be a
		// worse defect than one that returned the wrong status.
		const after = await page.request.get('/api/v1/admin/schedules');
		const rows = (await after.json()).data as Array<{ schedule_date: string }>;
		expect(
			rows.filter((row) => row.schedule_date.startsWith('2026-12-24')),
			'a refused create must leave no row'
		).toHaveLength(0);
	});

	/**
	 * The two states must not collapse.
	 *
	 * Asserted as its own test rather than folded into the one above, because the
	 * failure mode is a REGRESSION in either direction and each needs its own
	 * message: an empty-scope actor being shown a capability panel (a false
	 * "you cannot manage" to someone who cannot even see anything), or a
	 * readable-scope actor being shown the empty state (a false "you have no
	 * facilities" to someone who demonstrably has one).
	 *
	 * The zero-scope comparison is made through the API rather than by running
	 * the suite as that actor, so both halves are visible in one place.
	 */
	test('the refusal rests on empty OPTIONS, not on an empty SCOPE', async ({ page }) => {
		const readable = await page.request.get('/api/v1/admin/facilities');
		const readableFacilities = (await readable.json()).data as Array<{ name: string }>;
		const options = await page.request.get('/api/v1/admin/schedules/options');
		const optionFacilities = ((await options.json()).data as { facilities: unknown[] })
			.facilities;

		// Stated as a relationship rather than two literals, so the test fails if
		// either side changes independently.
		expect(
			readableFacilities.length > 0 && optionFacilities.length === 0,
			`the capability-refusal state is "scope=${readableFacilities.length}, ` +
				`manageable=${optionFacilities.length}"; it requires a non-empty scope and an ` +
				'empty manageable list, and neither state alone is sufficient'
		).toBe(true);

		// The options payload itself carries no authorization vocabulary. It
		// answers a domain question, so a permission or role name appearing here
		// would mean the endpoint had started leaking the model it deliberately
		// withholds.
		const raw = await (await page.request.get('/api/v1/admin/schedules/options')).text();
		for (const secret of ['permission', 'role', 'super_admin', 'schedule.manage', 'grant']) {
			expect(raw, `the options payload must not contain "${secret}"`).not.toContain(secret);
		}
	});
});