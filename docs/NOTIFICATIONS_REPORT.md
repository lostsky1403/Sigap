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
                                          (future worker / cron —
                                           intentionally NOT shipped)
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

The future worker (cron / queue consumer) that drains pending rows is
**not** shipped in this milestone. The dev provider is invoked
synchronously by future code; in this milestone the dev provider exists
only as a deterministic helper that is exercised by tests.

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

The seam is already in place. A future phase adds a real provider by:

1. Adding `apps/api/internal/notification/<vendor>.go` with a struct that
   implements `Provider` and is constructor-injected in `cmd/server/main.go`.
2. Adding the vendor's API key as a config value (never committed;
   read from env or secret manager).
3. Implementing a worker (cron / queue consumer) that drains
   `notification_outbox WHERE status='pending'` and calls
   `provider.Deliver(...)`. The partial index on `next_attempt_at`
   already exists for this.
4. Wiring exponential-backoff via `next_attempt_at` and a configurable
   `max_attempts` per template.

None of these changes the schema, the API, or the privacy model.

---

## 10. Limitations and non-goals

- **No real vendor**: messages never leave the host.
- **No outbox worker**: rows sit in `pending` until something else
  invokes `DevProvider.Deliver`.
- **No multi-channel orchestration**: a row has exactly one channel.
- **No email/phone normalisation library**: we use a hand-rolled
  `MaskPhone` / `MaskEmail` / `HashContact`. A future phase may swap in
  `google/libphonenumber` for full E.164 normalisation; the interface
  does not change.
- **No template rendering**: `body_template` is stored verbatim. A
  future phase adds a small `{placeholder}` substitution engine; the
  `{appointment_id}` / `{checkin_code}` placeholders in the demo
  templates are illustrative, not yet substituted.
- **No rate-limit on enqueue**: the service has no upper bound on
  inserts-per-second. Real production needs a per-recipient per-day
  quota enforced by the worker before `Deliver`.