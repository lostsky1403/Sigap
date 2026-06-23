# Sigap Notification Foundation — Design Report

> **Status**: foundation (Phase 9). No real vendor integration.
> **Scope**: dev / local delivery simulation only. Real SMS, WhatsApp, and email
> provider integrations are **intentionally deferred** to a later phase.
> **Privacy posture**: privacy-first by construction. No raw patient contact is
> stored, returned, or logged.

---

## 1. Why this exists

Sigap already books appointments and prints queue tickets, but patients
receive no proactive acknowledgement. Without a foundation, every future
"send a reminder" or "tell the patient their queue number moved up"
feature would invent its own (and inevitably inconsistent) ad-hoc
implementation. This milestone adds a stable, privacy-first foundation
that any future provider integration can plug into without changing the
data model or the audit story.

The foundation is deliberately *boring*: deterministic, offline,
testable, and free of secrets.

---

## 2. Architecture overview

```
┌────────────────┐    fire-and-forget     ┌────────────────────────────┐
│ POST           │  ───────────────────►  │ notification.Service       │
│ /api/v1/       │   goroutine            │   - validate               │
│ appointments   │                        │   - mask contact           │
│ (Book /        │                        │   - hash contact (SHA-256) │
│  CheckIn)      │                        │   - reject phone leaks     │
└────────────────┘                        │   - INSERT outbox row      │
                                          └──────────┬────────────────┘
                                                     │
                                                     ▼
                                          ┌────────────────────────────┐
                                          │ notification_outbox        │
                                          │   id, status=pending,      │
                                          │   recipient_contact_       │
                                          │   masked, hash (32B),      │
                                          │   subject, body_template,  │
                                          │   attempt_count=0          │
                                          └──────────┬────────────────┘
                                                      │
                                           ┌────────────────────────────┐
                                           │ notification-worker        │
                                           │ (apps/api/cmd/             │
                                           │  notification-worker/)     │
                                           │  - FOR UPDATE SKIP LOCKED  │
                                           │  - MaxAttempts=3           │
                                           │  - backoff 1m→5m→15m       │
                                           └──────────┬─────────────────┘
                                                      │
                                                      ▼
                                           ┌────────────────────────────┐
                                           │ DevProvider.Deliver        │
                                          │   - no network call         │
                                          │   - deterministic outcome  │
                                          │     on UUID hash            │
                                          │   - writes one              │
                                          │     notification_delivery_ │
                                          │     attempts row            │
                                          │   - updates outbox status   │
                                          └────────────────────────────┘
```

The notification worker drains pending rows out of `notification_outbox` in
this milestone. It is a separate binary under
`apps/api/cmd/notification-worker/` and depends on the existing
`internal/notification` package (DevProvider included). The worker ships
in this PR with DevProvider only; no real vendor provider is wired.
Execution is manual via `go run ./cmd/notification-worker` — there is no
docker-compose service for the worker in this PR. See the **Notification
Worker** subsection below for the full claim strategy, backoff schedule,
and privacy guards.

---

## 3. Data model

Three forward-only tables are added by
`packages/db/migrations/0006_notifications.sql`. No existing table,
column, index, sequence, or row is modified.

### `notification_templates`

| Column           | Type         | Notes |
|------------------|--------------|-------|
| `template_key`   | TEXT PK      | e.g. `appointment.booked.confirmation` |
| `channel`        | TEXT         | CHECK in {dev, sms, whatsapp, email} |
| `subject_template` | TEXT (1..200) | Safe subject only |
| `body_template`  | TEXT (1..4000) | Template with `{placeholder}` variables only |
| `is_active`      | BOOLEAN      | |
| `created_at` / `updated_at` | TIMESTAMPTZ | |

Two demo templates are seeded: `appointment.booked.confirmation` and
`appointment.checked_in.confirmation`. **No vendor templates are
seeded.**

### `notification_outbox`

| Column                       | Type | Notes |
|------------------------------|------|-------|
| `id`                          | UUID PK | |
| `facility_id`                 | UUID FK (NULLABLE, ON DELETE SET NULL) | |
| `channel`                     | TEXT  | CHECK in {dev, sms, whatsapp, email} |
| `template_key`                | TEXT  | |
| `subject`                     | TEXT (1..200) | |
| `body_template`               | TEXT (1..4000) | |
| `recipient_type`              | TEXT  | CHECK in {patient, staff, facility_admin} |
| `recipient_contact_masked`    | TEXT (3..200) | The ONLY contact-shape field stored |
| `recipient_contact_hash`      | BYTEA (32) | SHA-256 of the normalised contact (dedup only) |
| `status`                      | TEXT  | CHECK in {pending, processing, delivered, failed, cancelled} |
| `attempt_count`               | INTEGER ≥ 0 | |
| `next_attempt_at`             | TIMESTAMPTZ | |
| `last_error_code`             | TEXT  | |
| `related_resource_type`       | TEXT  | e.g. `appointment` (audit-only) |
| `related_resource_id`         | UUID  | e.g. the appointment id |
| `created_at` / `updated_at`   | TIMESTAMPTZ | |

Two CHECK constraints enforce the denylist at the database layer as
defence-in-depth (the Go service also enforces it before INSERT):

- `subject !~ '[0-9]{8,}'`
- `body_template !~ '[0-9]{8,}'`

Indexes:

- `(facility_id, status, created_at DESC)` — admin list, scoped by facility
- `(template_key, status)` — template health checks
- `(next_attempt_at) WHERE status='pending'` — partial index for the future worker

### `notification_delivery_attempts`

| Column                    | Type | Notes |
|---------------------------|------|-------|
| `id`                       | UUID PK | |
| `outbox_id`                | UUID FK ON DELETE CASCADE | |
| `attempt_number`           | INTEGER ≥ 1 | |
| `channel`                  | TEXT | |
| `status`                   | TEXT | |
| `provider_response_excerpt`| TEXT ≤ 256 chars | Capped; sanitised |
| `error_code`               | TEXT | |
| `attempted_at`             | TIMESTAMPTZ | |
| `duration_ms`              | INTEGER | |

UNIQUE `(outbox_id, attempt_number)` — one row per attempt.

---

## 4. Privacy model

The privacy model is enforced at **three** layers.

| Layer | Mechanism |
|-------|-----------|
| Go service | `MaskPhone` / `MaskEmail` strip the bulk of digits / local-part before insert; denylist regex rejects 8+ consecutive digits in subject / body; raw contact is held only in the function-local variable and goes out of scope after the call returns. |
| Database  | `recipient_contact_masked` is the only contact-shape column; CHECK constraints forbid raw-phone-like digit sequences in `subject` and `body_template`. |
| API       | `notification.OutboxRow` has **no** `RecipientContact` field (compile-time test `TestOutboxRowHasNoRawContactField`) and **no** `RecipientContactHash` field. The Go `json.Marshal` projection cannot leak what the struct does not carry. |

Phone masking rules:

| Input                         | Mask                  |
|-------------------------------|-----------------------|
| `+6281234567890`              | `+62••••7890`         |
| `+62-812-3456-7890`           | `+62••••7890`         |
| `081234567890`               | `+62••••7890`         |
| `6281234567890`              | `••••••••7890`        |
| `+12025550100`               | `+1••••0100`          |
| `budi@example.com`           | `b•••@example.com`    |
| `alice@puskesmas.go.id`       | `a•••@puskesmas.go.id` |

Hash canonicalisation: `+6281234567890`, `081234567890`, `6281234567890`,
`+62-812-3456-7890`, `  +62 812 3456 7890  ` all hash to the same SHA-256
(strip leading `0` and country code, keep local digits). This makes the
hash a safe dedup key.

Audit metadata is restricted to:

```
notification_id, facility_id, channel, template_key, status, outcome
```

The audit sanitizer's forbidden-key list (`phone`, `nama`, `alamat`,
`email`) catches every accidental PII leak even if a future contributor
adds an unknown metadata key.

---

## 5. RBAC

Two new permissions are added:

- `notification.read` — view the outbox and masked recipients
- `notification.manage` — retry / cancel outbox rows

Role mapping (additive; no existing role / permission is modified):

| Role            | notification.read | notification.manage |
|-----------------|:-----------------:|:--------------------:|
| `super_admin`   | ✓                 | ✓                    |
| `facility_admin`| ✓                 | ✓                    |
| `operator`      | ✓                 | ✗                    |
| `viewer`        | ✓                 | ✗                    |

Operator / viewer can browse the outbox but cannot retry or cancel. The
`POST /retry` and `POST /cancel` endpoints return `403 Forbidden` for
them, enforced by the existing `RequirePermission` middleware against
the `notification.manage` policy declared in `router/router.go`.

---

## 6. Triggers

The two trigger points are deliberately minimal and fire-and-forget:

| Trigger point                         | Template key                          |
|---------------------------------------|---------------------------------------|
| After `BookAppointment` writes a row  | `appointment.booked.confirmation`     |
| After `CheckIn` writes `status='queued'` | `appointment.checked_in.confirmation` |

The trigger contract:

1. The HTTP response is written **before** the goroutine launches, so a
   slow enqueue can never block the patient.
2. The goroutine uses `context.WithTimeout(context.Background(), 5*time.Second)`
   because `r.Context()` is cancelled when the HTTP response completes.
3. `recover()` is installed; any panic is logged via `slog.Warn` with
   the appointment id (UUID, no PII) and `err = fmt.Sprintf("%v", r)`.
4. Enqueue errors do **not** roll back the appointment or check-in; the
   patient-visible action has already succeeded.
5. The enqueue helper receives the raw patient phone but never logs it.
   The masking / hashing helpers are invoked inside the service; the raw
   value is then eligible for garbage collection.

---

## 7. Dev provider

`notification.DevProvider` is the only `Provider` shipped in this
milestone. Properties:

- **Offline**: no `http.Client`, no `net.Dial`, no DNS resolver, no
  socket. Verified by code review.
- **Deterministic**: the outcome is derived from `fnv32a(uuid) % 100`,
  bucketed into `delivered` (< 75) and `failed` (>= 75). Two calls with
  the same outbox id always produce the same outcome.
- **No secret config required**: there is no API key, no webhook
  secret, no environment variable to set.
- **Outcome extracted for testability**: `DevSimulateOutcome(uuid)` is a
  pure function; the database-writing `Deliver` method delegates to it.

Real vendor providers (Twilio, WhatsApp Cloud API, SMTP, SendGrid,
MessageBird, …) are explicitly **not** shipped. The `Channel` enum
already lists them so a future provider can be wired without a database
migration.

---

## Notification Worker

The notification worker is shipped in this milestone as a separate binary
under `apps/api/cmd/notification-worker/`. It depends on the existing
`internal/notification` package and uses `DevProvider.Deliver` for all
delivery in this PR. There is no real vendor wired in this PR and no
external network call is made.

### Architecture

- Separate CLI binary: `apps/api/cmd/notification-worker/main.go`.
- Shares the existing `internal/notification` package (Service,
  DevProvider, renderer).
- Two execution modes:
  - **Loop mode (daemon)**: poll every `SIGAP_NOTIFICATION_WORKER_INTERVAL_SECONDS` seconds.
  - **ONCE mode (one-shot)**: drain pending rows once and exit.
- Reads `SIGAP_DATABASE_URL` (same as the API); batch size controlled by
  `SIGAP_NOTIFICATION_WORKER_BATCH_SIZE`.

### Claim strategy

- Uses `SELECT ... FOR UPDATE SKIP LOCKED` against
  `notification_outbox WHERE status = 'pending' AND next_attempt_at <= now()`,
  ordered by `next_attempt_at ASC`.
- Sets `status = 'processing'` and increments `attempt_count` inside the
  same transaction so concurrent workers cannot double-deliver the same
  row.
- Commits the claim; only then calls the provider.

### Retry / backoff

- `MaxAttempts = 3` (constant, not configurable per template in this PR).
- Backoff schedule on transient failure: `1 minute → 5 minutes → 15 minutes`,
  applied by updating `next_attempt_at` via `scheduleRetry()`.
- After the third failure, the row is marked `failed` (terminal).

### Status state machine

`pending → processing → delivered | failed` (terminal). `cancelled` rows
are skipped (already terminal). `processing` rows are not picked up by
the next poll until `next_attempt_at` elapses again.

### Delivery

- The worker calls `DevProvider.Deliver(...)` only. No `http.Client`,
  `net.Dial`, DNS resolver, or external socket is opened.
- The dev provider's deterministic outcome (`fnv32a(uuid) % 100`, bucketed
  `delivered` < 75 / `failed` >= 75) is recorded in
  `notification_delivery_attempts` and reflected on the outbox row.

### Privacy guards

- Logs include only `notification_id`, `template_key`, `status`,
  `attempt_number`, and the rendered outcome — never the recipient
  contact, the dedup hash, or the rendered message body.
- The recipient contact exists only inside `DevProvider.Deliver` as a
  function-local variable (already masked / hashed upstream by the
  service that enqueued the row). The worker never sees the raw
  contact.

### Execution

- Manual: `cd apps/api && go run ./cmd/notification-worker`.
- No docker-compose service for the worker in this PR.
- Safe to run multiple instances locally; `FOR UPDATE SKIP LOCKED`
  prevents duplicate work. A production deployment should still use a
  single instance per environment until the rate limiter / dedup model
  is extended.

---

## Template Renderer

The template renderer (`apps/api/internal/notification/renderer.go`)
performs `{placeholder}` substitution for the body_template. It is
**not** `text/template`; it is a hand-rolled, deliberately boring
substitution engine with a closed allow-list and multiple pre-flight /
post-flight guards.

### Signature

```go
func RenderTemplate(tpl string, vars map[string]string) (string, error)
```

### Closed allow-list (8 variables)

| Variable           | Used by                                    |
|--------------------|--------------------------------------------|
| `appointment_code` | seeded confirmation templates              |
| `appointment_time` | seeded confirmation templates              |
| `checkin_code`     | seeded check-in confirmation template      |
| `checked_in_at`    | seeded check-in confirmation template      |
| `facility_name`    | both seeded templates                      |
| `patient_name`     | both seeded templates                      |
| `queue_number`     | seeded check-in confirmation template      |
| `template_key`     | both seeded templates (audit-friendly)     |

The allow-list matches the `{placeholder}` occurrences used by the demo
seed templates. Adding a new variable requires editing the allow-list
intentionally; unknown keys are rejected before any substitution.

### Pre-flight checks

1. **Allow-list check**: every key in `vars` must be in the 8-element
   allow-list. A key like `raw_phone` is rejected with
   `ErrUnsafeVariable` before any substitution occurs.
2. **Placeholder completeness**: every `{placeholder}` occurrence in
   `tpl` must have a matching key in `vars`. Otherwise
   `ErrMissingPlaceholder` is returned.
3. **Empty template**: an empty `tpl` returns `ErrEmptyTemplate`.

### Substitution

- Regex `{[a-z_]{1,64}}` (placeholder names are lowercase + underscore,
  1..64 chars).
- Pure string replacement; no expression evaluation, no logic, no I/O.

### Post-check

- The rendered output is scanned for any run of **10 or more
  consecutive digits**. If found, the renderer returns
  `ErrRenderedOutputContainsRawDigits`. This is a phone-number guard:
  even if an upstream caller accidentally passed an unmasked contact
  via a variable, the post-check blocks the rendered message from
  ever being delivered.

### Domain errors

```
ErrMissingPlaceholder                  — tpl references an unknown placeholder
ErrUnsafeVariable                      — vars contains a key not in the allow-list
ErrRenderedOutputContainsRawDigits     — rendered body has 10+ consecutive digits
ErrEmptyTemplate                       — tpl is empty
```

### Why hand-rolled and not `text/template`

`text/template` was explicitly avoided to keep the surface area
minimal and to prevent accidental logic injection (e.g., a
`{{.Env}}`-style construct slipping through). The renderer is a pure
function with no globals, no I/O, and no file access.

### Privacy and security model (renderer)

| Guard                              | Where applied   | What it blocks                                  |
|------------------------------------|-----------------|-------------------------------------------------|
| Allow-list of 8 variable names     | pre-flight      | Unknown keys (e.g. `raw_phone`, `recipient_*`)  |
| Placeholder completeness check     | pre-flight      | Half-substituted output leaking template source |
| Empty template rejection           | pre-flight      | Empty / blank messages                          |
| 10+ consecutive digits post-check  | post-substitution | Raw phone number, raw NIK, raw numeric PII    |
| Pure function, no I/O              | runtime         | Side channels, file system, network             |
| No `text/template` evaluation      | design choice   | Expression injection, control flow injection    |

---

## 8. Inspecting the outbox locally

After applying the migration and the seed:

```bash
# 1. Apply the new migration.
psql "$DATABASE_URL" -f packages/db/migrations/0006_notifications.sql

# 2. Re-run the RBAC seed (additive — safe to re-run).
psql "$DATABASE_URL" -f packages/db/seed/rbac.sql

# 3. Book an appointment (the public endpoint is the simplest way to
#    populate a row).
curl -X POST http://localhost:8080/api/v1/appointments \
  -H 'Content-Type: application/json' \
  -d '{"facility_id":"<UUID>","service_unit_id":"00000000-0000-0000-0000-00000000d001","patient_display_name":"Test","patient_phone":"+6281234567890","appointment_time":"2026-12-31T09:00:00Z"}'

# 4. Inspect the outbox.
psql "$DATABASE_URL" -c "SELECT id, channel, template_key, recipient_contact_masked, status, attempt_count FROM notification_outbox ORDER BY created_at DESC LIMIT 20;"

# 5. Inspect delivery attempts.
psql "$DATABASE_URL" -c "SELECT outbox_id, attempt_number, status, provider_response_excerpt, duration_ms FROM notification_delivery_attempts ORDER BY attempted_at DESC LIMIT 20;"

# 6. Use the admin web UI at http://localhost:5173/admin/notifications.
```

You should see rows whose `recipient_contact_masked` is `+62••••7890`,
**never** the raw `+6281234567890`.

---

## 9. Future vendor integration (intentionally deferred)

The seam is already in place. The notification worker (see **Notification
Worker** above) closes the execution gap in this PR: it drains pending
rows and invokes the provider. What is still deferred is the **provider
integration** itself — no real vendor is wired. A future phase adds a
real provider by:

1. Adding `apps/api/internal/notification/<vendor>.go` with a struct that
   implements `Provider` and is constructor-injected in
   `cmd/server/main.go` and `cmd/notification-worker/main.go`.
2. Adding the vendor's API key as a config value (never committed;
   read from env or secret manager).
3. Toggling the worker's `channel` selection so the appropriate provider
   is invoked per row. The partial index on `next_attempt_at` already
   exists for this.
4. Wiring per-template `max_attempts` / `channel` overrides (today the
   constant `MaxAttempts = 3` is hard-coded for the DevProvider).

None of these changes the schema, the API, or the privacy model.

---

## 10. Limitations and non-goals

- **No real vendor**: messages never leave the host. The worker
  delivers only via `DevProvider` in this PR.
- **Worker ships in this PR, but is not wired to docker-compose**: it
  must be started manually with
  `cd apps/api && go run ./cmd/notification-worker`. Production deploys
  should add their own orchestrator (systemd, k8s, supervisord, …).
  See `docs/DEV_SETUP.md` for the manual run commands and env vars.
- **Template renderer shipped, allow-list closed at 8 variables**: the
  renderer is hand-rolled (no `text/template`), rejects unknown
  variable keys, and post-checks the output for raw digit runs.
  Adding a new placeholder requires editing the allow-list in
  `renderer.go` deliberately; the seeded demo templates cover the
  current allow-list.
- **No multi-channel orchestration**: a row has exactly one channel.
- **No email/phone normalisation library**: we use a hand-rolled
  `MaskPhone` / `MaskEmail` / `HashContact`. A future phase may swap in
  `google/libphonenumber` for full E.164 normalisation; the interface
  does not change.
- **No rate-limit on enqueue**: the service has no upper bound on
  inserts-per-second. Real production needs a per-recipient per-day
  quota enforced by the worker before `Deliver`.
- **No per-template `max_attempts` override**: the constant
  `MaxAttempts = 3` is hard-coded in this PR. Per-template tuning is
  deferred until a real provider is wired.
## Ops Runbook

This section is the operator-facing guide to the `notification-worker`
binary: how to run it, how to dry-run it, how to read its output, and
how to fix the common failure modes. All commands assume
`SIGAP_DATABASE_URL` is exported in the shell.

### How to run the worker once (real processing)

```bash
SIGAP_NOTIFICATION_WORKER_ENABLED=true \
SIGAP_NOTIFICATION_WORKER_ONCE=true \
go run ./cmd/notification-worker
```

The worker claims up to `SIGAP_NOTIFICATION_WORKER_BATCH_SIZE` rows
(default 25), processes each one through `DevProvider`, applies the
backoff schedule on failures, and exits.

Sample real-run output:

```
INFO worker run complete dry_run=false inspected_pending=12 claimed=12 delivered=8 failed=3 retried=1 skipped=0
```

### How to run a dry-run (preview — zero database mutation)

```bash
SIGAP_NOTIFICATION_WORKER_DRY_RUN=true \
SIGAP_NOTIFICATION_WORKER_ENABLED=true \
go run ./cmd/notification-worker
```

Preview only. The worker performs a plain read-only SELECT of the
eligibility set, runs `DevSimulateOutcome` (a pure deterministic
function) per row to predict the outcome, and reports counts. It
does NOT call `claim()`, `processRow()`, `scheduleRetry()`, or
`provider.Deliver()`. It does NOT use `pool.Exec`, `Begin`, or
`BeginTx`. It does NOT write to `notification_outbox` or
`notification_delivery_attempts`. Safe to run on production data.

Sample dry-run output:

```
INFO worker run complete dry_run=true inspected_pending=15 claimed=0 delivered=0 failed=0 retried=0 skipped=0
```

The `claimed=0` and all-zero delivery counters are the visual
confirmation that no mutation occurred. Only `inspected_pending` is
populated; the rest of the counters are pinned to zero by the
preview-mode invariant.

### How to troubleshoot pending / failed notifications

- **Status summary** (read-only): `GET /api/v1/admin/notifications/summary`
- **Filtered list**: `GET /api/v1/admin/notifications?status=failed&channel=dev`
- **Retry a single row**: `POST /api/v1/admin/notifications/<id>/retry`
  (requires `notification.manage` scope)
- **Cancel a stuck row**: `POST /api/v1/admin/notifications/<id>/cancel`
- **Worker logs**: look for the per-row `worker: delivered`,
  `worker: retry scheduled`, or `worker: terminal failure` slog lines.

### Limitations of DevProvider-only mode

- All outcomes are simulated via `DevSimulateOutcome` (fnv32a-based
  deterministic split: ~75% delivered, ~25% failed).
- `DevProvider.Deliver()` does not validate recipient format.
- The worker's `Delivered` count is always artificial until a real
  Provider is wired in.
- No SMS, WhatsApp, or email can actually be sent until a real
  Provider implements the `notification.Provider` interface.
