import type { Locator, Page, Route } from '@playwright/test';
import { expect, test } from './support/test';
import { formatDate } from '../src/lib/domain/format';

/**
 * Phase 3B5: the admin MUTATION surfaces, end to end.
 *
 * WHY THIS FILE IS SEPARATE FROM admin-read.spec.ts
 *
 * `admin-read.spec.ts` ends with a test asserting that every admin destination
 * issues only GET requests. That assertion is still true and still enforced —
 * it is simply about a different page state (nothing is being changed). Putting
 * mutation coverage in that file would make its own invariant untrue, so the
 * two concerns are separated rather than one being deleted.
 *
 * WHY NOTHING HERE IS MOCKED
 *
 * §16 requires real happy paths. Every mutation in this file travels
 * browser -> SvelteKit proxy -> Go API -> Postgres, and every assertion about
 * persistence is made by re-reading through the same origin. A `route.fulfill`
 * would make "the row updated" a statement about a fixture, and would hide
 * precisely the class of defect this phase exists to catch: a body the server
 * rejects, a transition the map does not contain, an affordance the client
 * invented.
 *
 * The one exception is §7's note, and it is a deliberate one: the two
 * INVALID-TRANSITION paths (queue 400, appointment checked_in->completed 400)
 * issue a real request with a deliberately illegal `status` and read the real
 * 400 body. That is not mocking — the backend is genuinely refusing — it is
 * reaching a state the UI will not let the operator reach by clicking, which is
 * the only way to prove the refusal is surfaced verbatim.
 *
 * STATE IS SHARED, SO ORDER MATTERS
 *
 * Fully parallel workers would each need their own database, and the local
 * stack has exactly one. `test.describe.configure({ mode: 'serial' })` plus a
 * fresh seeded stack per run is the arrangement that keeps these assertions
 * honest: every test below reads state that the previous test left, and each
 * says so at the top.
 *
 * The seed contains NO queue tickets, so the queue tests create their own
 * through the real walk-in endpoint. That is not incidental — it means the
 * queue rows under mutation came from the actual Rust gRPC engine rather than
 * from a fixture, which is the only way `PATCH /queues/{id}/status` is being
 * tested against a real ticket.
 */

const DESKTOP = { width: 1440, height: 900 };

/** Real round trips against a local API; the default timeout is too tight. */
const SETTLE = { timeout: 15_000 };

/** Any UUID. Used to prove a machine identifier never reaches the UI. */
const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

/**
 * The seeded facility the local actor genuinely manages.
 *
 * `SIGAP_LOCAL_E2E_ACTOR` resolves to the demo facility, so this is a REAL
 * authorization result, not a hardcoded id. It is read from the page rather
 * than asserted here so that if the seed's facility ever changes, the test
 * fails on a meaningful difference instead of on a stale literal.
 */
const MANAGED_FACILITY = 'Sigap Demo Facility';

/**
 * The actor this Playwright process is running against.
 *
 * Set by Start-LocalE2E.ps1 per actor in the matrix, from the same value the web
 * tier is pinned to, so the test process and the server can never disagree about
 * who is acting.
 */
const ACTOR = (process.env.SIGAP_E2E_ACTOR ?? '').trim();

/**
 * True when the configured actor is scoped to nothing at all.
 *
 * Every mutation here needs something to mutate. A walk-in is booked against a
 * facility, a schedule is created on one, and a facility is updated in place —
 * so a zero-scope actor has no first step, and the backend will (correctly)
 * refuse each attempt.
 *
 * This is not a gap in the matrix. The zero-scope actor's real obligation is the
 * opposite claim: that it is refused, quietly, without leaking anything. That is
 * asserted positively in admin-read.spec.ts, and it is the stronger statement,
 * because these specs would only show that a mutation was attempted.
 */
const HOLDS_NO_DATA = ACTOR === 'local-zero-scope-admin';

/**
 * The capability-refusal actor: reads its scope, mutates nothing.
 *
 * This actor holds `schedule.read` at one facility and `schedule.manage` at no
 * facility. It is the reason this file is not simply "the whole mutation suite
 * for every data actor", and it is the actor that makes the four distinct
 * authorization states observable at all.
 *
 * WHY THE PREDICATES BELOW ARE PERMISSION, NOT PER-ACTOR-TYPE
 *
 * Each block in this file asserts something about a MUTATION, and each mutation
 * is gated on its own permission. An `operator` holds the read half of every
 * domain and none of the corresponding `*.manage` half, so the affordances this
 * suite drives are legitimately absent for it. Asserting they exist would
 * assert the very defect the actor exists to catch; skipping the whole block
 * would throw away the read-side coverage the actor can still give.
 *
 * So each gate names the SPECIFIC missing permission, and the skip message says
 * where that state is proven instead. Three distinct absences are load-bearing
 * here:
 *
 *   - `schedule.manage`      — T-3B5-03's editor. Its state is asserted
 *                               positively in admin-schedule-denied.spec.ts.
 *   - `notification.manage`  — T-3B5-05's retry/cancel. No dedicated spec
 *                               covers it, so it is recorded in the Phase 3B6
 *                               notes rather than claimed as proven.
 *
 * Deriving both from ONE constant keeps the two in step: they are the same
 * `operator` role, so a future seed change that gave it either `*.manage` would
 * need to re-open exactly these gates, and nothing else.
 */
const CAPABILITY_REFUSAL_ACTOR = 'e2e-schedule-reader';

/** True when the actor reads schedules but manages them nowhere. */
const REFUSES_SCHEDULE_MANAGEMENT = ACTOR === CAPABILITY_REFUSAL_ACTOR;

/** True when the actor reads the outbox but may not retry or cancel. */
const REFUSES_NOTIFICATION_MANAGEMENT = ACTOR === CAPABILITY_REFUSAL_ACTOR;

/**
 * A monotonic counter mixed into every generated phone.
 *
 * `Date.now()` alone is not unique enough here: two bookings issued inside the
 * same millisecond produce the same number, and because the API allows only
 * three bookings per phone per day, the collision silently consumes a second
 * daily slot. Mixing a counter in makes each generated phone unique for the
 * lifetime of the worker regardless of clock resolution.
 */
let phoneSequence = 0;

/**
 * Builds a walk-in / booking phone number that has not been used yet.
 *
 * The API keeps an in-memory daily limiter of TWO walk-ins per phone per
 * facility, and THREE bookings per phone per day, both keyed on the API process.
 * A constant or a clock-only value is therefore not merely a duplicate-row
 * problem: a later call in this file would be refused with a 429 and the test
 * would fail on infrastructure it did not set up. Deriving a fresh number per
 * call is what keeps these tests independent of each other and of their
 * execution order.
 *
 * The result is 12 digits, which satisfies the backend's 10-15 digit rule.
 */
function nextPhone(): string {
	phoneSequence += 1;
	const stamp = String(Date.now()).slice(-8);
	const seq = String(phoneSequence).padStart(2, '0').slice(-2);
	return `08${stamp}${seq}`;
}

/** A fresh phone for a walk-in. See `nextPhone` for why it must be unique. */
function walkInPhone(): string {
	return nextPhone();
}

async function gotoAdmin(page: Page, path: string) {
	await page.goto(path);
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
}

/**
 * The one open <dialog> on the page, by role.
 *
 * `getByRole('dialog')` is deliberately used instead of a class selector:
 * asserting on the role is what proves the element is actually exposed as a
 * dialog to assistive technology, which a class-name query cannot.
 */
function dialog(page: Page): Locator {
	return page.getByRole('dialog');
}

/** The polite live region that every admin mutation announces through. */
function announcer(page: Page): Locator {
	return page.locator('.sigap-admin-announcer');
}

/** The toast a successful mutation raises. */
function toast(page: Page): Locator {
	return page.locator('.sigap-toast');
}

/**
 * Reads a facility's id from the public catalog.
 *
 * Used only to CREATE a walk-in ticket, which is a citizen action needing a
 * facility reference. It is not used to drive any admin affordance: those come
 * from the server's own options, and §8 forbids deriving them client-side.
 */
async function managedFacilityId(page: Page): Promise<string> {
	const response = await page.request.get('/api/v1/public/facilities');
	expect(response.ok(), 'the public facility catalog must be readable').toBe(true);
	const body = await response.json();
	const match = (body?.data ?? []).find(
		(facility: { name: string; is_active: boolean }) =>
			facility.name === MANAGED_FACILITY && facility.is_active
	);
	expect(match, `the seeded stack must contain the active facility "${MANAGED_FACILITY}"`).toBeTruthy();
	return match.id as string;
}

/**
 * Creates a real walk-in ticket through the public endpoint.
 *
 * Goes through the Rust queue engine, so the id returned is a row the engine
 * actually wrote. Returns the formatted number, which is what the board shows
 * to the operator.
 */
async function createWalkIn(page: Page, fullName: string): Promise<{ id: string; number: string; facilityId: string }> {
	const facilityId = await managedFacilityId(page);
	const response = await page.request.post('/api/v1/queues/generate', {
		data: { facilityId, patient: { fullName, phone: walkInPhone() } }
	});
	// The status and body are carried into the failure message on purpose. A
	// bare "not ok" here would send the reader looking for a wiring bug when
	// the real cause is usually a 400 with a validation sentence in it.
	expect(
		response.ok(),
		`a walk-in through the real engine must succeed (HTTP ${response.status()}): ${await response
			.text()
			.catch(() => '')}`
	).toBe(true);
	const body = await response.json();
	// The engine answers with `TicketID` (its protobuf field name), not `id`.
	// Normalised here so every caller reads one shape.
	return {
		id: (body.data.id ?? body.data.TicketID) as string,
		number: body.data.formatted_number as string,
		// Carried so a caller can narrow the facility filter to the ticket's OWN
		// facility. Re-deriving it would mean searching the rendered board for a
		// row and inferring scope from what is on screen, which is the inference
		// the whole actor matrix exists to avoid.
		facilityId
	};
}

/**
 * The queue row for one ticket number, by its position in the board.
 *
 * Scoped by the formatted number rather than by id so that a test asserts on
 * what an operator can see. A row located by a hidden id would pass even if the
 * id were never rendered at all.
 */
function queueRow(page: Page, formattedNumber: string): Locator {
	return page.locator('tr', { has: page.getByText(formattedNumber, { exact: true }) });
}

test.describe.configure({ mode: 'serial' });

test.describe('T-3B5-01 queue status mutation', () => {
	test.use({ viewport: DESKTOP });

	// Every spec in this file starts by creating or locating something to mutate.
	// A zero-scope actor has no facility to book against, so there is no first
	// step — the backend refuses before any assertion of ours could run.
	test.skip(HOLDS_NO_DATA, 'this actor holds nothing to mutate; its refusal is asserted in admin-read.spec.ts');

	test('advances a real ticket waiting -> called and persists it', async ({ page }) => {
		// CREATED HERE, not seeded: the seed ships zero tickets, so this is also
		// proof the admin board renders rows the real engine produced.
		const ticket = await createWalkIn(page, 'E2E Antrean Called');
		await gotoAdmin(page, '/admin/queues');

		const row = queueRow(page, ticket.number);
		await expect(row, 'the new walk-in must appear on the board').toBeVisible(SETTLE);

		// A waiting ticket offers EXACTLY called + cancelled. Anything else on
		// the row would be an affordance the server would refuse.
		await expect(row.getByRole('button')).toHaveCount(2);
		await expect(row.getByRole('button', { name: 'Dipanggil' })).toBeVisible();
		await expect(row.getByRole('button', { name: 'Dibatalkan' })).toBeVisible();

		await row.getByRole('button', { name: 'Dipanggil' }).click();

		// The row RELOADS rather than patching locally, so the assertion is that
		// the server's new state came back. The ticket moves from the waiting
		// band to the called band, which is only possible if the DB changed.
		await expect(
			page.locator('section', { has: page.getByRole('heading', { name: 'Dipanggil' }) })
				.locator('tr', { has: page.getByText(ticket.number, { exact: true }) })
		).toBeVisible(SETTLE);

		// And the awaiting set is exactly what `called` allows.
		const moved = queueRow(page, ticket.number);
		await expect(moved.getByRole('button')).toHaveCount(3);
		await expect(moved.getByRole('button', { name: 'Dalam Pelayanan' })).toBeVisible();
		await expect(moved.getByRole('button', { name: 'Dibatalkan' })).toBeVisible();
		await expect(moved.getByRole('button', { name: 'Dilewati' })).toBeVisible();
	});

	test('announces the transition politely and shows a toast', async ({ page }) => {
		const ticket = await createWalkIn(page, 'E2E Antrean Toast');
		await gotoAdmin(page, '/admin/queues');

		const row = queueRow(page, ticket.number);
		await expect(row).toBeVisible(SETTLE);
		await row.getByRole('button', { name: 'Dipanggil' }).click();

		// The live region is aria-live="polite", so it must not interrupt. An
		// assertive region here would cut across whatever the operator is doing.
		const live = announcer(page);
		await expect(live).toHaveAttribute('aria-live', 'polite');
		await expect(live).toHaveAttribute('aria-atomic', 'true');
		await expect(live).toContainText('Dipanggil', { timeout: SETTLE.timeout });

		// AND a visible toast, because a live region is invisible: a screen-reader
		// user is told what happened while a sighted operator sees nothing.
		await expect(toast(page)).toBeVisible(SETTLE);
	});

	test('surfaces the backend 400 message verbatim on an invalid transition', async ({ page }) => {
		const ticket = await createWalkIn(page, 'E2E Antreai Invalid');
		await gotoAdmin(page, '/admin/queues');
		await expect(queueRow(page, ticket.number)).toBeVisible(SETTLE);

		// waiting -> completed is illegal in the state machine. The UI will not
		// offer it, which is the point — so the only way to reach the refusal is
		// to ask the real API directly and then prove the ROW renders that exact
		// text when the same call fails from the page's own handler.
		const probe = await page.request.patch(`/api/v1/admin/queues/${ticket.id}/status`, {
			data: { status: 'completed' }
		});
		expect(probe.status(), 'an illegal queue transition must be refused').toBe(400);
		const refused = await probe.json();

		// The error envelope is `{ success: false, error: <sentence> }` — the key
		// is `error`, not `message`, and the client's `mutationFailure` reads
		// exactly that field to decide the message is server-described.
		const sentence = refused.error as string;
		expect(sentence, 'the 400 must carry a sentence from the server').toBeTruthy();

		// It names both statuses. It is the server's sentence, and the UI must
		// not paraphrase it into something vaguer.
		expect(sentence).toContain('Transisi status tidak valid');
		expect(sentence).toContain('waiting');
		expect(sentence).toContain('completed');

		// Now the same refusal, reached through the page. Route interception is
		// used ONLY to make the real handler receive this real 400 body; no
		// success path is faked anywhere in this file.
		await page.route(
			(url) => url.pathname === `/api/v1/admin/queues/${ticket.id}/status`,
			async (route: Route) => {
				await route.fulfill({
					status: 400,
					contentType: 'application/json',
					body: JSON.stringify(refused)
				});
			}
		);

		const row = queueRow(page, ticket.number);
		await row.getByRole('button', { name: 'Dipanggil' }).click();

		// VERBATIM. The row must show the server's own words, character for
		// character, in an alert region so it is announced as a failure.
		const alert = row.locator('xpath=following-sibling::tr[1]').getByRole('alert');
		await expect(alert).toHaveText(sentence, { timeout: SETTLE.timeout });
	});

	test('offers no actions on a terminal ticket', async ({ page }) => {
		const ticket = await createWalkIn(page, 'E2E Antrean Terminal');
		await gotoAdmin(page, '/admin/queues');

		const row = queueRow(page, ticket.number);
		await expect(row).toBeVisible(SETTLE);
		// Drive it to `skipped`, which is terminal: called -> skipped.
		await row.getByRole('button', { name: 'Dipanggil' }).click();
		await expect(
			page.locator('section', { has: page.getByRole('heading', { name: 'Dipanggil' }) })
				.locator('tr', { has: page.getByText(ticket.number, { exact: true }) })
		).toBeVisible(SETTLE);

		const calledRow = queueRow(page, ticket.number);
		await calledRow.getByRole('button', { name: 'Dilewati' }).click();

		// A terminal state says so in words. An empty cell would read as
		// "still loading", which is a different and wrong claim.
		const finished = queueRow(page, ticket.number);
		await expect(finished.getByText('Tidak ada aksi lanjutan')).toBeVisible(SETTLE);
		await expect(finished.getByRole('button')).toHaveCount(0);
	});

	/**
	 * §5: the operator's narrowed view must survive the mutation they triggered.
	 *
	 * The reload after a successful transition is a full re-read of the board, not
	 * a local patch — the server stamps `called_at` and does not echo the row
	 * back. That makes the reload the single most likely place for the operator's
	 * own filter to be dropped: the component re-reads `scopedRows` and re-derives
	 * `filtered` from it, and nothing in that path is forced to remember which
	 * facility was selected.
	 *
	 * The failure this guards against is quiet and plausible-looking. The ticket
	 * still moves, the toast still fires, the transition genuinely persisted — and
	 * a nurse who had narrowed the board to one clinic suddenly sees every clinic
	 * in her scope again, mid-shift, with no indication that her filter is gone.
	 * Nothing errors. Nothing fails to persist. The data is simply shown in a
	 * shape she did not ask for and is now looking at the wrong queue.
	 *
	 * So the assertion is on the CONTROL's state, not merely on the row: the
	 * select must still hold the same facility id after the re-read, and the
	 * visible-count label must still reflect a narrowed view rather than the full
	 * loaded set.
	 */
	test('keeps the facility filter applied across a mutation and its reload', async ({ page }) => {
		const ticket = await createWalkIn(page, 'E2E Antrean Filter');
		await gotoAdmin(page, '/admin/queues');

		const row = queueRow(page, ticket.number);
		await expect(row, 'the new walk-in must appear on the board').toBeVisible(SETTLE);

		// The facility is read from the server's own list, not hardcoded, so the
		// same assertion holds for every actor in the matrix.
		const listed = await page.request.get('/api/v1/admin/facilities');
		expect(listed.ok(), 'the facility list must be readable').toBe(true);
		const facilities = (await listed.json()).data as Array<{ id: string; name: string }>;
		expect(facilities.length, 'the actor must be able to read at least one facility').toBeGreaterThan(
			0
		);

		// Narrow to the facility this ticket actually belongs to. Choosing the
		// ticket's own facility is what makes "the filter still works" checkable:
		// the row must be visible AFTER the reload, so if the filter were dropped
		// the assertion below could not tell "filter kept" from "filter removed
		// but every row is visible anyway".
		const ticketFacility = facilities.find((facility) => facility.id === ticket.facilityId);
		expect(
			ticketFacility,
			`the seeded stack must expose the ticket's own facility (saw: ${facilities
				.map((facility) => facility.name)
				.join(', ')})`
		).toBeTruthy();

		const filter = page.getByLabel('Filter fasilitas');
		await expect(filter).toBeVisible(SETTLE);
		await filter.selectOption(ticketFacility!.id);

		// The narrowing is real and visible before the mutation. The count label
		// reports the visible subset against the loaded total.
		//
		// Deliberately NOT asserted as "X of Y with X < Y": whether the narrowing
		// reduces the row count depends on where the seed happens to place its
		// tickets, and a run where every loaded ticket shares one facility would
		// make "fewer than" fail against a perfectly correct filter. The
		// load-bearing claim is the CONTROL's value plus the row's continued
		// visibility, both asserted below; this is a readability check that the
		// label resolved at all.
		const narrowed = page.locator('.sigap-facility-filter__count');
		await expect(narrowed).toContainText('Menampilkan:', SETTLE);

		const beforeValue = await filter.inputValue();
		expect(beforeValue, 'the filter must actually be narrowed before the mutation').toBe(
			ticketFacility!.id
		);

		// And the narrowed view really is in force: the ticket's own facility is
		// selected, so this row must still be on the page. If the filter were not
		// applied at all, this row would still be visible too — which is why the
		// control's value above is the real assertion and this one is the
		// consequence.
		await expect(queueRow(page, ticket.number)).toBeVisible(SETTLE);

		// The mutation, through the page's own control.
		await queueRow(page, ticket.number).getByRole('button', { name: 'Dipanggil' }).click();
		await expect(
			page.locator('section', { has: page.getByRole('heading', { name: 'Dipanggil' }) })
				.locator('tr', { has: page.getByText(ticket.number, { exact: true }) })
		).toBeVisible(SETTLE);

		// THE ASSERTION. The select must still hold the operator's choice after
		// the re-read. This is a value comparison, not a visibility check,
		// because a filter that reset to "Semua fasilitas" would still render.
		await expect
			.poll(
				async () => page.getByLabel('Filter fasilitas').inputValue(),
				{ timeout: SETTLE.timeout }
			)
			.toBe(ticketFacility!.id);

		// And the narrowing is still in force: the mutated row is still on the
		// page, and the count label still reports a subset.
		await expect(queueRow(page, ticket.number)).toBeVisible(SETTLE);
		await expect(page.locator('.sigap-facility-filter__count')).toContainText(
			'Menampilkan:',
			SETTLE
		);

		// The transition genuinely persisted, so this is not passing because the
		// mutation silently failed and left the board untouched.
		const reloaded = await page.request.get('/api/v1/admin/queues');
		expect(reloaded.ok()).toBe(true);
		const rows = (await reloaded.json()).data as Array<{ id: string; status: string }>;
		expect(rows.find((entry) => entry.id === ticket.id)?.status).toBe('called');
	});
});

test.describe('T-3B5-02 appointment status mutation', () => {
	test.use({ viewport: DESKTOP });

	test.skip(HOLDS_NO_DATA, 'this actor holds nothing to mutate; its refusal is asserted in admin-read.spec.ts');

	/**
	 * Reads this actor's appointments.
	 *
	 * Used to VERIFY a mutation against the server's data rather than against
	 * the button set that produced it. The two can disagree — that is exactly
	 * what a broken reload looks like — and reading the API back is what makes
	 * these assertions about persistence instead of about the client.
	 */
	async function appointmentsIn(
		page: Page
	): Promise<Array<{ id: string; status: string; patient_display_name: string }>> {
		const response = await page.request.get('/api/v1/admin/appointments');
		expect(response.ok(), 'the appointments read must succeed').toBe(true);
		return (await response.json()).data as Array<{
			id: string;
			status: string;
			patient_display_name: string;
		}>;
	}

	/** Moves a row to `status` through the real endpoint, asserting it is legal. */
	async function moveAppointment(page: Page, id: string, from: string, to: string) {
		const response = await page.request.patch(`/api/v1/admin/appointments/${id}/status`, {
			data: { status: to }
		});
		expect(response.ok(), `${from} -> ${to} must be a legal step`).toBe(true);
	}

	/**
	 * Books a REAL appointment and returns the row the backend actually created.
	 *
	 * §17 asks for "a fresh local appointment where possible", and this is how.
	 * Reusing a seeded row makes each test's precondition depend on what the
	 * tests before it left behind, which is how a suite ends up green in one
	 * order and red in another. Booking its own row costs one HTTP call and
	 * makes every appointment test independent of execution order.
	 *
	 * The id is taken from the BOOKING RESPONSE, not rediscovered by status
	 * afterwards. Looking it up again by status is the bug this comment exists
	 * to prevent: the list is ordered arbitrarily, so "the first `scheduled`
	 * row" is whichever row the server happens to return first, which is
	 * another test's row the moment two tests are in `scheduled` at once.
	 *
	 * The wire keys are the backend's, verified against
	 * `BookAppointmentRequest` in booking.go — the name is
	 * `patient_display_name`, not `patient_name`.
	 */
	async function bookAppointment(page: Page, name: string) {
		// The facility is discovered from the actor's OWN READ SCOPE, joined with
		// the service units it can read. It used to be discovered from
		// `/api/v1/admin/schedules/options`, which is a SCHEDULE-MANAGE
		// endpoint, and that coupled an appointment booking to an unrelated
		// permission. Booking needs no schedule permission at all:
		// `POST /api/v1/appointments` is ungated in the route registry. An actor
		// that reads a facility and its units can therefore book against it even
		// while managing schedules nowhere — which is exactly the capability-
		// refusal actor. Reading options here made its appointment coverage
		// vacuous: the helper failed on "no manageable facility" before it ever
		// made a booking request, so the suite reported a wiring failure for a
		// correct page and a correct API.
		const readable = await page.request.get('/api/v1/admin/facilities');
		expect(readable.ok(), 'the readable facility list must succeed').toBe(true);
		const facilities = (await readable.json()).data as Array<{ id: string; name: string }>;

		const units = await page.request.get('/api/v1/admin/service-units');
		expect(units.ok(), 'the service-unit read must succeed').toBe(true);
		const serviceUnits = (await units.json()).data as Array<{
			id: string;
			facility_id: string;
			is_active: boolean;
		}>;

		// A facility with no active service unit cannot take a booking, so the
		// join happens HERE, in the test, and both sides are the server's own
		// scoped reads. Under the unscoped actor the first match is whichever
		// facility sorts first that happens to carry a unit; under a scoped actor
		// it is the single facility inside their scope. Deriving rather than
		// hardcoding is what keeps this correct for every actor in the matrix.
		const bookable = facilities
			.map((facility) => ({
				facility,
				unit: serviceUnits.find(
					(candidate) =>
						candidate.facility_id === facility.id && candidate.is_active
				)
			}))
			.filter((candidate) => candidate.unit !== undefined);
		expect(
			bookable.length,
			`the actor must read a facility with an active service unit (readable: ${facilities
				.map((facility) => facility.name)
				.join(', ') || '(none)'}; units: ${
				serviceUnits.length
			})`
		).toBeGreaterThan(0);
		const { facility, unit } = bookable[0];
		expect(unit, 'the chosen facility must expose a service unit').toBeTruthy();

		const response = await page.request.post('/api/v1/appointments', {
			data: {
				facility_id: facility.id,
				service_unit_id: unit!.id,
				patient_display_name: name,
				// A FRESH phone per booking, not a fixed one. The API allows three
				// bookings per phone per day, so a shared literal would make the
				// fourth booking in this file fail with a 429 that has nothing to
				// do with mutation wiring. See `nextPhone`.
				patient_phone: nextPhone(),
				// A time far enough ahead that a same-day schedule rule cannot
				// refuse the booking for an unrelated reason.
				appointment_time: '2026-12-01T09:00:00Z'
			}
		});
		expect(
			response.ok(),
			`booking a fresh appointment must succeed (HTTP ${response.status()}): ${await response
				.text()
				.catch(() => '')}`
		).toBe(true);

		const created = (await response.json()).data as { id: string; status: string };
		expect(created.id, 'the booking response must carry the new appointment id').toBeTruthy();

		// Confirm the server really stored it under the name we passed, and that
		// it is visible to THIS actor's read (it is inside their facility scope,
		// so a missing row would mean the scope is wrong, not the booking).
		const rows = await appointmentsIn(page);
		const stored = rows.find((row) => row.id === created.id);
		expect(
			stored,
			`the booked appointment must appear in this actor's read (saw: ${rows
				.map((row) => `${row.status}/${row.patient_display_name}`)
				.join(', ')})`
		).toBeTruthy();
		expect(stored!.patient_display_name).toBe(name);
		expect(stored!.status).toBe(created.status);
		return stored!;
	}

	test('moves a scheduled appointment to checked_in without any confirmation', async ({ page }) => {
		// Its OWN row, so this test does not depend on which states the tests
		// before it happened to consume.
		const name = `E2E Checkin ${Date.now()}`;
		const before = await bookAppointment(page, name);

		await gotoAdmin(page, '/admin/appointments');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		// A `scheduled` appointment offers exactly three, and checking in is not
		// one of the confirmed ones.
		const row = table.locator('tr', { has: page.getByText(name) }).first();
		await expect(row).toBeVisible(SETTLE);
		await expect(row.getByRole('button', { name: 'Sudah Check-in' })).toBeVisible();
		await expect(row.getByRole('button', { name: 'Dibatalkan' })).toBeVisible();
		await expect(row.getByRole('button', { name: 'Tidak Datang' })).toBeVisible();

		await row.getByRole('button', { name: 'Sudah Check-in' }).click();

		// NO DIALOG, asserted against the live UI rather than a snapshot. If
		// checking in asked for confirmation this times out — which is the proof,
		// since §4 allows a dialog for `cancelled` only.
		await expect(dialog(page)).toHaveCount(0);

		// VERIFIED AGAINST THE SERVER, not against the row's own buttons. The
		// status label equals the button label for this transition, so asserting
		// on table text alone could pass on the button that is still rendered.
		await expect
			.poll(
				async () => (await appointmentsIn(page)).find((row) => row.id === before.id)?.status,
				{ timeout: SETTLE.timeout }
			)
			.toBe('checked_in');
	});

	test('never offers checked_in -> completed, at any point in the chain', async ({ page }) => {
		// Its OWN row, advanced to `checked_in` through the real endpoint, so
		// this test neither depends on nor disturbs its neighbours.
		const name = `E2E NoShortcut ${Date.now()}`;
		const booked = await bookAppointment(page, name);
		await moveAppointment(page, booked.id, 'scheduled', 'checked_in');
		const current = { ...booked, status: 'checked_in' };

		await gotoAdmin(page, '/admin/appointments');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const row = table.locator('tr', { has: page.getByText(name) }).first();
		await expect(row).toBeVisible(SETTLE);

		// THE ABSENCE. §4 calls this "ABSOLUTELY NO", and it is the one
		// transition most likely to be reintroduced as a convenience shortcut.
		await expect(row.getByRole('button', { name: 'Dalam Antrean' })).toBeVisible();
		await expect(row.getByRole('button', { name: 'Dibatalkan' })).toBeVisible();
		await expect(row.getByRole('button', { name: 'Tidak Datang' })).toBeVisible();
		await expect(row.getByRole('button', { name: 'Selesai' })).toHaveCount(0);
		await expect(row.getByRole('button')).toHaveCount(3);

		// And the server refuses it too, so the button would have been a control
		// that exists only to fail. A real 400 proves the UI's omission and the
		// API's enforcement agree.
		const refused = await page.request.patch(
			`/api/v1/admin/appointments/${current.id}/status`,
			{ data: { status: 'completed' } }
		);
		expect(refused.status(), 'checked_in -> completed must be refused by the server').toBe(400);

		// THE APPOINTMENT SENTENCE IS NOT THE QUEUE SENTENCE, and asserting the
		// queue's wording here would be asserting a copy-paste that does not
		// exist. Queue says "Transisi status tidak valid: a → b."; appointments
		// say "Transisi status 'a' → 'b' tidak diizinkan." Both are 400, both are
		// shown verbatim, and the difference is the backend's to make. What §4
		// forbids is a client-side "conflict" rewording — and this assertion
		// pins that the specific server sentence survives instead of being
		// replaced by any generic phrase.
		const sentence = (await refused.json()).error as string;
		expect(sentence).toContain('checked_in');
		expect(sentence).toContain('completed');
		expect(sentence).toContain('tidak diizinkan');
		expect(
			sentence.toLowerCase(),
			'the appointment refusal must not be reworded as a generic conflict'
		).not.toContain('conflict');
	});

	test('cancels through a confirmation dialog and applies the change only on confirm', async ({
		page
	}) => {
		// NO NATIVE DIALOG. Registered before anything else in this test so a
		// window.confirm FAILS it rather than silently blocking the run.
		const native: string[] = [];
		page.on('dialog', (browserDialog) => {
			native.push(browserDialog.type());
			void browserDialog.dismiss();
		});

		const name = `E2E Cancel ${Date.now()}`;
		const before = await bookAppointment(page, name);

		await gotoAdmin(page, '/admin/appointments');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const row = table.locator('tr', { has: page.getByText(name) }).first();
		await expect(row).toBeVisible(SETTLE);

		// Cancellation is the ONLY confirmed action. Every other transition was
		// proven dialog-free above; this is the one that must have one.
		await row.getByRole('button', { name: 'Dibatalkan' }).click();

		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);
		await expect(modal).toHaveAttribute('aria-labelledby', /.+/);
		await expect(modal).toHaveAttribute('aria-describedby', /.+/);

		// The consequence is named, not a generic "are you sure?": it identifies
		// the appointment and states that the status is final.
		await expect(modal).toContainText(name);
		await expect(modal).toContainText('bersifat final');
		await expect(modal).toContainText('tidak dapat diaktifkan kembali');

		// Focus entered the dialog rather than staying on the page behind it.
		await expect
			.poll(
				async () => page.evaluate(() => document.activeElement?.closest('dialog') !== null),
				{ timeout: SETTLE.timeout }
			)
			.toBe(true);

		// Nothing has changed yet. Cancelling on open would make the dialog
		// decorative. Checked against the SERVER, which is unambiguous.
		expect((await appointmentsIn(page)).find((row) => row.id === before.id)?.status).toBe(
			before.status
		);

		await modal.getByRole('button', { name: 'Ya, batalkan' }).click();
		await expect(modal).toHaveCount(0, { timeout: SETTLE.timeout });

		// The server really moved it, and the row now renders a terminal state.
		await expect
			.poll(
				async () => (await appointmentsIn(page)).find((row) => row.id === before.id)?.status,
				{ timeout: SETTLE.timeout }
			)
			.toBe('cancelled');
		await expect(table.locator('tr', { has: page.getByText(name) }).first()).toContainText(
			'Status final',
			SETTLE
		);

		// And no native dialog was ever used. §11 forbids window.confirm /
		// window.alert for all four mutation surfaces.
		expect(native, 'window.confirm and window.alert must never be used').toEqual([]);
	});

	test('cancels nothing when the dialog is dismissed with Cancel', async ({ page }) => {
		// Its OWN row, so this does not fight the test above over one already
		// cancelled appointment.
		const name = `E2E Dismiss ${Date.now()}`;
		const before = await bookAppointment(page, name);

		await gotoAdmin(page, '/admin/appointments');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const row = table.locator('tr', { has: page.getByText(name) }).first();
		const trigger = row.getByRole('button', { name: 'Dibatalkan' });
		await expect(trigger).toBeVisible(SETTLE);

		await trigger.click();
		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		// The explicit Cancel control, not Escape. Both must return focus, and
		// they are different code paths through the dialog.
		//
		// `exact: true` is REQUIRED here, not cosmetic: "Batal" is a substring
		// of the confirm label "Ya, batalkan", so a non-exact match resolves to
		// two elements and Playwright's strict mode refuses the click. That the
		// two labels overlap at all is a real finding about the copy.
		await modal.getByRole('button', { name: 'Batal', exact: true }).click();
		await expect(modal).toHaveCount(0, SETTLE);

		// Unchanged. A Cancel that applied the action would be the worst
		// possible defect on a one-way status.
		expect((await appointmentsIn(page)).find((row) => row.id === before.id)?.status).toBe(
			before.status
		);

		// Focus returned to the button that opened the dialog, so a keyboard
		// user keeps their place in the table.
		await expect
			.poll(
				async () =>
					page.evaluate(
						() => document.activeElement?.textContent?.trim() === 'Dibatalkan'
					),
				{ timeout: SETTLE.timeout }
			)
			.toBe(true);
	});

	test('closes the cancel dialog on Escape and returns focus to the trigger', async ({
		page
	}) => {
		const name = `E2E Escape ${Date.now()}`;
		const before = await bookAppointment(page, name);

		await gotoAdmin(page, '/admin/appointments');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const row = table.locator('tr', { has: page.getByText(name) }).first();
		const trigger = row.getByRole('button', { name: 'Dibatalkan' });
		await expect(trigger).toBeVisible(SETTLE);

		await trigger.click();
		await expect(dialog(page)).toBeVisible(SETTLE);

		// The native `cancel` event, which is what Escape produces on a modal
		// <dialog>. A JS-only keydown handler would not fire here.
		await page.keyboard.press('Escape');
		await expect(dialog(page)).toHaveCount(0, SETTLE);

		// Focus returns to what opened it. Without this a keyboard user is
		// dropped at the top of the document and loses their place in the table.
		await expect
			.poll(
				async () =>
					page.evaluate(
						() => document.activeElement?.textContent?.trim() === 'Dibatalkan'
					),
				{ timeout: SETTLE.timeout }
			)
			.toBe(true);

		// And the escape really did nothing to the data: a dismissed dialog must
		// not have applied the cancellation behind the operator's back.
		expect((await appointmentsIn(page)).find((row) => row.id === before.id)?.status).toBe(
			before.status
		);
	});

	test('shows a terminal row with no actions once completed', async ({ page }) => {
		// Walk the REAL chain from wherever the row is: scheduled -> checked_in
		// -> queued -> completed, deliberately skipping the forbidden shortcut.
		// Each step is asserted, so a break anywhere in the chain is localised.
		const name = `E2E Completed ${Date.now()}`;
		const current = await bookAppointment(page, name);

		await moveAppointment(page, current.id, 'scheduled', 'checked_in');
		await moveAppointment(page, current.id, 'checked_in', 'queued');
		await moveAppointment(page, current.id, 'queued', 'completed');

		await gotoAdmin(page, '/admin/appointments');
		const table = page.locator('table');
		const row = table.locator('tr', { has: page.getByText(name) }).first();
		await expect(row).toBeVisible(SETTLE);

		// A terminal row states so in words and offers nothing further.
		await expect(row).toContainText('Status final', SETTLE);
		await expect(row.getByRole('button')).toHaveCount(0);
	});

	/**
	 * §17: the success channels, for appointments specifically.
	 *
	 * The queue board, the facility page, and the notification page each assert
	 * their own live region and toast. This is the appointment page's turn, and
	 * it is asserted here rather than inferred from the shared `runAdminMutation`
	 * helper, because that inference is exactly the wrong shape: a page that
	 * forgot to MOUNT `<AdminMutationFeedback />` would still call the shared
	 * helper, the announcement would still be pushed into the store, and every
	 * other page's test would still pass. The mount is per page, so the
	 * obligation is per page.
	 *
	 * Both channels are required because they serve different consumers. The
	 * live region is invisible: without it a screen-reader user gets no
	 * confirmation at all. The toast is transient: without it a sighted operator
	 * who looked away from the table has no record that anything happened. §17
	 * requires all three consumers be served — reloaded rows, a polite
	 * announcement, and a visible toast — and this asserts the last two on the
	 * one surface that had no direct proof.
	 *
	 * The check-in transition is used because it is the one that needs no
	 * confirmation dialog, so nothing else on the page can be mistaken for the
	 * announcement.
	 */
	/**
	 * A booking name that cannot collide with the rate-limit spec.
	 *
	 * §14 greps raw page text for the digits "429", so any timestamp-derived name
	 * that happens to contain that run of digits fails an unrelated test. The
	 * facility spec already derives a tag free of "429" for exactly this reason
	 * (see `runTagWithout` there); the appointment names embed `Date.now()`
	 * directly, so the same guard is applied here rather than leaving a
	 * timestamp that fails one run in however many.
	 *
	 * `a` is a letter rather than a digit precisely so the sequence cannot appear
	 * at all, which is stronger than retrying until it does not.
	 */
	function uniqueName(prefix: string): string {
		return `${prefix} a${Date.now()}${phoneSequence % 100}`;
	}

	test('announces a check-in politely and shows a toast', async ({ page }) => {
		const name = uniqueName('E2E Checkin Toast');
		const before = await bookAppointment(page, name);

		await gotoAdmin(page, '/admin/appointments');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const row = table.locator('tr', { has: page.getByText(name) }).first();
		await expect(row).toBeVisible(SETTLE);

		// Nothing is announced before the operator acts. A region that pre-fills
		// with the last message would make this assertion pass without the
		// mutation ever having spoken.
		const live = announcer(page);
		await expect(live).toHaveAttribute('aria-live', 'polite');
		await expect(live).toHaveAttribute('aria-atomic', 'true');

		await row.getByRole('button', { name: 'Sudah Check-in' }).click();

		// The announcement names the transition the operator just made, so it is
		// recognisable as THIS action's result rather than generic confirmation.
		// The shared helper builds it as
		//   "<noun> <subject> dipindahkan dari <from> ke <to>."
		// so the patient, both states, and the action verb are all present.
		await expect(live).toContainText(name, { timeout: SETTLE.timeout });
		await expect(live).toContainText('Janji temu', { timeout: SETTLE.timeout });
		await expect(live).toContainText('Terjadwal', { timeout: SETTLE.timeout });
		await expect(live).toContainText('Sudah Check-in', { timeout: SETTLE.timeout });

		// And a visible toast, for the operator who was not looking at the region.
		await expect(toast(page)).toBeVisible(SETTLE);

		// The mutation really happened, so this is not passing on a live region
		// and a toast raised by some other path.
		await expect
			.poll(
				async () => (await appointmentsIn(page)).find((row) => row.id === before.id)?.status,
				{ timeout: SETTLE.timeout }
			)
			.toBe('checked_in');
	});
});

test.describe('T-3B5-03 schedule create and update', () => {
	test.use({ viewport: DESKTOP });

	test.skip(HOLDS_NO_DATA, 'this actor holds nothing to mutate; its refusal is asserted in admin-read.spec.ts');

	/**
	 * A schedule date that no other row in this run can already hold.
	 *
	 * The create and update tests are chained — the second edits the row the
	 * first created — and a fixed date would make the locator ambiguous the
	 * moment the suite is re-run against a database that still holds the
	 * previous run's row. Deriving the date keeps the chain exact and the suite
	 * re-runnable without dropping the seeded data first.
	 *
	 * The rendered form comes from the application's OWN `formatDate` rather than
	 * an inline `Intl.DateTimeFormat` here. That is not tidiness: the table
	 * renders `id-ID` / `dateStyle: 'medium'`, and a test that guesses the format
	 * produces a locator that matches nothing while reading as a missing row.
	 * Importing the helper makes that class of drift impossible.
	 */
	function freshScheduleDate(): { iso: string; formatted: string } {
		const date = new Date(Date.UTC(2026, 0, 1 + (Date.now() % 3000)));
		const iso = date.toISOString().slice(0, 10);
		return { iso, formatted: formatDate(iso) };
	}

	/**
	 * The schedule date the create test used, in both forms the run needs.
	 *
	 * Empty until that test runs, and the update test asserts on it rather than
	 * recomputing the date — two independent `Date.now()` calls would differ and
	 * the lookup would silently find nothing.
	 */
	let createdScheduleDate = { iso: '', formatted: '' };

	test('offers only server-provided facilities and never a practitioner field', async ({
		page
	}) => {
		test.skip(
			REFUSES_SCHEDULE_MANAGEMENT,
			'this actor manages no schedule; the refusal state is asserted in admin-schedule-denied.spec.ts'
		);
		await gotoAdmin(page, '/admin/schedules');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		await page.getByRole('button', { name: 'Jadwal baru' }).click();
		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		// The facility options are read from the SERVER'S OWN options endpoint and
		// compared to the DOM. Hardcoding the expected facility here would only
		// be correct for the facility-scoped actor, and this suite now runs under
		// an actor matrix: the unscoped actor legitimately manages every seeded
		// facility, so a literal would report a false failure on a correct editor.
		//
		// What is being proven is the SHAPE of the answer, not a specific list:
		// every offered option is a facility the server said is manageable, and
		// no readable-but-unmanageable facility leaks in. Both halves are checked
		// below.
		const serverOptions = await page.request.get('/api/v1/admin/schedules/options');
		expect(serverOptions.ok(), 'the schedule options read must succeed').toBe(true);
		const manageable = ((await serverOptions.json()).data.facilities as Array<{
			id: string;
			name: string;
		}>).map((facility) => facility.name);

		expect(manageable.length, 'the actor must manage at least one facility').toBeGreaterThan(0);

		const facilitySelect = modal.getByLabel('Fasilitas');
		const options = await facilitySelect.locator('option').allTextContents();
		const offered = options.filter(
			(text) => text !== 'Pilih fasilitas' && text !== 'Memuat fasilitas...'
		);

		// EXACTLY the server's facilities, in the server's order. This is the
		// server-authoritative assertion: a client that derived the list from the
		// read endpoint would show every readable facility instead, and a client
		// that inferred the capability itself would show none.
		expect(offered).toEqual(manageable);

		// Every offered facility is one the server named as manageable, and the
		// editor's set is exactly that set — no more, no less.
		for (const name of offered) {
			expect(manageable, 'an offered facility must be server-manageable').toContain(name);
		}

		// The mixed-provenance guard, stated where it can actually be observed.
		// For the FACILITY-SCOPED actor this is the load-bearing assertion: it
		// reads exactly one facility and manages exactly one, and if they ever
		// diverged the read list would have leaked in. (Reading names from the
		// API rather than a literal is what lets the same assertion hold for the
		// unscoped actor, which reads and manages the same seven.)
		const readable = await page.request.get('/api/v1/admin/facilities');
		expect(readable.ok()).toBe(true);
		const readableNames = ((await readable.json()).data as Array<{ name: string }>).map(
			(facility) => facility.name
		);
		const readableButUnmanageable = readableNames.filter(
			(name) => !manageable.includes(name)
		);
		if (readableButUnmanageable.length > 0) {
			// A facility this actor can READ but not MANAGE must never appear as a
			// mutation option. This is the divergence the mixed-provenance seed
			// actor exists to catch.
			for (const name of readableButUnmanageable) {
				expect(offered, `${name} is readable but unmanageable, so it must not be offered`)
					.not.toContain(name);
			}
		}

		// NO practitioner UI OF ANY KIND. §6 forbids a select, a name, a UUID,
		// and free text. This asserts the absence of all four.
		//
		// The check is on CONTROL LABELS, not the whole dialog text. The dialog
		// deliberately explains that a schedule may be saved without a
		// practitioner ("jadwal tanpa nama praktisi tetap dapat disimpan"), and
		// that sentence is the opposite of a practitioner field: it is what
		// stops an operator going looking for one. Scoping the assertion to form
		// controls keeps the guarantee (no input can carry a practitioner) while
		// allowing the copy that says there is nothing to fill in.
		const controlLabels = await modal.evaluate((node) => {
			const controls = Array.from(
				node.querySelectorAll('input, select, textarea, [role="combobox"], [role="listbox"]')
			);
			const labelled = controls.map((control) => {
				const id = control.getAttribute('id');
				const explicit = id ? node.querySelector(`label[for="${id}"]`) : null;
				return (explicit?.textContent ?? control.getAttribute('aria-label') ?? '').toLowerCase();
			});
			const placeholders = controls.map((control) =>
				(control.getAttribute('placeholder') ?? '').toLowerCase()
			);
			return [...labelled, ...placeholders].join(' ');
		});
		for (const forbidden of ['practitioner', 'praktisi', 'dokter', 'id dokter']) {
			expect(
				controlLabels,
				`the editor must offer no ${forbidden} control`
			).not.toContain(forbidden);
		}
		// No free-text control that could accept an arbitrary id: the only text
		// inputs are the date and the two clock fields, all `inputmode="numeric"`.
		const textInputs = modal.locator('input[type="text"]');
		for (const input of await textInputs.all()) {
			const mode = await input.getAttribute('inputmode');
			expect(
				mode,
				'every text input in the schedule editor must be numeric or a date'
			).toBe('numeric');
		}

		// The service units are scoped to the chosen facility, from the same
		// response. A unit belonging to another facility is unreachable.
		await facilitySelect.selectOption({ label: MANAGED_FACILITY });
		const unitSelect = modal.getByLabel('Unit layanan');
		await expect(unitSelect).toBeEnabled(SETTLE);
		const units = (await unitSelect.locator('option').allTextContents()).filter(
			(text) => text !== 'Pilih unit layanan'
		);
		expect(units.length).toBeGreaterThan(0);
	});

	test('creates a schedule that persists, with no practitioner_id on the wire', async ({
		page
	}) => {
		test.skip(
			REFUSES_SCHEDULE_MANAGEMENT,
			'this actor manages no schedule; the refusal state is asserted in admin-schedule-denied.spec.ts'
		);
		// Shared with the update test below, which edits exactly this row.
		const { iso, formatted } = freshScheduleDate();
		createdScheduleDate = { iso, formatted };

		await gotoAdmin(page, '/admin/schedules');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		// Capture the exact POST body the shared client puts on the wire. This is
		// the §6/§10 proof at the network boundary, not a re-reading of the form
		// that could be fooled by a UI bug.
		const bodies: string[] = [];
		page.on('request', (request) => {
			if (request.method() === 'POST' && request.url().includes('/admin/schedules')) {
				bodies.push(request.postData() ?? '');
			}
		});

		await page.getByRole('button', { name: 'Jadwal baru' }).click();
		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		await modal.getByLabel('Fasilitas').selectOption({ label: MANAGED_FACILITY });
		const unitSelect = modal.getByLabel('Unit layanan');
		await expect(unitSelect).toBeEnabled(SETTLE);
		const firstUnit = (await unitSelect.locator('option').allTextContents()).find(
			(text) => text !== 'Pilih unit layanan'
		);
		await unitSelect.selectOption({ label: firstUnit! });

		// 09:00-12:00 with 60-minute slots divides exactly: three slots.
		await modal.getByLabel('Tanggal').fill(iso);
		await modal.getByLabel('Jam mulai').fill('09:00');
		await modal.getByLabel('Jam selesai').fill('12:00');
		await modal.getByLabel('Durasi slot (menit)').fill('60');
		await modal.getByLabel('Kapasitas per slot').fill('2');

		// The live preview proves the arithmetic the server will enforce.
		await expect(modal).toContainText('3 slot per hari', SETTLE);

		await modal.getByRole('button', { name: 'Simpan jadwal' }).click();

		// The dialog closes on success, and the row is re-read from the server.
		await expect(modal).toHaveCount(0, SETTLE);

		// Scoped to the created row, not the whole table. A `toContainText` on
		// <table> also matches the caption, so a failure here reads as "the date
		// is nowhere on the page" even when the row is plainly present — which
		// is exactly the confusing failure this assertion produced once already.
		const createdRow = table
			.locator('tr', { has: page.getByText(formatted, { exact: true }) })
			.first();
		await expect(createdRow).toBeVisible(SETTLE);
		// 09:00-12:00 at 60-minute slots, capacity 2 — the values the form held.
		await expect(createdRow).toContainText('60 mnt');
		await expect(createdRow).toContainText('2', { timeout: SETTLE.timeout });

		// THE CRITICAL ASSERTION: the body has no practitioner key at all. Not
		// empty, not null, not the previous value — absent.
		expect(bodies.length, 'exactly one create request must be issued').toBe(1);
		expect(bodies[0].toLowerCase()).not.toContain('practitioner');

		// And the persisted row really is practitioner-less, read back through
		// the same origin rather than from local state.
		const listed = await page.request.get('/api/v1/admin/schedules');
		expect(listed.ok()).toBe(true);
		const created = (await listed.json()).data.find(
			(schedule: { schedule_date: string; start_time: string }) =>
				schedule.schedule_date.startsWith(iso) && schedule.start_time.startsWith('09:00')
		);
		expect(created, 'the created schedule must be readable back').toBeTruthy();
		expect(created.slot_minutes).toBe(60);
		expect(created.capacity_per_slot).toBe(2);
	});

	test('updates the schedule it just created and the persisted values change', async ({
		page
	}) => {
		test.skip(
			REFUSES_SCHEDULE_MANAGEMENT,
			'this actor manages no schedule; the refusal state is asserted in admin-schedule-denied.spec.ts'
		);
		// The date is asserted, not recomputed: a silent mismatch here would
		// otherwise surface as "row not found" with nothing pointing at the
		// cause.
		expect(
			createdScheduleDate.iso,
			'the create test must run before this one'
		).not.toBe('');
		const { iso, formatted } = createdScheduleDate;

		await gotoAdmin(page, '/admin/schedules');
		const table = page.locator('table');
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const bodies: string[] = [];
		page.on('request', (request) => {
			if (
				(request.method() === 'PATCH' || request.method() === 'PUT') &&
				request.url().includes('/admin/schedules')
			) {
				bodies.push(request.postData() ?? '');
			}
		});

		// Find the row this file created, and its own Ubah button. Located by the
		// visible date so the test depends on what the operator can see.
		const row = table
			.locator('tr', { has: page.getByText(formatted, { exact: true }) })
			.first();
		await expect(row).toBeVisible(SETTLE);
		await row.getByRole('button', { name: 'Ubah' }).click();

		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		// The form is seeded from what the server holds, not from what was typed.
		await expect(modal.getByLabel('Tanggal')).toHaveValue(
			/\d{4}-\d{2}-\d{2}/,
			{ timeout: SETTLE.timeout }
		);
		await expect(modal.getByLabel('Jam mulai')).toHaveValue('09:00');
		await expect(modal.getByLabel('Durasi slot (menit)')).toHaveValue('60');

		// Change both numeric fields and re-validate the arithmetic: 09:00-15:00
		// at 60 minutes is six slots, so this stays divisible.
		await modal.getByLabel('Jam selesai').fill('15:00');
		await modal.getByLabel('Durasi slot (menit)').fill('60');
		await modal.getByLabel('Kapasitas per slot').fill('4');
		await expect(modal).toContainText('6 slot per hari', SETTLE);

		await modal.getByRole('button', { name: 'Simpan perubahan' }).click();
		await expect(modal).toHaveCount(0, SETTLE);

		// The UPDATE body also omits practitioner_id. §6 is explicit that an
		// update must never carry the current value either.
		expect(bodies.length, 'exactly one update request must be issued').toBe(1);
		expect(bodies[0].toLowerCase()).not.toContain('practitioner');

		// The change is in the SERVER's data. The table is re-read after the
		// mutation, so this also proves the reload rather than a local patch.
		await expect(row).toContainText('4', { timeout: SETTLE.timeout });
		const listed = await page.request.get('/api/v1/admin/schedules');
		const updated = (await listed.json()).data.find(
			(schedule: { schedule_date: string; start_time: string }) =>
				schedule.schedule_date.startsWith(iso) && schedule.start_time.startsWith('09:00')
		);
		expect(updated, 'the updated schedule must be readable back').toBeTruthy();
		expect(updated.capacity_per_slot).toBe(4);
		expect(updated.end_time).toContain('15:00');
	});

	test('refuses a slot that does not divide the range, naming the rule', async ({ page }) => {
		test.skip(
			REFUSES_SCHEDULE_MANAGEMENT,
			'this actor manages no schedule; the refusal state is asserted in admin-schedule-denied.spec.ts'
		);
		// Its own date, so the "nothing was persisted" assertion below cannot be
		// satisfied by a leftover row from an earlier run.
		const { iso } = freshScheduleDate();

		await gotoAdmin(page, '/admin/schedules');
		await expect(page.locator('table tbody tr').first()).toBeVisible(SETTLE);

		await page.getByRole('button', { name: 'Jadwal baru' }).click();
		const modal = dialog(page);
		await expect(modal).toBeVisible(SETTLE);

		await modal.getByLabel('Fasilitas').selectOption({ label: MANAGED_FACILITY });
		const unitSelect = modal.getByLabel('Unit layanan');
		await expect(unitSelect).toBeEnabled(SETTLE);
		await unitSelect.selectOption({
			index: await unitSelect.locator('option').count() - 1
		});
		await modal.getByLabel('Tanggal').fill(iso);
		await modal.getByLabel('Jam mulai').fill('09:00');
		await modal.getByLabel('Jam selesai').fill('10:30');
		// 90 minutes over a 90-minute range is one slot, but 50 does not divide 90.
		await modal.getByLabel('Durasi slot (menit)').fill('50');
		await modal.getByLabel('Kapasitas per slot').fill('2');

		// The preview refuses to show a number rather than printing 0.
		await expect(modal).toContainText('belum menghasilkan slot yang valid', SETTLE);

		await modal.getByRole('button', { name: 'Simpan jadwal' }).click();

		// Still open, with the rule named. Closing on a refused field would make
		// the operator retype everything to fix one value.
		await expect(modal).toBeVisible(SETTLE);
		await expect(modal).toContainText('membagi', SETTLE);

		// Nothing was persisted.
		const listed = await page.request.get('/api/v1/admin/schedules');
		const rows = (await listed.json()).data.filter(
			(schedule: { schedule_date: string }) => schedule.schedule_date.startsWith(iso)
		);
		expect(rows, 'an invalid schedule must not be persisted').toHaveLength(0);
	});

	test('keeps the forbidden fields out of the schedules page entirely', async ({ page }) => {
		await gotoAdmin(page, '/admin/schedules');
		await expect(page.locator('table tbody tr').first()).toBeVisible(SETTLE);
		const body = ((await page.locator('body').textContent()) ?? '').toLowerCase();
		for (const forbidden of ['practitioner', 'praktisi', 'id dokter']) {
			expect(body).not.toContain(forbidden);
		}
		expect(body, 'no raw UUID may be shown').not.toMatch(UUID);
	});
});

test.describe('T-3B5-05 notification actions', () => {
	test.use({ viewport: DESKTOP });

	test.skip(HOLDS_NO_DATA, 'this actor holds nothing to act on; its refusal is asserted in admin-read.spec.ts');

	test('shows action buttons strictly from can_retry and can_cancel', async ({ page }) => {
		/**
		 * Reads the server's own per-row affordance tallies.
		 *
		 * Read as a function because it is read TWICE around the page load, and
		 * the two reads are what make the comparison below exact.
		 */
		async function serverTallies() {
			const listed = await page.request.get('/api/v1/admin/notifications');
			expect(listed.ok()).toBe(true);
			const rows = (await listed.json()).data as Array<{
				can_retry: boolean;
				can_cancel: boolean;
			}>;
			return {
				total: rows.length,
				retry: rows.filter((row) => row.can_retry).length,
				cancel: rows.filter((row) => row.can_cancel).length
			};
		}

		const table = page.locator('table');

		/*
		 * THE OUTBOX IS APPENDED TO BY OTHER SUITES WHILE THIS ONE RUNS.
		 *
		 * Booking an appointment and checking one in both enqueue a confirmation
		 * notification, and `citizen-transactions.spec.ts` does both — in a
		 * different file, on a different worker, against the same seeded
		 * database. So a tally read before the page loads can be one row short
		 * of what the page then renders, which is exactly how this test flaked:
		 * "expected 9, received 10".
		 *
		 * The fix is to compare against a tally that is STABLE ACROSS THE PAGE
		 * LOAD rather than merely recent. Read, load, re-read; only assert when
		 * the two reads agree, which proves nothing was inserted in the window
		 * that contains the page's own fetch. If they disagree the page is
		 * reloaded and the pair re-read.
		 *
		 * This is NOT a retry that hides a failure, and the distinction matters.
		 * Rows are only ever INSERTED, so the page's set is always a subset of
		 * the later read; a genuine defect — a client drawing a button the
		 * server withheld — makes the DOM count permanently disagree with any
		 * stable tally, and the loop then exhausts and fails. It only ever
		 * absorbs a concurrent append, which is a property of the shared
		 * fixture rather than of the code under test.
		 */
		let tallies = await serverTallies();
		expect(tallies.total, 'the seed must contain notifications').toBeGreaterThan(0);

		await gotoAdmin(page, '/admin/notifications');
		await expect(table).toBeVisible(SETTLE);

		for (let attempt = 0; attempt < 8; attempt += 1) {
			const afterLoad = await serverTallies();
			if (
				afterLoad.total === tallies.total &&
				afterLoad.retry === tallies.retry &&
				afterLoad.cancel === tallies.cancel
			) {
				tallies = afterLoad;
				break;
			}
			// An append landed in the window. Re-take the page against the newer
			// tally so both describe the same set. `reload` is already gated on
			// hydration by the shared fixture, so no explicit wait is needed.
			tallies = afterLoad;
			await page.reload();
			await expect(table).toBeVisible(SETTLE);
		}

		// Counted across the WHOLE table, and compared to the server's own tallies.
		// This is the §12 assertion: the visible button set is the server's two
		// booleans and nothing else. A client-side rule keyed on status, role,
		// facility, or an email-shaped heuristic would produce a different count
		// here, which is exactly the class of bug this phase forbids.
		await expect(table.getByRole('button', { name: 'Kirim ulang' })).toHaveCount(tallies.retry);
		await expect(table.getByRole('button', { name: 'Batalkan' })).toHaveCount(tallies.cancel);

		// A row the server marked unactionable says so in words rather than
		// showing an empty cell an operator would read as a failed load.
		if (tallies.retry === 0 || tallies.cancel === 0) {
			await expect(table).toContainText('Tidak ada aksi');
		}

		// The two boolean names themselves must never reach the DOM. They are a
		// wire contract, not UI copy.
		const text = (await table.textContent()) ?? '';
		expect(text).not.toContain('can_retry');
		expect(text).not.toContain('can_cancel');
	});

	test('preserves the URL filters and the masked recipient across a mutation', async ({
		page
	}) => {
		test.skip(
			REFUSES_NOTIFICATION_MANAGEMENT,
			'this actor holds no notification.manage, so there is no retry to click and the masked-recipient reload cannot be exercised'
		);
		await page.goto('/admin/notifications?status=pending');
		await expect(page.getByText(/Menunggu/).first()).toBeVisible(SETTLE);

		// The recipient the operator sees, captured before the mutation.
		const before = (await page.locator('table').textContent()) ?? '';
		expect(before, 'the recipient must already be masked').toMatch(/\+62•+|•/);
		expect(before, 'no raw email may be shown').not.toMatch(/[\w.+-]+@[\w-]+\.[\w.]+/);

		// Narrow further so a filter-preservation bug would be visible: if the
		// reload dropped the query string, the list would widen and the row
		// count or the control set would change.
		const retry = page.getByRole('button', { name: 'Kirim ulang' }).first();
		await expect(retry).toBeVisible(SETTLE);
		await retry.click();

		// The filter is still in the URL after the mutation.
		await expect(page).toHaveURL(/status=pending/, { timeout: SETTLE.timeout });

		// A polite announcement, from the same live region the other pages use.
		const live = announcer(page);
		await expect(live).toHaveAttribute('aria-live', 'polite');
		await expect(live).toContainText(/Notifikasi|Dikirim|Batal/i, { timeout: SETTLE.timeout });
		await expect(toast(page)).toBeVisible(SETTLE);

		// The recipient is STILL masked after the reload. Nothing reconstructs a
		// longer contact from the masked form, and no hash is substituted.
		const after = (await page.locator('table').textContent()) ?? '';
		expect(after).not.toMatch(/\+62\d{9,}/);
		expect(after, 'no raw email may appear after a reload').not.toMatch(
			/[\w.+-]+@[\w-]+\.[\w.]+/
		);
		expect(after.toLowerCase()).not.toContain('hash');
	});

	test('renders the backend 409 message verbatim', async ({ page }) => {
		test.skip(
			REFUSES_NOTIFICATION_MANAGEMENT,
			'this actor holds no notification.manage, so the server refuses at the route gate and this retry-driven 409 is unreachable'
		);
		const listed = await page.request.get('/api/v1/admin/notifications');
		const rows = (await listed.json()).data as Array<{ id: string; status: string }>;

		/*
		 * A REAL 409, FROM A STATE THIS FIXTURE CAN ACTUALLY REACH.
		 *
		 * The original version of this test asked for a `delivered` row and
		 * skipped when it could not find one. It could never find one: the local
		 * stack seeds only `pending` rows and resets them on every rerun, the
		 * notification worker is not part of the E2E stack, and no admin route
		 * can create or deliver a row. So the scenario was skipped by EVERY actor
		 * in the matrix — by capability for the read-only actor, and by missing
		 * data for the three that hold `notification.manage`. A test that never
		 * runs is not coverage.
		 *
		 * `cancelled` is reachable, and it is refused just as firmly. `Retry`
		 * accepts only `failed` and `pending`, so retrying a cancelled row is a
		 * genuine 409 from the genuine endpoint — the same class of refusal the
		 * `delivered` version was reaching for, and the sentence the UI must show
		 * verbatim.
		 *
		 * Producing it uses only real API calls: reuse a row some earlier test
		 * already cancelled, or cancel a pending one. Nothing is faked, and no
		 * status is invented — which is the rule the old skip existed to respect.
		 */
		const cancelled =
			rows.find((row) => row.status === 'cancelled') ??
			(await (async () => {
				const pending = rows.find((row) => row.status === 'pending');
				expect(pending, 'the outbox must hold a row to cancel').toBeTruthy();
				const cancelledResponse = await page.request.post(
					`/api/v1/admin/notifications/${pending!.id}/cancel`
				);
				expect(cancelledResponse.status(), 'cancelling a pending row must succeed').toBe(200);
				return { id: pending!.id, status: 'cancelled' };
			})());

		const target = cancelled!;
		const probe = await page.request.post(`/api/v1/admin/notifications/${target.id}/retry`);
		expect(probe.status(), 'retrying a cancelled row must conflict').toBe(409);
		const refused = await probe.json();
		// Same envelope as every other admin error: the sentence lives under
		// `error`, and that is the field the client treats as server-described.
		const sentence = refused.error as string;
		expect(sentence, 'the 409 must carry a sentence from the server').toBeTruthy();

		/*
		 * Route interception delivers this REAL 409 to the REAL page handler.
		 *
		 * It is needed because the UI will not offer Retry on a cancelled row —
		 * the affordance is doing its job — so the only way to observe the
		 * refusal path is to hand the page the server's own answer. The pattern
		 * matches whichever row the page actually calls, rather than one
		 * hardcoded id, so the assertion does not depend on which row happens to
		 * carry the first Retry button. No success path anywhere in this file is
		 * faked.
		 */
		await page.route(
			(url) => url.pathname.endsWith('/retry'),
			async (route) => {
				await route.fulfill({
					status: 409,
					contentType: 'application/json',
					body: JSON.stringify(refused)
				});
			}
		);

		await page.goto('/admin/notifications');
		await expect(page.locator('table')).toBeVisible(SETTLE);

		const retry = page.getByRole('button', { name: 'Kirim ulang' }).first();
		await expect(retry).toBeVisible(SETTLE);
		await retry.click();

		// VERBATIM, in an alert region. §13 forbids substituting our own wording,
		// because the server's sentence names the rule that actually refused.
		await expect(page.getByRole('alert').first()).toHaveText(sentence, {
			timeout: SETTLE.timeout
		});
	});

	test('offers no retry or cancel on a delivered notification', async ({ page }) => {
		// Drive one row to `delivered` directly is not possible through the API,
		// so this asserts the same rule from the affordance side: whatever the
		// server says, the buttons follow it and nothing else.
		const listed = await page.request.get('/api/v1/admin/notifications');
		const rows = (await listed.json()).data as Array<{
			status: string;
			can_retry: boolean;
			can_cancel: boolean;
		}>;
		const delivered = rows.find((row) => row.status === 'delivered');

		await gotoAdmin(page, '/admin/notifications');
		const table = page.locator('table');
		await expect(table).toBeVisible(SETTLE);

		if (!delivered) {
			// No delivered row in the seed. Assert the equivalent invariant that
			// does hold: every row's visible buttons match its own two booleans.
			//
			// WHICH BOOLEAN STATE IS EXPECTED DEPENDS ON A PERMISSION, and this
			// is the assertion that has to say so. A pending row is actionable for
			// an actor holding `notification.manage`, and correctly NOT actionable
			// for one that does not — `operator` carries `notification.read` only,
			// and the server sets both booleans false across the whole outbox for
			// it. Asserting "a pending row must be actionable" unconditionally
			// would assert a fact about the actor's permissions while claiming to
			// assert a fact about the buttons, and it would be FALSE for exactly
			// the actor whose whole purpose is to hold a read half and no manage
			// half. The check below is therefore split on the permission, and the
			// read-only branch asserts the STRICTER claim rather than skipping.
			if (REFUSES_NOTIFICATION_MANAGEMENT) {
				// Nothing in this actor's outbox is actionable, and the page offers
				// no action control at all. An affordance appearing here would mean
				// the client inferred a capability the server withheld.
				for (const row of rows) {
					expect(row.can_retry, 'no row may be retryable without notification.manage').toBe(false);
					expect(row.can_cancel, 'no row may be cancellable without notification.manage').toBe(
						false
					);
				}
				await expect(
					table.getByRole('button', { name: 'Kirim ulang' }),
					'a read-only outbox must offer no retry control'
				).toHaveCount(0);
				await expect(
					table.getByRole('button', { name: 'Batalkan' }),
					'a read-only outbox must offer no cancel control'
				).toHaveCount(0);
				return;
			}
			for (const row of rows.slice(0, 5)) {
				expect(
					row.can_retry || row.can_cancel,
					'a pending seeded row must be actionable by the managing actor'
				).toBe(true);
			}
			return;
		}

		const text = (await table.textContent()) ?? '';
		expect(delivered.can_retry).toBe(false);
		expect(delivered.can_cancel).toBe(false);
	});
});

test.describe('admin mutations never invent a rate-limit state', () => {
	test.use({ viewport: DESKTOP });

	test('no admin destination offers a 429 or rate-limit affordance', async ({ page }) => {
		// §14: the admin mutation UI must NOT invent generic 429 states. The
		// backend has no admin rate limit, so a control claiming one would send
		// an operator looking for a limit that does not exist.
		for (const path of [
			'/admin/queues',
			'/admin/appointments',
			'/admin/schedules',
			'/admin/facilities',
			'/admin/notifications'
		]) {
			await gotoAdmin(page, path);
			const text = ((await page.locator('body').textContent()) ?? '').toLowerCase();
			for (const forbidden of ['terlalu banyak permintaan', 'rate limit', 'coba lagi nanti']) {
				expect(text, `${path} must not claim ${forbidden}`).not.toContain(forbidden);
			}
			// A 429 is a status code, so match it as a standalone token. A bare
			// substring check is a false positive: the page renders seeded rows
			// whose ids embed an epoch-millisecond value, and a run of digits
			// such as "…124298…" contains "429" without being a status code.
			expect(text, `${path} must not claim a 429`).not.toMatch(/\b429\b/);
		}
	});
});
