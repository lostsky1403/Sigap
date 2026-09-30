import { expect, test, type Locator, type Page } from '@playwright/test';

/**
 * Phase 3B5 §9/§10: facility create, update, and one-way deactivation.
 *
 * WHY THIS IS A SEPARATE FILE FROM admin-mutations.spec.ts
 *
 * Not for tidiness — because facility CREATE is the one admin mutation whose
 * authorization is GLOBAL rather than facility-scoped, and that single fact
 * dictates which database actor can run it.
 *
 * `AdminHandler.CreateFacility` requires `scope.Unrestricted` and answers 404
 * "Fasilitas tidak ditemukan." otherwise. That is deliberate: creating a
 * facility mints a resource no facility-scoped administrator could be
 * responsible for, so only an actor holding an unscoped `super_admin`
 * assignment may do it. Update and deactivate differ — they go through
 * `AuthorizeFacilityMutation`, which needs an in-scope facility AND
 * permission-at-that-facility, so a `facility_admin` may change a facility
 * inside its own scope.
 *
 * The consequence for the E2E is that one actor cannot cover both halves:
 *
 *   - the harness default (`e2e-schedule-manager`) is `facility_admin` at the
 *     demo facility, so it can update and deactivate there but can never
 *     create. It is also the WRONG actor to deactivate: the demo facility is
 *     load-bearing for the queue, appointment, and schedule suites, and
 *     deactivation is one-way with no reverse operation.
 *   - `local-global-super-admin` holds `super_admin` with a NULL facility, so
 *     it is the only seeded subject that can create a facility — and creating
 *     a DISPOSABLE one is what lets the deactivation test run without
 *     destroying state the rest of the suite depends on.
 *
 * So this file runs its real happy-path chain under the global actor, and
 * asserts the REAL refusal under a scoped actor. Both are genuine outcomes of
 * the genuine authorization model; neither is mocked, and neither is faked by
 * loosening a check. `Start-LocalE2E.ps1` runs the suite once per actor.
 *
 * WHY THE ACTOR IS READ FROM THE HARNESS, NOT FROM THE PAGE
 *
 * `process.env.SIGAP_E2E_ACTOR` is the same value the web tier hands the API as
 * `X-Sigap-Local-Test-Subject`, read here in the TEST PROCESS. Asking the page
 * instead would mean inferring capability from rendered output, and — worse —
 * would tempt the test into asserting on a permission list that the browser is
 * never allowed to see. The browser holds `hasSession` and nothing else; the
 * server decides what this subject may do, on every request, independently.
 */

const DESKTOP = { width: 1440, height: 900 };

/** Real round trips against a local API; the default timeout is too tight. */
const SETTLE = { timeout: 15_000 };

/** The one seeded subject holding an unscoped `super_admin` assignment. */
const UNRESTRICTED_ACTOR = 'local-global-super-admin';

/** The actor this Playwright process is running against. */
const ACTOR = (process.env.SIGAP_E2E_ACTOR ?? '').trim();

/** True when the configured actor may create a facility. */
const CAN_CREATE = ACTOR === UNRESTRICTED_ACTOR;

/**
 * True when the configured actor is scoped to nothing at all.
 *
 * Such an actor correctly sees an empty, table-less admin. Every assertion in
 * this file is about acting ON a facility, so there is nothing for it to act on
 * and the specs skip rather than pretend otherwise. Its denial is asserted as a
 * positive claim in admin-read.spec.ts — empty state shown, no table, no leaked
 * identifier — which is a stronger statement than "these mutation tests did not
 * apply".
 */
const HOLDS_NO_DATA = ACTOR === 'local-zero-scope-admin';

/**
 * True when the configured actor holds `facility.manage` at a SCOPED facility.
 *
 * THIS IS THE ACTOR SET THAT CAN REACH THE 404 BRANCH, and it is derived from
 * the seed rather than from a capability probe, for the reason the test body
 * explains: `POST /api/v1/admin/facilities` is gated twice — the route registry
 * demands `facility.manage`, and only the handler then demands
 * `scope.Unrestricted` and answers 404. An actor can only observe the second
 * gate by passing the first, so this set is exactly the actors holding
 * `facility.manage` under a facility-scoped `user_roles` row:
 *
 *   - `e2e-schedule-manager`  (d993) facility_admin @ demo facility
 *   - `e2e-schedule-mixed`    (d994) facility_admin @ demo facility, plus
 *                                        operator @ a second facility
 *   - `local-facility-admin`  (d991) facility_admin @ demo facility
 *
 * The unscoped actors are deliberately absent. `local-global-super-admin` is
 * already skipped by `CAN_CREATE` because it is the one actor that SUCCEEDS;
 * `local-zero-scope-admin` is skipped by `HOLDS_NO_DATA` because it holds no
 * row to act on. `e2e-schedule-reader` is absent because it is `operator`,
 * which carries no `facility.manage` at all — the server refuses it at the
 * registry, which is a different and equally correct claim.
 */
const HOLDS_FACILITY_MANAGE = [
	'e2e-schedule-manager',
	'e2e-schedule-mixed',
	'local-facility-admin'
].includes(ACTOR);

/**
 * The facility this run creates, named so a re-run against a retained database
 * cannot collide with the previous run's row.
 *
 * The short code is what the tests look the row up by, and it is what the
 * backend's uniqueness constraint would reject a duplicate of, so it carries
 * the same clock-derived uniqueness as the phone numbers in the main spec.
 *
 * THE NAME IS DERIVED PER RUN, NOT A LITERAL. This suite is executed once per
 * actor in the matrix, against ONE database, so a fixed facility name would be
 * created twice — once per actor that can create — and the second row would
 * make an unrelated citizen test that filters the public catalog by name
 * resolve to two elements instead of one. That is not a hypothetical: it is
 * exactly how the first matrix run failed three citizen tests. Deriving the
 * name keeps the row identifiable AND unique, so the suite is re-runnable and
 * cannot leak into another spec's assumptions.
 */

/**
 * Derives the per-run digits, rejecting any tag that would collide with a
 * forbidden substring elsewhere.
 *
 * WHY THE FILTER EXISTS. The tag is derived from the clock, so it is arbitrary
 * digits — and an arbitrary digit string will occasionally contain "429", which
 * is one of the strings the §14 rate-limit spec asserts is absent from every
 * admin page. A tag of 76429624 therefore made `/admin/facilities` fail that
 * spec through no fault of the page: the digits were inside a facility NAME the
 * page rendered correctly.
 *
 * That is a real flake with a real trigger, not a hypothetical, and it was
 * observed on a matrix run. Two things are worth separating:
 *
 *   - the rate-limit spec's INTENT, which is that no page claims a 429 state, and
 *   - its METHOD, which greps raw page text for the digits "429".
 *
 * The intent is legitimate and stays. The method is what needs help: a three-digit
 * HTTP status is a word in this UI's language, not a number that may appear inside
 * an identifier. Rather than weaken the assertion, this makes the test DATA safe —
 * the same way a fixture avoids a reserved character — so the spec can only fail
 * for the reason it was written to catch.
 *
 * Regenerating until the tag is clean keeps the uniqueness guarantee intact; it
 * only skips tags that would be ambiguous to a substring search. The attempt
 * counter is part of the tag, so two runs in the same millisecond still differ.
 */
function runTagWithout(substring: string): string {
	for (let attempt = 0; attempt < 1000; attempt += 1) {
		const tag = `${String(Date.now()).slice(-6)}${String(ACTOR.length)}${attempt}`;
		if (!tag.includes(substring)) return tag;
	}
	throw new Error(`could not derive a run tag free of "${substring}"`);
}

/** The digits actually used, filtered so they cannot spoof a status code. */
const SAFE_RUN_TAG = runTagWithout('429');

test.describe.configure({ mode: 'serial' });

/** A short code within the backend's length rule, unique to this run. */
function shortCode(): string {
	return `E2E${SAFE_RUN_TAG}`;
}

/** The facility this run creates, before the rename. */
const CREATED_NAME = `Klinik E2E Sementara ${SAFE_RUN_TAG}`;

/** The facility after the update test renames it. */
const RENAMED_NAME = `Klinik E2E Diperbarui ${SAFE_RUN_TAG}`;

async function gotoAdmin(page: Page, path: string) {
	await page.goto(path);
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
}

/**
 * The one open <dialog>, by role.
 *
 * Asserting on the role is what proves the element is exposed as a dialog to
 * assistive technology; a class-name query cannot show that.
 */
function dialog(page: Page): Locator {
	return page.getByRole('dialog');
}

/** The polite live region every admin mutation announces through. */
function announcer(page: Page): Locator {
	return page.locator('.sigap-admin-announcer');
}

/** Reads a facility back from the server, by short code. */
async function facilityByShortCode(
	page: Page,
	code: string
): Promise<Record<string, unknown> | undefined> {
	const listed = await page.request.get('/api/v1/admin/facilities');
	expect(listed.ok(), 'the facility list must be readable').toBe(true);
	const data = (await listed.json()).data as Array<Record<string, unknown>>;
	return data.find((facility) => facility.short_code === code);
}

test.describe('T-3B5-04 facility create, update, and one-way deactivate', () => {
	test.use({ viewport: DESKTOP });

	/**
	 * The refusal a facility-scoped actor gets, asserted for real.
	 *
	 * This is not a consolation prize. §1 requires that a mutation endpoint
	 * authorize independently of anything the UI shows, and this is the direct
	 * evidence: the create form is fully rendered and submittable, the actor can
	 * genuinely manage facilities inside its own scope, and the server STILL
	 * refuses to mint a new one. A client that trusted its own affordances would
	 * let the operator fill in nine fields and lose them.
	 */
	test('a facility-scoped actor is refused, and the refusal is the server\'s', async ({
		page
	}) => {
		test.skip(CAN_CREATE, 'the configured actor may create facilities');
		// A zero-scope actor holds no facility rows, so the list precondition below
		// cannot be met. Its denial is already asserted in admin-read.spec.ts, and
		// duplicating it here would prove the same thing twice.
		test.skip(
			HOLDS_NO_DATA,
			'this actor holds no facility rows; its denial is asserted in admin-read.spec.ts'
		);

		// THE 404 PREMISE IS A PERMISSION-SHAPED CLAIM, not a generic one.
		//
		// `POST /api/v1/admin/facilities` is gated TWICE, in this order:
		//
		//   1. the route registry requires `facility.manage`, and
		//   2. the handler then requires `scope.Unrestricted`, answering 404
		//      "Fasilitas tidak ditemukan." otherwise.
		//
		// Only an actor that clears gate 1 ever reaches gate 2. A facility-scoped
		// actor that does NOT hold `facility.manage` is therefore refused at the
		// registry with 403 "Akses ditolak: izin tidak mencukupi." and never
		// produces the not-found-shaped refusal this test is written to assert.
		//
		// Asserting the 404 unconditionally would be asserting an implementation
		// detail of the route table, and it would be asserting something FALSE for
		// an actor that simply lacks the permission: the server is not claiming the
		// facility is missing, it is saying the actor may not create one. Those are
		// different facts and the test must only claim the one the server actually
		// makes. The actor that proves the 404 branch is the one that HOLDS
		// `facility.manage` and is still facility-scoped — the same actor the file
		// header names as the subject of this test.
		test.skip(
			!HOLDS_FACILITY_MANAGE,
			'this actor is refused at the route gate (no facility.manage) before the handler runs; the 404 branch is unreachable for it'
		);

		await gotoAdmin(page, '/admin/facilities');
		await expect(page.locator('table tbody tr').first()).toBeVisible(SETTLE);

		await page.getByRole('button', { name: 'Fasilitas baru' }).click();
		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		// EVERY required field is filled. This is the point of the test: a
		// half-filled form is refused by the CLIENT's own validation, which
		// proves nothing about the server. The refusal under examination is the
		// one that arrives after a body the backend itself considers well formed.
		await modal.getByLabel('Nama fasilitas').fill('Klinik E2E Ditolak');
		await modal.getByLabel('Tipe').selectOption('puskesmas');
		await modal.getByLabel('Jalan').fill('Jl. Uji Otomatis No. 1');
		await modal.getByLabel('Kecamatan').fill('Cibinong');
		await modal.getByLabel('Kabupaten/Kota').fill('Bogor');
		await modal.getByLabel('Provinsi').fill('Jawa Barat');
		await modal.getByLabel('Nomor telepon').fill('08123456789');
		await modal.getByLabel('Total tempat tidur').fill('12');
		await modal.getByLabel('Tempat tidur tersedia').fill('7');
		await modal.getByLabel('Kode singkat').fill(shortCode());
		await modal.getByRole('button', { name: 'Simpan', exact: true }).click();

		// The dialog stays open. Closing it would tell the operator the facility
		// was saved, which is the one thing that did not happen.
		await expect(modal, 'a refused create must not close the editor').toBeVisible(SETTLE);

		// The server's own words, not a client-invented authorization message.
		const refused = page.getByRole('alert').first();
		await expect(refused).toContainText('Fasilitas tidak ditemukan', { timeout: SETTLE.timeout });

		// And nothing was created.
		expect(
			await facilityByShortCode(page, shortCode()),
			'a refused create must leave no row'
		).toBeUndefined();
	});

	test('creates a facility that persists with the typed bed count', async ({ page }) => {
		test.skip(!CAN_CREATE, 'facility creation requires the unrestricted DB actor');

		await gotoAdmin(page, '/admin/facilities');
		await expect(page.locator('table tbody tr').first()).toBeVisible(SETTLE);

		await page.getByRole('button', { name: 'Fasilitas baru' }).click();
		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		await modal.getByLabel('Nama fasilitas').fill(CREATED_NAME);
		await modal.getByLabel('Tipe').selectOption('puskesmas');
		await modal.getByLabel('Jalan').fill('Jl. Uji Otomatis No. 1');
		await modal.getByLabel('Kecamatan').fill('Cibinong');
		await modal.getByLabel('Kabupaten/Kota').fill('Bogor');
		await modal.getByLabel('Provinsi').fill('Jawa Barat');
		await modal.getByLabel('Nomor telepon').fill('08123456789');
		// `available_beds` is the field most likely to be dropped between the
		// form, the request body, and the column list, so it is asserted rather
		// than assumed.
		await modal.getByLabel('Total tempat tidur').fill('12');
		await modal.getByLabel('Tempat tidur tersedia').fill('7');
		await modal.getByLabel('Kode singkat').fill(shortCode());

		await modal.getByRole('button', { name: 'Simpan', exact: true }).click();
		await expect(modal).toHaveCount(0, SETTLE);

		// Re-read through the same origin. Asserting against the table the page
		// just patched would prove only that the component re-rendered.
		const created = await facilityByShortCode(page, shortCode());
		expect(created, 'the created facility must be readable back').toBeTruthy();
		expect(created!.name).toBe(CREATED_NAME);
		expect(created!.total_beds).toBe(12);
		expect(created!.available_beds).toBe(7);
		expect(created!.is_active).toBe(true);

		// §17: the success channels. All three, because they serve three
		// different consumers.
		const live = announcer(page);
		await expect(live).toHaveAttribute('aria-live', 'polite');
		await expect(live).toContainText(/Fasilitas/i, { timeout: SETTLE.timeout });
		await expect(page.locator('.sigap-toast')).toBeVisible(SETTLE);
	});

	test('updates a facility and persists only the changed field', async ({ page }) => {
		test.skip(!CAN_CREATE, 'depends on the facility the create test made');

		await gotoAdmin(page, '/admin/facilities');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const row = table.locator('tr', { has: page.getByText(CREATED_NAME) }).first();
		await expect(row, 'the created row must be on the page').toBeVisible(SETTLE);

		// The method and the body are part of the contract, not implementation
		// detail: §9 says update through the shared client, and a body that
		// re-sent every field would overwrite a concurrent change.
		const bodies: Array<{ method: string; body: string }> = [];
		page.on('request', (request) => {
			if (
				(request.method() === 'PATCH' || request.method() === 'PUT') &&
				request.url().includes('/admin/facilities/')
			) {
				bodies.push({ method: request.method(), body: request.postData() ?? '' });
			}
		});

		await row.getByRole('button', { name: 'Ubah' }).click();
		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);
		// Seeded from the server's values, not from local state.
		await expect(modal.getByLabel('Nama fasilitas')).toHaveValue(CREATED_NAME);
		await expect(modal.getByLabel('Total tempat tidur')).toHaveValue('12');

		await modal.getByLabel('Nama fasilitas').fill(RENAMED_NAME);
		await modal.getByRole('button', { name: 'Simpan', exact: true }).click();
		await expect(modal).toHaveCount(0, SETTLE);

		expect(bodies.length, 'exactly one update request').toBe(1);
		expect(bodies[0].method, 'update uses PATCH').toBe('PATCH');
		expect(Object.keys(JSON.parse(bodies[0].body)), 'only the changed field is sent').toEqual([
			'name'
		]);

		const afterUpdate = await page.request.get('/api/v1/admin/facilities');
		const updated = ((await afterUpdate.json()) as {
			data: Array<{ name: string; total_beds: number }>;
		}).data.find((facility) => facility.name === RENAMED_NAME);
		expect(updated, 'the rename must be persisted').toBeTruthy();
		// The untouched field survived the diff-only update.
		expect(updated!.total_beds).toBe(12);
	});

	test('rejects a phone containing the characters the backend forbids', async ({ page }) => {
		// This drives the CREATE form, so it belongs to the unrestricted actor. The
		// refusal under test is a field-level validation, which is only reachable
		// by an actor allowed to open that form at all.
		test.skip(!CAN_CREATE, 'facility creation requires the unrestricted DB actor');

		await gotoAdmin(page, '/admin/facilities');
		await expect(page.locator('table tbody tr').first()).toBeVisible(SETTLE);

		await page.getByRole('button', { name: 'Fasilitas baru' }).click();
		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		await modal.getByLabel('Nama fasilitas').fill('Klinik E2E Telepon');
		await modal.getByLabel('Nomor telepon').fill('0812<script>');
		await modal.getByRole('button', { name: 'Simpan', exact: true }).click();

		// The dialog stays open and names the field. A closed dialog would let
		// the operator believe an unsaved value had been stored.
		await expect(modal).toBeVisible(SETTLE);
		await expect(modal).toContainText('Nomor telepon', SETTLE);

		const listed = await page.request.get('/api/v1/admin/facilities');
		const injected = ((await listed.json()).data as Array<{ phone: string }>).filter(
			(facility) => facility.phone.includes('script')
		);
		expect(injected, 'the rejected value must not reach the database').toHaveLength(0);
	});

	test('deactivates with PATCH, is one-way, and states the public impact', async ({ page }) => {
		test.skip(!CAN_CREATE, 'depends on the facility the create test made');

		await gotoAdmin(page, '/admin/facilities');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		// §10 names PATCH explicitly, so the method is asserted rather than
		// assumed: a POST would be a different endpoint.
		const methods: string[] = [];
		page.on('request', (request) => {
			if (request.url().includes('/deactivate')) methods.push(request.method());
		});

		const row = table.locator('tr', { has: page.getByText(RENAMED_NAME) }).first();
		await expect(row).toBeVisible(SETTLE);
		await row.getByRole('button', { name: 'Nonaktifkan' }).click();

		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		// §10's exact requirement: the dialog states the approved public impact,
		// in the words a resident would recognise, and says the action is final.
		await expect(modal).toContainText('tidak lagi muncul untuk warga');
		await expect(modal).toContainText('tidak dapat dibatalkan');

		await modal.getByRole('button', { name: 'Ya, nonaktifkan' }).click();
		await expect(modal).toHaveCount(0, SETTLE);

		expect(methods, 'deactivation must use PATCH').toEqual(['PATCH']);

		// The inactive state renders, with the one-way consequence on the row.
		const inactive = table.locator('tr', { has: page.getByText(RENAMED_NAME) }).first();
		await expect(inactive).toContainText('Nonaktif', { timeout: SETTLE.timeout });
		await expect(inactive).toContainText('Penonaktifan bersifat permanen', SETTLE);

		// ONE-WAY. There is no reverse operation, so a reactivate control would
		// be a button that exists only to fail.
		await expect(inactive.getByRole('button', { name: 'Nonaktifkan' })).toHaveCount(0);
		await expect(inactive.getByRole('button', { name: /Aktifkan/i })).toHaveCount(0);

		// The state came from a re-read, not from the response body. The handler
		// returns `is_active` as the STRING "false" (it builds a map[string]string),
		// and `Boolean("false")` is `true` — so trusting the response would show
		// this facility as still active on the one screen whose job is to say
		// otherwise.
		const reloaded = await facilityByShortCode(page, shortCode());
		expect(reloaded!.is_active, 'the server really deactivated it').toBe(false);
		expect(
			typeof reloaded!.is_active,
			'the re-read yields a real boolean, not the string "false"'
		).toBe('boolean');

		// The success channels fire for deactivation too.
		const live = announcer(page);
		await expect(live).toHaveAttribute('aria-live', 'polite');
		await expect(live).toContainText(/nonaktif/i, { timeout: SETTLE.timeout });
		await expect(page.locator('.sigap-toast')).toBeVisible(SETTLE);
	});

	test('removes the deactivated facility from the public catalog', async ({ page }) => {
		test.skip(!CAN_CREATE, 'depends on the facility the deactivate test changed');

		// §10: the impact the dialog promised must actually hold. A catalog that
		// still listed it would make the dialog a claim about the product that
		// the product does not keep.
		const catalog = await page.request.get('/api/v1/public/facilities');
		expect(catalog.ok()).toBe(true);
		const names = ((await catalog.json()).data as Array<{ name: string }>).map(
			(facility) => facility.name
		);
		expect(names, 'a deactivated facility must not be offered to residents').not.toContain(
			RENAMED_NAME
		);
	});

	/* --------------------------- §11 dialog accessibility --------------------------- */

	/*
		The dialog contract is about a dialog, but it can only be demonstrated from a
		real trigger — a row action — so it needs an actor that can see a facility.
		For a zero-scope actor there is no row, hence no trigger, hence nothing to
		prove about focus restoration; the same behaviour is covered by the
		Dialog.test.ts component tests, which need no data at all, and end to end
		for every actor that holds a row.
	*/
	test.describe('dialog accessibility, from a real row trigger', () => {
		test.skip(HOLDS_NO_DATA, 'this actor holds no facility row, so there is no trigger');

			test('traps focus, closes on Escape, and returns focus to the trigger', async ({ page }) => {
				await gotoAdmin(page, '/admin/facilities');
				const table = page.locator('table');
				await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);
	
				// A DATA row this run is allowed to edit. Scoped to `tbody` rather than
				// `tr`, because `table.locator('tr').first()` is the HEADER row and
				// carries no action buttons at all. Under the global actor every seeded
				// facility is in scope, so the first row is a safe, stable target for an
				// EDIT (as opposed to a deactivate, which is one-way).
				const row = table.locator('tbody tr').first();
				const trigger = row.getByRole('button', { name: 'Ubah' });
				await expect(trigger).toBeVisible(SETTLE);
	
				// Focus enters the dialog.
				await trigger.click();
				const modal = dialog(page);
				await expect(modal).toBeVisible(SETTLE);
				const focusInsideDialog = await page.evaluate(() => {
					const open = document.querySelector('dialog[open]');
					return !!open && !!open.contains(document.activeElement);
				});
				expect(focusInsideDialog, 'focus must move into the dialog on open').toBe(true);
	
				// aria-labelledby names the title, so the dialog has an accessible name.
				const labelledBy = await modal.getAttribute('aria-labelledby');
				expect(labelledBy, 'the dialog must be labelled by its title').toBeTruthy();
				const labelText = await page.evaluate((id: string) => {
					const el = document.getElementById(id);
					return el?.textContent?.trim() ?? '';
				}, labelledBy!);
				expect(labelText.length, 'the accessible name must not be empty').toBeGreaterThan(0);
	
				// Focus is TRAPPED: tabbing from the last control wraps inside the modal
				// rather than reaching the page behind it.
				for (let i = 0; i < 25; i += 1) {
					await page.keyboard.press('Tab');
				}
				const stillInside = await page.evaluate(() => {
					const open = document.querySelector('dialog[open]');
					return !!open && !!open.contains(document.activeElement);
				});
				expect(stillInside, 'focus must stay inside the dialog while it is open').toBe(true);
	
				// Escape closes it, and focus goes back to the control that opened it.
				await page.keyboard.press('Escape');
				await expect(modal).toHaveCount(0, SETTLE);
				await expect
					.poll(() => page.evaluate(() => document.activeElement?.textContent?.trim() ?? ''), {
						timeout: SETTLE.timeout
					})
					.toBe('Ubah');
			});
	
			test('Cancel returns focus to the trigger and changes nothing', async ({ page }) => {
				await gotoAdmin(page, '/admin/facilities');
				const table = page.locator('table');
				await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);
	
				const row = table.locator('tbody tr').first();
				const trigger = row.getByRole('button', { name: 'Ubah' });
				await trigger.click();
	
				const modal = dialog(page);
				await expect(modal).toBeVisible(SETTLE);
	
				// Typed and then abandoned: the point of the assertion below is that
				// nothing was sent, so a change has to be on the table.
				const original = await modal.getByLabel('Nama fasilitas').inputValue();
				await modal.getByLabel('Nama fasilitas').fill('Tidak Akan Tersimpan');
	
				// `Batal` must be matched exactly: the deactivation dialog's confirm
				// label is "Ya, nonaktifkan", and a substring match would be ambiguous
				// across the two dialogs.
				await modal.getByRole('button', { name: 'Batal', exact: true }).click();
				await expect(modal).toHaveCount(0, SETTLE);
	
				// Focus returns. Without this the operator is dropped onto <body> and
				// has to re-navigate the whole table with the keyboard.
				await expect
					.poll(() => page.evaluate(() => document.activeElement?.textContent?.trim() ?? ''), {
						timeout: SETTLE.timeout
					})
					.toBe('Ubah');
	
				// Nothing was persisted.
				const stillThere = await page.request.get('/api/v1/admin/facilities');
				const names = ((await stillThere.json()).data as Array<{ name: string }>).map(
					(facility) => facility.name
				);
				expect(names, 'a cancelled edit must not persist').not.toContain('Tidak Akan Tersimpan');
				expect(names).toContain(original);
			});
	
			test('keyboard alone reaches and activates the deactivate confirmation', async ({
				page
			}) => {
				await gotoAdmin(page, '/admin/facilities');
				const table = page.locator('table');
				await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);
	
				// Keyboard activation, not a click: a control that only responds to a
				// pointer is inaccessible, and this is the most destructive action in the
				// admin surface.
				//
				// Targeted by the CONTROL, not by position. An inactive row has no
				// deactivate button by design, and the deactivate test above leaves one
				// behind, so "the first row" is not a stable choice — it depends on
				// where this run's own disposable facility happens to sort.
				const trigger = table.getByRole('button', { name: 'Nonaktifkan' }).first();
				await expect(trigger).toBeVisible(SETTLE);
				await trigger.focus();
				await page.keyboard.press('Enter');
	
				const modal = dialog(page);
				await expect(modal).toBeVisible(SETTLE);
				await expect(modal).toContainText('tidak lagi muncul untuk warga');
	
				// Dismissed, not confirmed — this test is about reaching the dialog and
				// leaving it, and confirming would deactivate the demo facility for the
				// rest of the run.
				await page.keyboard.press('Escape');
				await expect(modal).toHaveCount(0, SETTLE);
			});
	
			test('uses no window.confirm anywhere on the facilities page', async ({ page }) => {
				const native: string[] = [];
				page.on('dialog', (browserDialog) => {
					native.push(browserDialog.type());
					void browserDialog.dismiss();
				});
	
				await gotoAdmin(page, '/admin/facilities');
				await expect(page.locator('table tbody tr').first()).toBeVisible(SETTLE);
	
				const row = page.locator('table tbody tr').first();
				await row.getByRole('button', { name: 'Ubah' }).click();
				await expect(dialog(page)).toBeVisible(SETTLE);
				await dialog(page).getByRole('button', { name: 'Batal', exact: true }).click();
	
				expect(native, 'native dialogs are not accessible and must not be used').toEqual([]);
			});
	});
});
