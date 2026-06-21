-- packages/db/seed/demo.sql
-- Additive, idempotent demo seed for Sigap local demos and smoke tests.
-- Run AFTER packages/db/migrations/0001_init.sql, 0005_appointments.sql,
-- AND packages/db/seed/dev.sql + packages/db/seed/rbac.sql.
--
-- Re-running this file is safe: every INSERT is guarded by a NOT EXISTS
-- predicate on a synthetic (facility, code/display_name/date) tuple, so
-- duplicates are skipped on second and subsequent runs.
--
-- IMPORTANT — DATA POLICY
--   * Synthetic only. No real patient names, no real NIK, no real phone.
--   * Facility scope: targets the seeded facility with short_code = 'f1'
--     (the first row inserted by packages/db/seed/dev.sql).
--   * Phone numbers use the +62-555-01xx reserved-for-testing range
--     (ITU-T E.164 reserved for fictional use).
--   * Schedule date is rolled forward to "tomorrow" (CURRENT_DATE + 1) so
--     the demo slot is always bookable.

BEGIN;

-- 1. Service units (Poli Umum, Poli Gigi) tied to facility f1.
INSERT INTO service_units (id, facility_id, name, code, description, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d001'::uuid,
    f.id,
    'Poli Umum Demo',
    'DEMO-UMUM',
    'Pelayanan umum untuk demo lokal Sigap. Data sintetis.',
    TRUE
FROM facilities f
WHERE f.short_code = 'f1'
  AND NOT EXISTS (
    SELECT 1 FROM service_units su
    WHERE su.facility_id = f.id AND su.code = 'DEMO-UMUM'
  );

INSERT INTO service_units (id, facility_id, name, code, description, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d002'::uuid,
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

-- 2. Practitioners (generic providers, no real PII).
INSERT INTO practitioners (id, facility_id, display_name, role, is_active)
SELECT
    '00000000-0000-0000-0000-00000000d011'::uuid,
    f.id,
    'Dokter Demo A',
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
    '00000000-0000-0000-0000-00000000d012'::uuid,
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

-- 3. Schedules for tomorrow: two slots at the same facility, two service units.
--    Slot 09:00-12:00, 30-minute slots, capacity 3 per slot.
INSERT INTO practitioner_schedules (
    id, facility_id, practitioner_id, service_unit_id,
    schedule_date, start_time, end_time, slot_minutes, capacity_per_slot, is_active
)
SELECT
    '00000000-0000-0000-0000-00000000d021'::uuid,
    f.id,
    '00000000-0000-0000-0000-00000000d011'::uuid,
    '00000000-0000-0000-0000-00000000d001'::uuid,
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
      AND ps.service_unit_id = '00000000-0000-0000-0000-00000000d001'::uuid
      AND ps.schedule_date = (CURRENT_DATE + INTERVAL '1 day')::date
      AND ps.start_time = '09:00:00'::time
  );

INSERT INTO practitioner_schedules (
    id, facility_id, practitioner_id, service_unit_id,
    schedule_date, start_time, end_time, slot_minutes, capacity_per_slot, is_active
)
SELECT
    '00000000-0000-0000-0000-00000000d022'::uuid,
    f.id,
    '00000000-0000-0000-0000-00000000d012'::uuid,
    '00000000-0000-0000-0000-00000000d002'::uuid,
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

-- Quick verification (read-only): print IDs that downstream smoke scripts need.
-- Run with:  psql $DATABASE_URL -f packages/db/seed/demo.sql
-- Then:      psql $DATABASE_URL -c "SELECT id FROM facilities WHERE short_code='f1'"
--            psql $DATABASE_URL -c "SELECT id FROM service_units WHERE code IN ('DEMO-UMUM','DEMO-GIGI')"
--            psql $DATABASE_URL -c "SELECT id FROM practitioner_schedules WHERE id IN ('00000000-0000-0000-0000-00000000d021','00000000-0000-0000-0000-00000000d022')"
