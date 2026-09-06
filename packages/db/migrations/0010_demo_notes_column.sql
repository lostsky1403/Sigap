-- 0010_demo_notes_column.sql
-- Isolate demo schema change from seed data (AUDIT-607).
-- Previously: packages/db/seed/demo.sql contained DDL
--   ALTER TABLE appointments ADD COLUMN IF NOT EXISTS notes TEXT
-- Now: schema change is tracked as a numbered migration so seeds are
-- data-only. This column is safe to add on both fresh installs and
-- existing databases that already ran the demo seed.
-- Forward-only: adds column idempotently; does not touch existing schema.

ALTER TABLE appointments ADD COLUMN IF NOT EXISTS notes TEXT;

COMMENT ON COLUMN appointments.notes IS 'Optional free-form notes attached to the appointment (demo/local use; synthetic data only)';
