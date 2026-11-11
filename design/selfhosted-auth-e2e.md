# Self-hosted auth E2E — the four-actor journey

The procedure of record for `apps/web/e2e/selfhosted-auth-journey.spec.ts`, the
one browser spec that exercises SIGAP's **real** authentication path instead of
the local test actor.

Related documents:

- `design/better-auth-integration-contract.md` — the auth seam itself.
- `design/auth-integration-inspection.md` §2b/§3 — why `app_users.subject` is
  the one piece of data the JWT path needs.
- `design/staff-provisioning.md` — the reviewed staff table and the
  id-keyed, never-email-keyed rule this spec's prep script follows.

---

## 1. What it proves

Every other spec in `apps/web/e2e/` reaches the Go API through the LOCAL
selector (`SIGAP_LOCAL_E2E_ACTOR` → `X-Sigap-Local-Test-Subject`), which names an
identity without authenticating anyone. This journey signs in through the real
form and carries a real session:

```
login FORM  →  Better Auth server action (self-hosted, `auth` schema in the
same Postgres)  →  `better-auth.session_token` cookie  →  hooks.server.ts mints
an ES256 JWT (auth.api.getToken)  →  the Go API verifies it against the web
tier's JWKS  →  rbac_resolver resolves `sub` → app_users.subject  →
permissions / organization scope / patient ownership from the database
```

It only means anything against the preview that turns the local selectors off:

```pwsh
pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1 -Jwt -NoAdminActor
```

`-Jwt` selects `SIGAP_AUTH_MODE=jwt` and clears `SIGAP_DEV_IDENTITY` /
`SIGAP_LOCAL_RBAC_TEST_IDENTITY`; `-NoAdminActor` leaves `SIGAP_LOCAL_E2E_ACTOR`
unset. The Go log line to check before believing a run is
`auth provider configured mode=jwt` (API log:
`.local-preview/api.log`). An unauthenticated admin read then answers 403, and
the `X-Sigap-Local-Test-Subject` header is ignored — both were verified while
writing this document.

## 2. Preconditions

| # | Precondition | How it is satisfied |
|---|---|---|
| 1 | Postgres on `127.0.0.1:5433/sigap`, seeded (`dev.sql`, `demo.sql`, `regional_tenancy.sql`, `rbac.sql`) | the preview script verifies the database is reachable before it starts anything; the seeds are what create the `app_users` rows and grants the journey resolves |
| 2 | Preview up in JWT mode with no admin actor | the command in §1 |
| 3 | Four Better Auth accounts exist and are mapped to the right `app_users` rows | `node scripts/prepare-auth-e2e.mjs --apply` |
| 4 | The citizen fixture holds two **scheduled** appointments | the same script resets them (see §5) |

`prepare-auth-e2e.mjs` is the deterministic half of the journey: the spec
asserts a fixed state, so something has to put the database into it. Dry run is
the default; nothing is written without `--apply`.

## 3. How to run

```pwsh
# 1. the stack (JWT authoritative, no local actor)
pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1 -Jwt -NoAdminActor

# 2. the fixtures — plan first, then write
cd apps/web
node scripts/prepare-auth-e2e.mjs --from-env-file                 # dry run
node scripts/prepare-auth-e2e.mjs --from-env-file --apply         # write

# 3. the journey (opt-in: it needs the preview mode AND the prepared fixtures;
#    the rest of the suite runs against a disposable cluster and must not be
#    made to depend on either)
$env:SIGAP_E2E_SELFHOSTED_AUTH = '1'
$env:SIGAP_E2E_BASE_URL = 'http://127.0.0.1:5173'
npx playwright test e2e/selfhosted-auth-journey.spec.ts --retries=0
```

`--retries=0` because the journey consumes its own fixture (§5, §8); see §8 for
the recorded run.

`SIGAP_E2E_SELFHOSTED_AUTH=1` is the opt-in the spec checks: without it the four
cases report as skipped with the recipe in the skip message. That is deliberate
rather than incidental — `scripts/dev/Start-LocalE2E.ps1` boots a disposable
cluster from the seeds and runs the whole suite as a pinned local actor, where
the fixture accounts do not exist and no form sign-in can succeed. The flag is
what keeps that harness from depending on a prepared developer database, and the
skip message is what keeps the flag from being forgotten.

`--from-env-file` matters on this workstation: the ambient `DATABASE_URL` of the
developer shell names the **neighbouring project's** Postgres on port 5434. The
script refuses a loopback target on a forbidden port with no override, and
`--from-env-file` resolves the target from this repo's `.env` files instead. If
your shell's ambient `SIGAP_DATABASE_URL` already points at
`127.0.0.1:5433/sigap`, the flag is unnecessary.

## 4. The actors

| Spec case | Login | `app_users.id` | Row email | Better Auth account | What it is for |
|---|---|---|---|---|---|
| patient A | `smoke@sigap.local` | `a62e05c1-…-742e` | `patient-a@sigap.local` (drift) | pre-existing | owns the two fixture appointments |
| patient B | `smoke-b@sigap.local` | `949eae99-…-a482` | same | pre-existing | an account with **no** appointments |
| org A admin | `org-a-admin@sigap.local` | `…c301` | `org-admin-a@sigap.local` (drift) | pre-existing | `organization_admin` scoped to `RS Sehat Nusantara Group` |
| super admin | `global-admin@sigap.local` | `…d990` | same | pre-existing | unscoped `super_admin` |

Two things about this table are deliberate:

- **Row-email drift is not a linking rule.** `smoke@sigap.local` signs in to the
  `app_users` row whose own email is `patient-a@sigap.local`. The subject write
  is `WHERE id = <reviewed UUID>` — email is used only to locate the Better Auth
  account, whose own schema keys accounts on email. The script prints the drift
  instead of "resolving" it.
- **Replacing the dev subjects.** `…c301` / `…d990` currently carry the dev
  selector subjects (`local-org-admin-a`, `local-global-super-admin`). Writing
  the Better Auth ids there is what makes the JWT path authorize these actors —
  and it is also why a **dev-mode** preview (`Start-V22Preview.ps1` with no
  flags, admin actor default) would then find no actor for those two subjects.
  To go back: re-run the seeds (`packages/db/seed/dev.sql`,
  `regional_tenancy.sql` upsert `subject` on conflict), or write the `local-*`
  values back. The script's dry-run table shows exactly this change before
  anything is written.

## 5. The citizen fixture and why it is reset

Patient A's two appointments are hand-built fixture rows (facility
`RS Sehat Nusantara Cimahi`, org A, and `Puskesmas Prima Tangerang`, org B):

| Appointment id | Facility | Prepared state |
|---|---|---|
| `5d2a54cb-…-3056` | RS Sehat Nusantara Cimahi | `scheduled` |
| `730ec946-…-7f1e` | Puskesmas Prima Tangerang | `scheduled` |

The spec's cancellation is a **real, one-way** transition: it drives the
citizen's own `scheduled → cancelled` path through the UI. A second run would
therefore find one card already cancelled and nothing eligible to cancel, so
`prepare-auth-e2e.mjs --apply` restores both rows to `scheduled`
(`cancelled_at = NULL`) before each run. Nothing else about them (id, facility,
time, check-in code, owner) is touched.

The reset is guarded and refuses any row that left the citizen-controlled
states: a `checked_in`, `queued`, `completed` or `no_show` appointment is a real
visit record, not fixture state, and the script fails rather than rewriting it.
It also refuses a row whose `patient_account_id` or facility name does not match
the reviewed fixture, and reports an error if the owner holds a number of
appointments other than the two the spec asserts on.

## 6. What each assertion proves

| # | Assertion | The rule it proves | Owner in the codebase |
|---|---|---|---|
| 1 | Sign-in through the form sets a `session_token` cookie and leaves `/auth/login` without an error banner | the server action's credential path works end to end against the self-hosted store | `apps/web/src/routes/auth/login/+page.server.ts` |
| 2 | Patient A sees **exactly two** cards, both seeded facilities by name, both `Terjadwal`, and the cards equal the API's own list | the account-scoped list is the citizen's own appointments — resolved by identity, sorted newest-first, not by any client-supplied id | `apps/api/internal/handler/patient_appointments.go` (`listMyAppointmentsSQL`: `WHERE a.patient_account_id = $1`) |
| 3 | Cancelling one card through the dialog flips **that** card's badge to `Dibatalkan`; the API then reports exactly **one** still-scheduled appointment; the page shows one `Terjadwal` + one `Dibatalkan` over the same two cards | the citizen-controlled transition is the server's, the list is re-read rather than locally sliced, and the status *wording* is what changed | `apps/api/internal/handler/booking.go::CancelAppointment` (guarded `WHERE id=$1 AND status='scheduled'`), `routes/patient/appointments/+page.svelte` (`await load()`) |
| 4 | Patient B's list is `[]` — no cards, no copy, neither facility name anywhere in the body | one citizen's data never reaches another's session, and an empty account renders the empty state instead of someone else's rows | same list handler; the actor resolves from B's JWT `sub` |
| 5 | B POSTs A's live check-in code to `/api/v1/appointments/cancel` → **403** `Anda tidak berhak membatalkan janji temu ini.`, and the code-keyed status lookup still says `scheduled` afterwards | possession of a check-in code is NOT sufficient to cancel an appointment that has an owner; the ownership re-check runs before the status switch and changes nothing | `booking.go::CancelAppointment` ("Server-side ownership re-check") |
| 6 | Org A admin's directory renders **exactly one** row (`RS Sehat Nusantara Group`), no sibling name, no UUID, and the direct read agrees | an `organization_admin` sees only its own organization's directory | `apps/api/internal/handler/org_overview.go` (`ListOrganizations` → `OrganizationScopeForActor`), `routes/api/v1/admin/organizations/+server.ts` |
| 7 | Org A admin's direct fetch of org B's id answers **404** `Organisasi tidak ditemukan.` without leaking org B's name | the scope is re-resolved per request and an out-of-scope id is indistinguishable from a forged one — the directory link is not the control | `org_overview.go` (`GetOrganizationOverview`: "a forged organization_id for a sibling org is denied here") |
| 8 | Super admin's directory renders **both** organizations, each linked to its own `/admin/organizations/<id>` | the unscoped `super_admin` grant is the one that confers global reach | `apps/api/internal/auth/rbac_resolver.go` (`superAdminRoleName` → `Unrestricted`) |
| 9 | Each case signs out through `/auth/logout`, the session cookie is gone, and the gated page then answers `Silakan masuk` | sign-out removes the credential, and the page distinguishes "no session" from "empty data" — the refusal is API-backed, not decorative | `routes/auth/logout/+page.server.ts`, `lib/ui/UnauthPanel.svelte`, `lib/admin/AdminReadState.svelte` |

Two deliberate non-assertions: the journey does not exercise email delivery,
2FA, or password reset (no mail transport is in the local stack), and it does
not attempt a remote target — `playwright.config.ts` rejects any non-loopback
base URL before a browser launches.

## 7. Evidence a run emits

**From `prepare-auth-e2e.mjs`** (stdout, and with `--log <path>` as a markdown
block):

- the resolved target (`127.0.0.1:5433/sigap (local)`) and the mode
  (`DRY RUN` / `APPLY`);
- per actor: the `app_users` row, its row email (drift flagged), the Better Auth
  id with its `createdAt`, the `subject` old → new, and the credential probe
  result (`verified`, or `FAILED: …` — a failing probe means the spec cannot sign
  in and is reported before the browser starts);
- per fixture appointment: `scheduled (unchanged)` or `cancelled → scheduled`
  with the `cancelled_at` disposition;
- the owner summary: `N appointment(s), M scheduled`;
- the **CHANGE LOG (old → new)** table (entity, identity, old, new) and
  `counts: identities=… appointments=… changed=… audit rows=… errors=…`.

**From the spec** (Playwright's `list` reporter):

- a pass/fail per case, in one worker, in order (`serial` mode — the four cases
  share one database);
- on failure: the trace (`trace: 'retain-on-failure'`), plus the assertion
  message, which names the fact that broke (`exactly one appointment remains
  scheduled`, `an out-of-scope organization id must be denied`, …).

The observed counts of a real run are recorded in §8.

## 8. Recorded run

The run performed while writing this document, on the local stack
(`2026-11-11`, preview `-Jwt -NoAdminActor`, warm dev server):

```
$ cd apps/web
$ node scripts/prepare-auth-e2e.mjs --from-env-file --apply
  ...
  counts: identities=4  appointments=2  changed=1  audit rows=14  errors=0
  password: verified (probe session discarded; 1 session(s) revoked)   # all four actors
  Puskesmas Prima Tangerang (7f1e): cancelled → scheduled  (cancelled_at cleared)
  owner …742e: 2 appointment(s), 2 scheduled

$ SIGAP_E2E_SELFHOSTED_AUTH=1 SIGAP_E2E_BASE_URL=http://127.0.0.1:5173 \
      npx playwright test e2e/selfhosted-auth-journey.spec.ts --retries=0
  Running 4 tests using 1 worker
  ok 1  patient A: two cards, and cancelling one through the UI leaves one scheduled  (7.1s)
  ok 2  patient B: an empty list, and A's appointment cannot be cancelled             (4.9s)
  ok 3  org A admin: exactly one organization, and a sibling org id is refused        (4.4s)
  ok 4  super admin: the directory holds both organizations                           (4.9s)
  4 passed (22.6s)

# the companion run, without the opt-in — what the rest of the suite sees
$ SIGAP_E2E_BASE_URL=http://127.0.0.1:5173 npx playwright test \
      e2e/selfhosted-auth-journey.spec.ts --retries=0
  4 skipped
```

Observed counts, read back from Postgres after the run:

| Fact | Observed |
|---|---|
| Patient A, before the cancel | 2 cards — `Puskesmas Prima Tangerang` (03:00Z) + `RS Sehat Nusantara Cimahi` (02:00Z), both `Terjadwal` |
| Patient A, after the cancel | 2 cards — `Puskesmas Prima Tangerang` `Dibatalkan` (the card the spec cancelled), `RS Sehat Nusantara Cimahi` `Terjadwal` → **exactly 1 still scheduled** (code `E4A5G9`) |
| Patient B | 0 appointments (`appointments` count by `patient_account_id` = 0); UI = `Belum ada janji temu`; no A facility name anywhere |
| B cancelling A's live appointment (`E4A5G9`) | `403` `Anda tidak berhak membatalkan janji temu ini.`; the code-keyed status lookup still answered `scheduled` |
| Org A admin directory | 1 organization: `RS Sehat Nusantara Group` (org A only) |
| Org A admin fetching org B's id | `404` `Organisasi tidak ditemukan.` |
| Super admin directory | 2 organizations, both linked to their own overview |
| Sessions after the run | 0 live `auth.session` rows for all four fixture accounts — every case signed out |

`--retries=0` is deliberate: the journey mutates its own fixture, so an
automatic retry of a partly-completed case would assert against a consumed
pre-state and report a second, misleading failure. Note that a CI environment
sets `retries: 1` in `playwright.config.ts`; on a developer machine the flag is
redundant.

## 9. Failure modes worth knowing

| Symptom | Cause |
|---|---|
| every login lands back on the form with `Email atau kata sandi salah.` | the fixtures were never prepared, or the password was changed out of band — run the prep script with `--apply` and read its probe line |
| the login form is replaced by `Masuk belum tersedia di lingkungan ini.` | the web process has no `SIGAP_DATABASE_URL`/`BETTER_AUTH_SECRET` — it was not started by the preview script |
| A's list is empty (`Belum ada janji temu`) although the DB holds two rows | `app_users.subject` for the A row does not match the Better Auth `sub` (unprepared, or the seeds rewrote the dev subject after the prep script ran) |
| every admin page shows `Silakan masuk` while signed in | the JWT path resolved no actor: the staff `subject` is still `local-*` |
| the A case fails with `the prepared fixture must hold exactly two scheduled appointments` | the run consumed its own fixture (the previous run's cancellation is real) — re-run the prep script; it is also the message you get when a second actor cancelled the fixture concurrently |
| `ERR_CONNECTION_REFUSED` at `page.goto` | `SIGAP_E2E_BASE_URL` was not set to `http://127.0.0.1:5173` (the default 4173 is a different server) |
| the first run after a fresh `vite dev` boot times out in `waitForHydration` (`data-sigap-hydrated` never becomes `true`) | Vite's dependency re-optimization: the first browser request triggers `optimized dependencies changed. reloading` (`.local-preview/web.log`) and the in-flight document never hydrates. Open `/auth/login` in a browser once after starting the preview, or simply re-run the spec — the module graph is warm from then on. Not a spec or app defect: a bare `chromium.launch()` load of the same URL hydrates in ~2 s immediately afterwards |
