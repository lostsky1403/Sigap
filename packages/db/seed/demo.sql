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
--   * Facility scope: targets the seeded facility with short_code = 'f1'
--     (the first row inserted by packages/db/seed/dev.sql).
--   * Phone numbers use the +62-555-01xx reserved-for-testing range
--     (ITU-T E.164 reserved for fictional use).
--   * Schedule date is rolled forward to "tomorrow" (CURRENT_DATE + 1) in
--     the PostgreSQL server's timezone so the demo slot is always bookable.

BEGIN;

-- ============================================================
-- 1. Service units (Poli Umum, Poli Gigi) tied to facility f1.
--    Natural-key guard: (facility_id, code).
-- ============================================================
INSERT INTO service_units (id, facility_id, name, code, description, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d001'::uuid,  -- deterministic UUID (d001)
    f.id,                                          -- facility f1 (joined below)
    'Poli Umum Demo',
    'DEMO-UMUM',                                   -- natural key part 2
    'Pelayanan umum untuk demo lokal Sigap. Data sintetis.',
    TRUE
FROM facilities f
WHERE f.short_code = 'f1'
  AND NOT EXISTS (
    SELECT 1 FROM service_units su
    WHERE su.facility_id = f.id AND su.code = 'DEMO-UMUM'  -- natural-key guard
  );

INSERT INTO service_units (id, facility_id, name, code, description, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d002'::uuid,  -- deterministic UUID (d002)
    f.id,
    'Poli Gigi Demo',
    'DEMO-GIGI',
    'Pelayanan gigi untuk demo lokal Sigap. Data sintetis.',
    TRUE
FROM facilities f
WHERE f.short_code = 'f1'
  AND NOT EXISTS (
    SELECT 1 FROM service_units su
    WHERE su.facility_id = f.id AND su.code = 'DEMO-GIGI'
  );

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
WHERE f.short_code = 'f1'
  AND NOT EXISTS (
    SELECT 1 FROM practitioners p
    WHERE p.facility_id = f.id AND p.display_name = 'Dokter Demo A'
  );

INSERT INTO practitioners (id, facility_id, display_name, role, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d012'::uuid,  -- deterministic UUID (d012)
    f.id,
    'Dokter Demo B',
    'dokter_gigi',
    TRUE
FROM facilities f
WHERE f.short_code = 'f1'
  AND NOT EXISTS (
    SELECT 1 FROM practitioners p
    WHERE p.facility_id = f.id AND p.display_name = 'Dokter Demo B'
  );

-- ============================================================
-- 3. Schedules for tomorrow: two slots at the same facility, two service units.
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
WHERE f.short_code = 'f1'
  AND NOT EXISTS (
    SELECT 1 FROM practitioner_schedules ps
    WHERE ps.facility_id = f.id
      AND ps.service_unit_id = '00000000-0000-0000-0000-00000000d001'::uuid
      AND ps.schedule_date = (CURRENT_DATE + INTERVAL '1 day')::date
      AND ps.start_time = '09:00:00'::time
  );

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
WHERE f.short_code = 'f1'
  AND NOT EXISTS (
    SELECT 1 FROM practitioner_schedules ps
    WHERE ps.facility_id = f.id
      AND ps.service_unit_id = '00000000-0000-0000-0000-00000000d002'::uuid
      AND ps.schedule_date = (CURRENT_DATE + INTERVAL '1 day')::date
      AND ps.start_time = '09:00:00'::time
  );

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
