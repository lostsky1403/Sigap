import type { Locator, Page } from '@playwright/test';
import { expect, test } from './support/test';
import type { MyAppointment } from '../src/lib/api/types/api';
import type { AdminOrganization } from '../src/lib/api/endpoints/admin';

/**
 * The SELF-HOSTED AUTH journey: four real actors, four real sessions.
 *
 * WHAT THIS PROVES, AND WHY IT IS A SEPARATE FILE
 *
 * Every other spec in this directory reaches the Go API through the LOCAL test
 * actor (`SIGAP_LOCAL_E2E_ACTOR` → `X-Sigap-Local-Test-Subject`), which is a
 * process-level selector, not an authentication. This file is the one spec that
 * proves the production-shaped path instead:
 *
 *   login FORM → Better Auth (self-hosted, `auth` schema in the same Postgres)
 *   → `better-auth.session_token` cookie → SvelteKit `hooks.server.ts` mints an
 *   ES256 JWT → the Go API verifies it against the web tier's JWKS → the RBAC
 *   resolver maps `sub` to `app_users.subject` → permissions, organization
 *   scope, and patient ownership are all resolved from the database.
 *
 * It therefore only means anything under the preview that turns the local
 * selectors OFF:
 *
 *   pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1 -Jwt -NoAdminActor
 *
 * and it requires the reviewed fixture state, which `--apply` writes (Better
 * Auth accounts, `app_users.subject` for the four actors, and the citizen
 * fixture appointments):
 *
 *   node apps/web/scripts/prepare-auth-e2e.mjs --apply
 *   SIGAP_E2E_SELFHOSTED_AUTH=1 SIGAP_E2E_BASE_URL=http://127.0.0.1:5173 \
 *     npx playwright test e2e/selfhosted-auth-journey.spec.ts
 *
 * (`SIGAP_E2E_SELFHOSTED_AUTH=1` is the opt-in the describe block below checks;
 * without it these cases skip, so the rest of the suite — which runs against a
 * disposable, seed-only cluster — never depends on a prepared database.)
 *
 * Procedure of record: design/selfhosted-auth-e2e.md.
 *
 * WHY THE ASSERTIONS USE THE API AS WELL AS THE PAGE
 *
 * The four cases are about AUTHORIZATION, and a page can only ever show what it
 * was given. So each case reads the same fact twice: once through the UI (what
 * an operator sees) and once through a direct same-origin fetch from the
 * signed-in browser context (what the server actually answered). A page that
 * happened to render the right rows for the wrong actor would pass the first
 * and fail the second.
 *
 * STATE IS SHARED AND THE CANCELLATION IS ONE-WAY
 *
 * Patient A's cancel is a real `scheduled → cancelled` transition, so the run
 * consumes its own fixture. `test.describe.configure({ mode: 'serial' })` keeps
 * the four cases in one worker, in order, and `prepare-auth-e2e.mjs --apply`
 * restores the fixture before each run. A test that cannot find its prepared
 * pre-state fails with that instruction rather than with a mystery UI timeout.
 */

const DESKTOP = { width: 1440, height: 900 };

/** Real round trips through the SvelteKit proxy to the Go API; default is too tight. */
const SETTLE = { timeout: 15_000 };

/** Any UUID. The directory must show names, never machine identifiers. */
const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

/**
 * The reviewed fixtures: the four logins the spec types into the form.
 *
 * Same table as `scripts/prepare-auth-e2e.mjs` — that script signs these emails
 * in to prove the credentials work and writes each one's Better Auth id into the
 * `app_users` row the reviewer paired it with, so the two files have to agree
 * about which login is which actor.
 */
const FIXTURE = {
	patientA: { email: 'smoke@sigap.local', password: 'Smoke12345!' },
	patientB: { email: 'smoke-b@sigap.local', password: 'SmokeB12345!' },
	orgAdminA: { email: 'org-a-admin@sigap.local', password: 'Admin12345!' },
	superAdmin: { email: 'global-admin@sigap.local', password: 'Admin12345!' }
} as const;

/** The two seeded organizations (`packages/db/seed/regional_tenancy.sql`). */
const ORG_A = { id: '00000000-0000-0000-0000-00000000c001', name: 'RS Sehat Nusantara Group' };
const ORG_B = { id: '00000000-0000-0000-0000-00000000c002', name: 'Puskesmas Prima Sejahtera Network' };

/** Facility substrings the citizen fixture must render (full names are longer). */
const FACILITY_A = 'RS Sehat Nusantara';
const FACILITY_B = 'Puskesmas Prima';

/** The citizen-facing status wording (`lib/domain/status.ts`). */
const SCHEDULED = 'Terjadwal';
const CANCELLED = 'Dibatalkan';

/**
 * Patient A's still-live appointment, recorded by the A case for the B case.
 *
 * Serial mode is what makes this a fact rather than a hope: test 2's denial
 * target is the appointment A did NOT cancel, i.e. one that is genuinely
 * cancellable by its owner, so a 403 can only mean the ownership check refused
 * it. (B holding A's code is the leak this asserts against — it is not a
 * prerequisite the fixture hands out.)
 */
let remainingScheduled: MyAppointment | null = null;

/* -------------------------------------------------------------------------- */
/* helpers                                                                     */
/* -------------------------------------------------------------------------- */

/**
 * Signs in through the real login FORM (server action → Better Auth).
 *
 * The submit is a plain form POST, so there is no client-side "signed in" state
 * to wait for: the 303 back to `/` is the transition, and the session cookie is
 * what the browser then carries.
 */
async function signIn(page: Page, actor: { email: string; password: string }): Promise<void> {
	await page.goto('/auth/login');
	const form = page.getByTestId('auth-form');
	await expect(form, 'the login form must be withheld only when auth is unconfigured').toBeVisible(
		SETTLE
	);

	await form.locator('input[name="email"]').fill(actor.email);
	await form.locator('input[name="password"]').fill(actor.password);
	await form.locator('button[type="submit"]').click();

	// The action redirects (303) on success and re-renders the form with an error
	// banner on failure, so leaving /auth/login is the success signal. The catch
	// reads that banner: a 30 s timeout with no diagnosis is the least useful
	// thing this file could produce for the most likely mistake (a database that
	// was never prepared).
	try {
		await page.waitForURL((url) => !url.pathname.startsWith('/auth/login'), SETTLE);
	} catch {
		const banner = (
			await page.getByTestId('auth-error').textContent({ timeout: 2_000 }).catch(() => null)
		)?.trim();
		throw new Error(
			`${actor.email} could not sign in${banner ? `: "${banner}"` : ' (the form never left /auth/login)'}. ` +
				'The fixtures must be prepared against the -Jwt -NoAdminActor preview: ' +
				'`node scripts/prepare-auth-e2e.mjs --apply` — design/selfhosted-auth-e2e.md §3.'
		);
	}
	await expect(
		page.getByTestId('auth-error'),
		'sign-in must not land back on the form with an error banner'
	).toBeHidden();

	const cookies = await page.context().cookies();
	expect(
		cookies.some((cookie) => cookie.name.includes('session_token')),
		`${actor.email} must hold a Better Auth session cookie after the form POST`
	).toBe(true);
}

/**
 * Signs out through the UI (`/auth/logout` → POST → 303 to `/`).
 *
 * The page's own submit button is the only visible control named `Keluar`: the
 * account menu's copy of that form lives inside a disclosure that is closed by
 * default.
 */
async function signOut(page: Page): Promise<void> {
	await page.goto('/auth/logout');
	await page.getByRole('button', { name: 'Keluar' }).click();
	await page.waitForURL((url) => url.pathname === '/', SETTLE);

	const cookies = await page.context().cookies();
	expect(
		cookies.filter((cookie) => cookie.name.includes('session_token')),
		'sign-out must clear the session cookie'
	).toHaveLength(0);
}

/** The account-scoped appointment list, read straight from the proxy. */
async function fetchMyAppointments(page: Page): Promise<MyAppointment[]> {
	const response = await page.request.get('/api/v1/patient/appointments');
	expect(response.ok(), 'the account-scoped appointment read must succeed').toBe(true);
	const body = (await response.json()) as { data: MyAppointment[] };
	return body.data;
}

/** The scoped organization directory, read straight from the proxy. */
async function fetchScopedOrganizations(page: Page): Promise<AdminOrganization[]> {
	const response = await page.request.get('/api/v1/admin/organizations');
	expect(response.ok(), 'the scoped organization read must succeed').toBe(true);
	const body = (await response.json()) as { data: AdminOrganization[] };
	return body.data;
}

/** "Janji Saya"'s list region and the appointment cards inside it. */
function appointmentList(page: Page): Locator {
	return page.getByRole('region', { name: 'Daftar janji temu Anda' });
}

function appointmentCards(page: Page): Locator {
	return appointmentList(page).getByRole('article');
}

/** One card, by the facility name it renders. */
function cardByFacility(page: Page, facility: string): Locator {
	return appointmentCards(page).filter({ has: page.getByRole('heading', { name: facility }) });
}

/** What each card currently says: the facility an operator reads, and its status. */
async function cardsSnapshot(page: Page): Promise<Array<{ facility: string; status: string }>> {
	const cards = appointmentCards(page);
	const count = await cards.count();
	const snapshot: Array<{ facility: string; status: string }> = [];
	for (let i = 0; i < count; i += 1) {
		const card = cards.nth(i);
		snapshot.push({
			facility: (await card.getByRole('heading').innerText()).trim(),
			status: (await card.locator('.sigap-badge').innerText()).trim()
		});
	}
	return snapshot;
}

/**
 * OPT-IN, and why it has to be.
 *
 * This is the only spec that needs BOTH a preview mode and a prepared database:
 * `-Jwt -NoAdminActor` (the local actor selectors off, so Better Auth's JWT is
 * the only thing that authenticates) and the fixture rows
 * `scripts/prepare-auth-e2e.mjs --apply` writes. The repo's own local-E2E
 * harness (`scripts/dev/Start-LocalE2E.ps1`) boots a DISPOSABLE cluster from the
 * seeds and runs the whole suite as a pinned local actor — the fixture accounts
 * do not exist there and a form sign-in cannot succeed — so an unconditional
 * spec would fail that gate for a reason that has nothing to do with it.
 *
 * The flag is the documented opt-in; the skip message carries the recipe.
 *   design/selfhosted-auth-e2e.md §3
 */
const ENABLED = process.env.SIGAP_E2E_SELFHOSTED_AUTH === '1';

test.describe.configure({ mode: 'serial' });

test.describe('self-hosted auth journey (real Better Auth sessions)', () => {
	test.skip(
		!ENABLED,
		'opt-in: needs the -Jwt -NoAdminActor preview plus `node scripts/prepare-auth-e2e.mjs --apply` ' +
			'and SIGAP_E2E_SELFHOSTED_AUTH=1 — see design/selfhosted-auth-e2e.md §3'
	);

	test.use({ viewport: DESKTOP });

	test('patient A: two cards, and cancelling one through the UI leaves one scheduled', async ({
		page
	}) => {
		await signIn(page, FIXTURE.patientA);

		// The prepared pre-state. This is read BEFORE the page is asserted so a
		// missing `--apply` fails with the instruction rather than as a UI
		// timeout — and it is the same fact the UI is about to be checked against.
		const before = await fetchMyAppointments(page);
		expect(
			before.filter((appointment) => appointment.appointment_status === 'scheduled'),
			'the prepared fixture must hold exactly two scheduled appointments — run `node scripts/prepare-auth-e2e.mjs --apply` first'
		).toHaveLength(2);

		await page.goto('/patient/appointments');
		await expect(page.getByRole('heading', { level: 1 })).toHaveText('Janji saya');

		// EXACTLY two cards, and both seeded facilities by name (substring: the
		// full names carry the branch, "…Cimahi" / "…Tangerang").
		await expect(appointmentCards(page), 'the account owns exactly two cards').toHaveCount(2);
		const snapshotBefore = await cardsSnapshot(page);
		expect(
			snapshotBefore.filter((card) => card.facility.includes(FACILITY_A)),
			`one card must be at ${FACILITY_A}`
		).toHaveLength(1);
		expect(
			snapshotBefore.filter((card) => card.facility.includes(FACILITY_B)),
			`one card must be at ${FACILITY_B}`
		).toHaveLength(1);
		// Every card is the state the fixture was prepared in, so the cancel button
		// below is on an appointment that is genuinely cancellable.
		for (const card of snapshotBefore) {
			expect(card.status, `${card.facility} must start scheduled`).toBe(SCHEDULED);
		}
		// The list the UI rendered is the list the server returned — same facilities,
		// no client-side invention or omission.
		expect([...snapshotBefore].map((card) => card.facility).sort()).toEqual(
			[...before].map((appointment) => appointment.facility_name).sort()
		);

		// The eligible one: `Batalkan` is rendered only for `scheduled`, and both
		// cards are scheduled. The first is the newest by `appointment_time`, which
		// is the order the backend returns.
		const eligible = appointmentCards(page).filter({
			has: page.getByRole('button', { name: 'Batalkan' })
		});
		await expect(eligible, 'at least one appointment must be cancellable').toHaveCount(2);
		const chosenCard = eligible.first();
		const chosenFacility = (await chosenCard.getByRole('heading').innerText()).trim();
		const chosen = before.find((appointment) => appointment.facility_name === chosenFacility);
		expect(chosen, `the card "${chosenFacility}" must be one the backend returned`).toBeTruthy();

		// Cancel it through the dialog. The dialog names the appointment, so the
		// person confirming knows which one they are cancelling.
		await chosenCard.getByRole('button', { name: 'Batalkan' }).click();
		const dialog = page.getByRole('dialog');
		await expect(dialog).toBeVisible(SETTLE);
		await expect(dialog.getByRole('heading')).toHaveText('Batalkan janji temu');
		await expect(dialog).toContainText(chosenFacility);
		await dialog.getByRole('button', { name: 'Ya, batalkan' }).click();
		await expect(dialog).toBeHidden(SETTLE);

		// The card updates in place: same appointment, new status text. Asserting
		// the wording (not only a count) is the difference between "the list
		// re-rendered" and "the server accepted the transition".
		await expect(cardByFacility(page, chosenFacility).locator('.sigap-badge')).toHaveText(
			CANCELLED,
			SETTLE
		);

		// ONE REMAINING. The server is asked again — the page does not get to
		// decide this — and the UI must agree with both the count and the text.
		const after = await fetchMyAppointments(page);
		expect(after, 'a cancelled appointment stays in the citizen’s own list').toHaveLength(2);
		expect(
			after.filter((appointment) => appointment.appointment_status === 'scheduled'),
			'exactly one appointment remains scheduled'
		).toHaveLength(1);
		const stillLive = after.find((appointment) => appointment.appointment_status === 'scheduled');
		expect(stillLive, 'the cancelled appointment is not the one still scheduled').toBeTruthy();
		expect(stillLive?.facility_name).not.toBe(chosenFacility);

		const snapshotAfter = await cardsSnapshot(page);
		expect(snapshotAfter, 'the card count is unchanged').toHaveLength(2);
		expect(
			snapshotAfter.filter((card) => card.status === SCHEDULED),
			'exactly one card still reads Terjadwal'
		).toHaveLength(1);
		expect(snapshotAfter.filter((card) => card.status === CANCELLED)).toHaveLength(1);
		await expect(cardByFacility(page, chosenFacility).locator('.sigap-badge')).toHaveText(
			CANCELLED
		);

		// Hand the live appointment to the isolation case, which tries to cancel it
		// as a different citizen.
		remainingScheduled = stillLive ?? null;

		await signOut(page);

		// The signed-out state: the page fetches again, the server refuses, and the
		// citizen is told to sign in rather than shown an empty list.
		await page.goto('/patient/appointments');
		await expect(page.getByRole('alert')).toContainText('Silakan masuk', SETTLE);
		await expect(appointmentList(page)).toBeHidden();
	});

	test("patient B: an empty list, and A's appointment cannot be cancelled", async ({ page }) => {
		await signIn(page, FIXTURE.patientB);

		// The same endpoint, a different session: identity — not a query parameter
		// — is what scopes this list, so B sees B's own (empty) list.
		const mine = await fetchMyAppointments(page);
		expect(mine, "B must not see any of A's appointments").toHaveLength(0);

		await page.goto('/patient/appointments');
		await expect(page.getByRole('heading', { level: 1 })).toHaveText('Janji saya');
		await expect(page.getByText('Belum ada janji temu')).toBeVisible(SETTLE);
		await expect(appointmentCards(page), 'an empty account renders no cards').toHaveCount(0);

		// The page must not leak the other citizen's facilities — not as a card, not
		// as copy, not as a hidden string in the DOM.
		const body = (await page.locator('body').innerText()) ?? '';
		expect(body, `${FACILITY_A} must not appear for B`).not.toContain(FACILITY_A);
		expect(body, `${FACILITY_B} must not appear for B`).not.toContain(FACILITY_B);

		// B holds A's check-in code (the leak this endpoint has to survive) and tries
		// to cancel A's live appointment with it.
		expect(
			remainingScheduled,
			'the patient-A case must run first: it records the still-scheduled appointment this denial targets'
		).not.toBeNull();
		const target = remainingScheduled as MyAppointment;

		const denied = await page.request.post('/api/v1/appointments/cancel', {
			data: { checkin_code: target.checkin_code }
		});
		expect(denied.status(), 'a non-owner must not be able to cancel').toBe(403);
		const deniedBody = await denied.text();
		expect(deniedBody).toContain('Anda tidak berhak membatalkan janji temu ini.');
		expect(deniedBody, 'the refusal must not name the appointment').not.toContain(
			target.facility_name
		);

		// The refusal must also have CHANGED nothing: the public code-keyed status
		// lookup (which B can reach with the code it holds) still reports scheduled.
		const status = await page.request.get(
			`/api/v1/patient/status?code=${encodeURIComponent(target.checkin_code)}`
		);
		expect(status.ok(), 'the code-keyed status read must still resolve').toBe(true);
		const statusBody = (await status.json()) as { data: { appointment_status: string } };
		expect(
			statusBody.data.appointment_status,
			'the denied cancel must leave the appointment scheduled'
		).toBe('scheduled');

		await signOut(page);
		await page.goto('/patient/appointments');
		await expect(page.getByRole('alert')).toContainText('Silakan masuk', SETTLE);
	});

	test('org A admin: exactly one organization, and a sibling org id is refused', async ({
		page
	}) => {
		await signIn(page, FIXTURE.orgAdminA);

		// The backend's answer first: an `organization_admin` scoped to org A holds
		// exactly one organization.
		const scoped = await fetchScopedOrganizations(page);
		expect(scoped.map((organization) => organization.name)).toEqual([ORG_A.name]);

		await page.goto('/admin/organizations');
		await expect(page.getByRole('heading', { level: 1 })).toHaveText('Organisasi');

		const table = page.locator('table.sigap-data-table');
		await expect(table).toBeVisible(SETTLE);
		await expect(table.locator('tbody tr'), 'the directory is the scoped read').toHaveCount(1);
		await expect(table.getByRole('link', { name: ORG_A.name })).toHaveAttribute(
			'href',
			`/admin/organizations/${ORG_A.id}`
		);

		const text = (await table.textContent()) ?? '';
		expect(text, 'a sibling organization must not appear').not.toContain(ORG_B.name);
		expect(text, 'the directory must not show a raw UUID').not.toMatch(UUID);

		// A direct proxy fetch of the SIBLING organization's id, from the same
		// signed-in browser context. The Go API re-resolves the actor's org scope on
		// every request and answers 404 for an id outside it, so this is refused
		// server-side rather than by the page not linking to it.
		const denied = await page.request.get(`/api/v1/admin/organizations/${ORG_B.id}`);
		expect(denied.status(), 'an out-of-scope organization id must be denied').toBe(404);
		const deniedBody = await denied.text();
		expect(deniedBody).toContain('Organisasi tidak ditemukan.');
		expect(deniedBody, 'the refusal must not leak the sibling organization').not.toContain(
			ORG_B.name
		);

		await signOut(page);

		// Signed-out admin surface: the shell renders, the scoped read is refused,
		// and the page asks for a sign-in instead of an empty directory (which would
		// read as "your scope has no organizations").
		await page.goto('/admin/organizations');
		await expect(page.getByRole('alert')).toContainText('Silakan masuk', SETTLE);
		await expect(page.locator('table.sigap-data-table')).toBeHidden();
	});

	test('super admin: the directory holds both organizations', async ({ page }) => {
		await signIn(page, FIXTURE.superAdmin);

		const scoped = await fetchScopedOrganizations(page);
		expect(scoped.map((organization) => organization.name).sort()).toEqual(
			[ORG_A.name, ORG_B.name].sort()
		);

		await page.goto('/admin/organizations');
		await expect(page.getByRole('heading', { level: 1 })).toHaveText('Organisasi');

		const table = page.locator('table.sigap-data-table');
		await expect(table).toBeVisible(SETTLE);
		await expect(table.locator('tbody tr'), 'the unrestricted scope holds both orgs').toHaveCount(2);
		await expect(table.getByRole('link', { name: ORG_A.name })).toHaveAttribute(
			'href',
			`/admin/organizations/${ORG_A.id}`
		);
		await expect(table.getByRole('link', { name: ORG_B.name })).toHaveAttribute(
			'href',
			`/admin/organizations/${ORG_B.id}`
		);

		await signOut(page);
		await page.goto('/admin/organizations');
		await expect(page.getByRole('alert')).toContainText('Silakan masuk', SETTLE);
		await expect(page.locator('table.sigap-data-table')).toBeHidden();
	});
});
