-- 0007_checkin_constraints.sql
-- Constraints supporting atomic check-in transitions (AUDIT-1004).
-- Forward-only: adds constraints to existing tables.
-- Run with: psql $DATABASE_URL -f packages/db/migrations/0007_checkin_constraints.sql

-- Prevent two appointments from sharing the same queue ticket.
-- The queue engine generates one ticket per check-in; a unique constraint
-- ensures the FK relationship is 1:1 at the database level.
ALTER TABLE appointments
  ADD CONSTRAINT uq_appointments_queue_ticket_id
  UNIQUE (queue_ticket_id);

-- Immutable wrapper for timestamptz::date so it can be used in index
-- expressions.  PostgreSQL requires index expressions to be IMMUTABLE;
-- the bare ::date cast is STABLE (depends on session TimeZone).
CREATE OR REPLACE FUNCTION appointment_day(timestamptz) RETURNS date AS $$
  SELECT $1::date
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;

-- Partial unique index: at most one non-cancelled appointment per patient
-- phone per facility per day.  Prevents duplicate bookings that waste
-- capacity slots.  Uses a partial index so cancelled/no_show appointments
-- do not block legitimate rebookings.
CREATE UNIQUE INDEX IF NOT EXISTS uq_active_booking_per_patient_day
  ON appointments (facility_id, patient_phone, appointment_day(appointment_time))
  WHERE status NOT IN ('cancelled', 'no_show');
