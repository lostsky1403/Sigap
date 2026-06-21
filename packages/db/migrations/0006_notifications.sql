-- 0006_notifications.sql
-- Notification outbox foundation for Sigap.
-- Forward-only: creates new tables and types only. Does NOT touch
-- existing schema, does NOT backfill, does NOT modify any existing row.
--
-- Run AFTER packages/db/migrations/0005_appointments.sql and the existing
-- RBAC seed (rbac.sql). Apply with:
--   psql $DATABASE_URL -f packages/db/migrations/0006_notifications.sql
--
-- DATA POLICY (privacy-first)
--   * No raw patient phone/email is ever stored in the outbox.
--   * `recipient_contact_masked` is the only contact-shape field stored.
--   * `recipient_contact_hash` stores SHA-256 of the NORMALISED contact for
--     dedup; the raw contact is consumed transiently and discarded.
--   * No clinical data, no diagnosis, no free-text PHI in templates or bodies.
--   * `subject` and `body_template` MUST NOT contain digits-only sequences
--     of length >= 8 (catches accidental phone leaks). Enforced at insert
--     time by a CHECK constraint.
--   * Delivery attempts store at most 256 chars of provider response, with
--     best-effort redaction (the dev provider never returns raw contact).
--
-- DEFERRED (intentionally NOT in this migration)
--   * Real SMS / WhatsApp / email provider schemas and templates.
--   * Outbox worker / cron / exponential backoff scheduling.
--   * Public opt-in / opt-out endpoints.
--   * Webhook receivers from external vendors.

BEGIN;

-- ---------------------------------------------------------------------------
-- Channel / status / recipient_type enums.
-- Using TEXT + CHECK rather than CREATE TYPE so the migration is purely
-- additive (no CREATE TYPE conflict with future migrations).
-- ---------------------------------------------------------------------------

-- notification_templates: small, read-mostly catalog. Two demo rows are
-- seeded idempotently at the bottom of this file.
CREATE TABLE IF NOT EXISTS notification_templates (
    template_key      TEXT PRIMARY KEY,
    channel           TEXT NOT NULL,
    subject_template  TEXT NOT NULL,
    body_template     TEXT NOT NULL,
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_templates_channel_chk
        CHECK (channel IN ('dev','sms','whatsapp','email')),
    CONSTRAINT notification_templates_subject_chk
        CHECK (octet_length(subject_template) BETWEEN 1 AND 200),
    CONSTRAINT notification_templates_body_chk
        CHECK (octet_length(body_template) BETWEEN 1 AND 4000)
);

-- notification_outbox: one row per notification that was enqueued. The raw
-- contact is NEVER stored here. The masked contact is for display; the
-- hash is for dedup.
CREATE TABLE IF NOT EXISTS notification_outbox (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    facility_id                 UUID REFERENCES facilities(id) ON DELETE SET NULL,
    channel                     TEXT NOT NULL,
    template_key                TEXT NOT NULL,
    subject                     TEXT NOT NULL,
    body_template               TEXT NOT NULL,
    recipient_type              TEXT NOT NULL,
    recipient_contact_masked    TEXT NOT NULL,
    recipient_contact_hash      BYTEA NOT NULL,
    status                      TEXT NOT NULL DEFAULT 'pending',
    attempt_count               INTEGER NOT NULL DEFAULT 0,
    next_attempt_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error_code             TEXT,
    related_resource_type       TEXT,
    related_resource_id         UUID,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_outbox_channel_chk
        CHECK (channel IN ('dev','sms','whatsapp','email')),
    CONSTRAINT notification_outbox_recipient_type_chk
        CHECK (recipient_type IN ('patient','staff','facility_admin')),
    CONSTRAINT notification_outbox_status_chk
        CHECK (status IN ('pending','processing','delivered','failed','cancelled')),
    CONSTRAINT notification_outbox_attempt_count_chk
        CHECK (attempt_count >= 0),
    CONSTRAINT notification_outbox_hash_len_chk
        CHECK (octet_length(recipient_contact_hash) = 32),
    CONSTRAINT notification_outbox_subject_chk
        CHECK (octet_length(subject) BETWEEN 1 AND 200),
    CONSTRAINT notification_outbox_body_chk
        CHECK (octet_length(body_template) BETWEEN 1 AND 4000),
    CONSTRAINT notification_outbox_masked_chk
        CHECK (octet_length(recipient_contact_masked) BETWEEN 3 AND 200),
    CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk
        CHECK (subject !~ '[0-9]{8,}'),
    CONSTRAINT notification_outbox_no_raw_phone_in_body_chk
        CHECK (body_template !~ '[0-9]{8,}')
);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_facility_status_created
    ON notification_outbox (facility_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_outbox_template_status
    ON notification_outbox (template_key, status);
CREATE INDEX IF NOT EXISTS idx_notification_outbox_pending_due
    ON notification_outbox (next_attempt_at)
    WHERE status = 'pending';

-- notification_delivery_attempts: append-only audit per delivery attempt.
-- One row per (outbox_id, attempt_number). On outbox deletion the attempts
-- are removed too (CASCADE).
CREATE TABLE IF NOT EXISTS notification_delivery_attempts (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    outbox_id                   UUID NOT NULL REFERENCES notification_outbox(id) ON DELETE CASCADE,
    attempt_number              INTEGER NOT NULL,
    channel                     TEXT NOT NULL,
    status                      TEXT NOT NULL,
    provider_response_excerpt   TEXT,
    error_code                  TEXT,
    attempted_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duration_ms                 INTEGER,
    CONSTRAINT notification_delivery_attempts_attempt_chk
        CHECK (attempt_number >= 1),
    CONSTRAINT notification_delivery_attempts_channel_chk
        CHECK (channel IN ('dev','sms','whatsapp','email')),
    CONSTRAINT notification_delivery_attempts_status_chk
        CHECK (status IN ('pending','processing','delivered','failed','cancelled')),
    CONSTRAINT notification_delivery_attempts_excerpt_len_chk
        CHECK (provider_response_excerpt IS NULL OR octet_length(provider_response_excerpt) <= 256),
    CONSTRAINT notification_delivery_attempts_unique_per_outbox
        UNIQUE (outbox_id, attempt_number)
);

CREATE INDEX IF NOT EXISTS idx_notification_delivery_attempts_outbox
    ON notification_delivery_attempts (outbox_id, attempted_at DESC);

-- ---------------------------------------------------------------------------
-- Idempotent seed of the two demo templates. We use WHERE NOT EXISTS so the
-- migration is safe to re-run on a populated database.
-- ---------------------------------------------------------------------------
INSERT INTO notification_templates
    (template_key, channel, subject_template, body_template)
SELECT 'appointment.booked.confirmation', 'dev',
       'Konfirmasi Janji Temu Sigap',
       'Janji temu Anda di fasilitas {facility_name} pada {appointment_time} telah tercatat. Kode check-in: {checkin_code}.'
WHERE NOT EXISTS (SELECT 1 FROM notification_templates WHERE template_key = 'appointment.booked.confirmation');

INSERT INTO notification_templates
    (template_key, channel, subject_template, body_template)
SELECT 'appointment.checked_in.confirmation', 'dev',
       'Status Check-in Sigap',
       'Check-in Anda di {facility_name} pada {checked_in_at} berhasil. Nomor antrean Anda: {queue_number}.'
WHERE NOT EXISTS (SELECT 1 FROM notification_templates WHERE template_key = 'appointment.checked_in.confirmation');

COMMIT;
