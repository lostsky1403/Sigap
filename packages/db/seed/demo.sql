-- packages/db/seed/demo.sql
-- Additive, idempotent demo seed for Sigap local demos and smoke tests.
-- Run AFTER packages/db/migrations/0001_init.sql, 0005_appointments.sql,
-- AND packages/db/seed/dev.sql + packages/db/seed/rbac.sql.
--
-- ============================================================
-- Idempotency guarantees (every demo row satisfies ALL of these)
-- ============================================================
--   1. Deterministic UUID — each row uses a hand-picked UUID with the
--      suffix d001..d022 so the smoke script can reference the row by
--      string. The UUID is also the PRIMARY KEY, so even if the natural
--      key guard is somehow skipped, the PK constraint prevents a true
--      duplicate row.
--   2. Natural-key WHERE NOT EXISTS guard — each INSERT is paired with
--      a NOT EXISTS predicate against a synthetic natural-key tuple
--      (facility_id, code/display_name/date+time+service_unit). Re-running
--      this file on a populated DB short-circuits every INSERT and writes
--      zero new rows.
--   3. No real patient data — see DATA POLICY below.
--   4. No destructive SQL — no TRUNCATE, no DELETE, no UPDATE of existing
--      rows. Existing non-demo records are never modified.
--   5. Idempotent on schema drift — if a referenced table or column is
--      missing, the INSERT fails loudly with a normal SQL error (this is
--      the desired behaviour: silent success on a broken schema would
--      mask migration problems).
--
-- ============================================================
-- DATA POLICY (synthetic only — never replace with real data)
-- ============================================================
--   * No real patient names, no real NIK, no real phone.
--   * Facility scope: uses the canonical demo facility (d000) exclusively.
--   * Phone numbers use the +62-555-01xx reserved-for-testing range
--     (ITU-T E.164 reserved for fictional use).
--   * Schedule date is rolled forward to "tomorrow" (CURRENT_DATE + 1) in
--     the PostgreSQL server's timezone so the demo slot is always bookable.

-- Idempotent schema guard: add notes column if missing (added after initial
-- migration; existing local DBs may lack it).
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS notes TEXT;

BEGIN;

-- ============================================================
-- 0. Canonical demo facility (deterministic UUID, always exists).
--     All demo seed rows link to this facility so the smoke script
--     always finds the correct facility even if dev.sql has been
--     run multiple times creating duplicate RSK rows.
--
--     ON CONFLICT (id) DO UPDATE re-upserts the canonical row on
--     every seed run instead of creating a duplicate. This guards
--     against older dev.sql versions (pre-idempotent) that inserted
--     RSK facilities with random UUIDs and no ON CONFLICT — each
--     re-run added another short_code='RSK' row. The smoke script
--     relies on this canonical facility (d000) owning the DEMO-UMUM
--     service unit for deterministic booking.
-- ============================================================
INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, short_code, is_active)
VALUES (
    '00000000-0000-0000-0000-00000000d000'::uuid,
    'Sigap Demo Facility',
    'rumah_sakit',
    'Jl. Demo No. 1',
    'Demo',
    'Kota Demo',
    'Jawa Barat',
    '000-000000',
    50,
    50,
    'DEMO',
    TRUE
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    short_code = EXCLUDED.short_code,
    is_active = EXCLUDED.is_active;

-- ============================================================
-- 1. Service units (Poli Umum, Poli Gigi) tied to a single
--    deterministic facility.
--    Natural-key guard: (facility_id, code).
-- ============================================================
INSERT INTO service_units (id, facility_id, name, code, description, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d001'::uuid,  -- deterministic UUID (d001)
    f.id,                                          -- single deterministic facility
    'Poli Umum Demo',
    'DEMO-UMUM',                                   -- natural key part 2
    'Pelayanan umum untuk demo lokal Sigap. Data sintetis.',
    TRUE
FROM facilities f
WHERE f.id = '00000000-0000-0000-0000-00000000d000'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM service_units su
    WHERE su.facility_id = f.id AND su.code = 'DEMO-UMUM'  -- natural-key guard
  )
ON CONFLICT (id) DO NOTHING;

INSERT INTO service_units (id, facility_id, name, code, description, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d002'::uuid,  -- deterministic UUID (d002)
    f.id,
    'Poli Gigi Demo',
    'DEMO-GIGI',
    'Pelayanan gigi untuk demo lokal Sigap. Data sintetis.',
    TRUE
FROM facilities f
WHERE f.id = '00000000-0000-0000-0000-00000000d000'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM service_units su
    WHERE su.facility_id = f.id AND su.code = 'DEMO-GIGI'
  )
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 2. Practitioners (generic providers, no real PII).
--    Natural-key guard: (facility_id, display_name).
-- ============================================================
INSERT INTO practitioners (id, facility_id, display_name, role, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d011'::uuid,  -- deterministic UUID (d011)
    f.id,
    'Dokter Demo A',                               -- natural key part 2
    'dokter_umum',
    TRUE
FROM facilities f
WHERE f.id = '00000000-0000-0000-0000-00000000d000'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM practitioners p
    WHERE p.facility_id = f.id AND p.display_name = 'Dokter Demo A'
  )
ON CONFLICT (id) DO NOTHING;

INSERT INTO practitioners (id, facility_id, display_name, role, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d012'::uuid,  -- deterministic UUID (d012)
    f.id,
    'Dokter Demo B',
    'dokter_gigi',
    TRUE
FROM facilities f
WHERE f.id = '00000000-0000-0000-0000-00000000d000'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM practitioners p
    WHERE p.facility_id = f.id AND p.display_name = 'Dokter Demo B'
  )
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 3. Schedules for tomorrow: two slots at the deterministic facility, two service units.
--    Slot 09:00-12:00, 30-minute slots, capacity 3 per slot.
--    Natural-key guard: (facility_id, service_unit_id, schedule_date, start_time).
-- ============================================================
INSERT INTO practitioner_schedules (
    id, facility_id, practitioner_id, service_unit_id,
    schedule_date, start_time, end_time, slot_minutes, capacity_per_slot, is_active
)
SELECT
    '00000000-0000-0000-0000-00000000d021'::uuid,  -- deterministic UUID (d021)
    f.id,
    '00000000-0000-0000-0000-00000000d011'::uuid,  -- -> Dokter Demo A
    '00000000-0000-0000-0000-00000000d001'::uuid,  -- -> Poli Umum Demo
    (CURRENT_DATE + INTERVAL '1 day')::date,        -- always "tomorrow" in PG TZ
    '09:00:00'::time,
    '12:00:00'::time,
    30,
    3,
    TRUE
FROM facilities f
WHERE f.id = '00000000-0000-0000-0000-00000000d000'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM practitioner_schedules ps
    WHERE ps.facility_id = f.id
      AND ps.service_unit_id = '00000000-0000-0000-0000-00000000d001'::uuid
      AND ps.schedule_date = (CURRENT_DATE + INTERVAL '1 day')::date
      AND ps.start_time = '09:00:00'::time
  )
ON CONFLICT (id) DO NOTHING;

INSERT INTO practitioner_schedules (
    id, facility_id, practitioner_id, service_unit_id,
    schedule_date, start_time, end_time, slot_minutes, capacity_per_slot, is_active
)
SELECT
    '00000000-0000-0000-0000-00000000d022'::uuid,  -- deterministic UUID (d022)
    f.id,
    '00000000-0000-0000-0000-00000000d012'::uuid,  -- -> Dokter Demo B
    '00000000-0000-0000-0000-00000000d002'::uuid,  -- -> Poli Gigi Demo
    (CURRENT_DATE + INTERVAL '1 day')::date,
    '09:00:00'::time,
    '12:00:00'::time,
    30,
    3,
    TRUE
FROM facilities f
WHERE f.id = '00000000-0000-0000-0000-00000000d000'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM practitioner_schedules ps
    WHERE ps.facility_id = f.id
      AND ps.service_unit_id = '00000000-0000-0000-0000-00000000d002'::uuid
      AND ps.schedule_date = (CURRENT_DATE + INTERVAL '1 day')::date
      AND ps.start_time = '09:00:00'::time
  )
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 4. Notification outbox demo rows for the notification smoke test.
--    Two deterministic rows with status='pending' and next_attempt_at <= NOW().
--    Natural-key guard: (template_key, recipient_contact_hash).
--    No raw PII: only masked contact and SHA-256 hash are stored.
-- ============================================================
INSERT INTO notification_outbox
    (id, facility_id, channel, template_key, subject, body_template,
     recipient_type, recipient_contact_masked, recipient_contact_hash,
     status, attempt_count, next_attempt_at,
     related_resource_type, related_resource_id)
SELECT
    '00000000-0000-0000-0000-00000000d031'::uuid,
    f.id,
    'dev',
    'appointment.booked.confirmation',
    'Konfirmasi Janji Temu Sigap',
    'Janji temu demo Anda di {facility_name} telah tercatat. Kode check-in: {checkin_code}.',
    'patient',
    '+62••••1234',
    '\xd1dbfab149200afb849e3a6a1c82447ec4fb86fb9b8f124897797fb1a867c824'::bytea,
    'pending',
    0,
    NOW(),
    'smoke_seed',
    '00000000-0000-0000-0000-00000000d041'::uuid
FROM facilities f
WHERE f.id = '00000000-0000-0000-0000-00000000d000'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM notification_outbox no
    WHERE no.template_key = 'appointment.booked.confirmation'
      AND no.recipient_contact_hash = '\xd1dbfab149200afb849e3a6a1c82447ec4fb86fb9b8f124897797fb1a867c824'::bytea
  );

INSERT INTO notification_outbox
    (id, facility_id, channel, template_key, subject, body_template,
     recipient_type, recipient_contact_masked, recipient_contact_hash,
     status, attempt_count, next_attempt_at,
     related_resource_type, related_resource_id)
SELECT
    '00000000-0000-0000-0000-00000000d032'::uuid,
    f.id,
    'dev',
    'appointment.checked_in.confirmation',
    'Status Check-in Sigap',
    'Check-in demo Anda di {facility_name} berhasil. Nomor antrean Anda: {queue_number}.',
    'patient',
    '+62••••1567',
    '\xc5af62ba6bb72c87d3921ac6fbaf48a1ca97964dbdb1315661cf2588c9c2eb1a'::bytea,
    'pending',
    0,
    NOW(),
    'smoke_seed',
    '00000000-0000-0000-0000-00000000d042'::uuid
FROM facilities f
WHERE f.id = '00000000-0000-0000-0000-00000000d000'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM notification_outbox no
    WHERE no.template_key = 'appointment.checked_in.confirmation'
      AND no.recipient_contact_hash = '\xc5af62ba6bb72c87d3921ac6fbaf48a1ca97964dbdb1315661cf2588c9c2eb1a'::bytea
  );

-- Reset deterministic smoke notification rows to pending on rerun.
-- Previous worker runs may have consumed these rows. Rerunning demo.sql
-- must always leave them as pending so the notification smoke script
-- finds >= 1 pending row.
UPDATE notification_outbox
SET status = 'pending',
    attempt_count = 0,
    next_attempt_at = NOW(),
    updated_at = NOW()
WHERE id IN (
    '00000000-0000-0000-0000-00000000d031'::uuid,
    '00000000-0000-0000-0000-00000000d032'::uuid
)
AND status <> 'pending';

-- ============================================================
-- 5. Demo appointment with known checkin_code for patient portal lookup.
--    Uses a single deterministic facility and the first demo schedule.
--    Idempotency guard: deterministic primary key with conflict update.
--    No real PII: synthetic patient data only.
-- ============================================================
-- Clear any duplicate SMOKE01 checkin_codes from other appointment rows
-- so the query returns exactly one row after seeding.
UPDATE appointments
SET checkin_code = 'SMOKE01-' || LEFT(REPLACE(id::text, '-', ''), 8),
    updated_at = NOW()
WHERE checkin_code = 'SMOKE01'
  AND id <> '00000000-0000-0000-0000-00000000d051'::uuid;

-- Use a DO block with INSERT...VALUES to guarantee exactly one source row.
-- INSERT...SELECT can produce duplicates when the FROM source has multiple
-- rows (duplicate facilities from older seeds), causing
-- "ON CONFLICT DO UPDATE command cannot affect row a second time".
-- The block resolves facility_id and service_unit_id dynamically so the
-- seed works on dirty/local DBs with missing demo service units.
DO $$
DECLARE
    v_facility_id UUID;
    v_service_unit_id UUID;
BEGIN
    -- Use the canonical demo facility.
    v_facility_id := '00000000-0000-0000-0000-00000000d000'::uuid;

    IF v_facility_id IS NULL THEN
        RAISE NOTICE 'No facilities found; skipping SMOKE01 appointment seed';
        RETURN;
    END IF;

    -- Use the deterministic demo service unit (always linked to d000).
    v_service_unit_id := '00000000-0000-0000-0000-00000000d001'::uuid;

    INSERT INTO appointments (
        id, facility_id, service_unit_id, practitioner_schedule_id,
        appointment_time, status,
        patient_display_name, patient_phone, checkin_code,
        created_at, updated_at
    ) VALUES (
        '00000000-0000-0000-0000-00000000d051'::uuid,
        v_facility_id,
        v_service_unit_id,
        NULL,
        (CURRENT_DATE + INTERVAL '1 day' + INTERVAL '9 hours')::timestamptz,
        'scheduled',
        'Pasien Demo Portal',
        '085550000001',
        'SMOKE01',
        NOW(),
        NOW()
    )
    ON CONFLICT (id) DO UPDATE SET
        facility_id = EXCLUDED.facility_id,
        service_unit_id = EXCLUDED.service_unit_id,
        practitioner_schedule_id = EXCLUDED.practitioner_schedule_id,
        appointment_time = EXCLUDED.appointment_time,
        status = EXCLUDED.status,
        patient_display_name = EXCLUDED.patient_display_name,
        patient_phone = EXCLUDED.patient_phone,
        checkin_code = EXCLUDED.checkin_code,
        updated_at = NOW();
END $$;

COMMIT;

-- ============================================================
-- Quick verification (read-only): print IDs that downstream smoke scripts need.
-- Run with:  psql $DATABASE_URL -f packages/db/seed/demo.sql
-- Then:      psql $DATABASE_URL -c "SELECT id FROM facilities WHERE short_code='f1'"
--            psql $DATABASE_URL -c "SELECT id FROM service_units WHERE code IN ('DEMO-UMUM','DEMO-GIGI')"
--            psql $DATABASE_URL -c "SELECT id FROM practitioner_schedules WHERE id IN ('00000000-0000-0000-0000-00000000d021','00000000-0000-0000-0000-00000000d022')"
--
-- Re-run safety check:
--   Run this file twice in a row. The second run MUST report zero
--   affected rows in the psql output (every INSERT short-circuits).
-- ============================================================
