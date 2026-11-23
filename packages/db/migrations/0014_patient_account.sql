-- 0014_patient_account.sql
-- Link appointments to the app_users account that OWNS them: the citizen's
-- internal patient identity (an app_user row that is a CITIZEN, NOT a staff
-- role). See design/patient-identity-model.md.
--
-- Backward-compatible and additive:
--   * The column is NULLABLE, so existing anonymous bookings (and future
--     check-in-code bookings made without a signed-in citizen) keep
--     patient_account_id = NULL — NULL is the "anonymous" state, never a
--     backfilled guess.
--   * The FK targets app_users(id) with ON DELETE SET NULL: deleting a
--     citizen account anonymizes their appointment rows (NULL) rather than
--     deleting clinical scheduling history.
--
-- Forward-only: adds one nullable FK column and one index. Does not edit
-- 0001–0013.
-- Run with: psql $DATABASE_URL -f packages/db/migrations/0014_patient_account.sql

ALTER TABLE appointments
  ADD COLUMN IF NOT EXISTS patient_account_id UUID REFERENCES app_users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_appointments_patient_account_id
  ON appointments (patient_account_id);

COMMENT ON COLUMN appointments.patient_account_id IS
  'The app_users.id of the citizen account that owns this appointment (internal patient identity). NULL for anonymous bookings made with a check-in code without a signed-in citizen. FK to app_users(id) ON DELETE SET NULL: deleting a citizen account anonymizes its appointments instead of deleting them.';
