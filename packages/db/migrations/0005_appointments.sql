-- 0005_appointments.sql
-- Appointment scheduling and practitioner schedule foundation for Sigap.
-- Forward-only: creates new tables, does not touch existing schema.
-- Run with: psql $DATABASE_URL -f packages/db/migrations/0005_appointments.sql

CREATE TYPE appointment_status AS ENUM ('scheduled', 'checked_in', 'queued', 'completed', 'cancelled', 'no_show');

CREATE TABLE service_units (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  facility_id UUID NOT NULL REFERENCES facilities(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  code TEXT,
  description TEXT,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE practitioners (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  facility_id UUID NOT NULL REFERENCES facilities(id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  role TEXT,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE practitioner_schedules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  facility_id UUID NOT NULL REFERENCES facilities(id) ON DELETE CASCADE,
  practitioner_id UUID REFERENCES practitioners(id) ON DELETE SET NULL,
  service_unit_id UUID NOT NULL REFERENCES service_units(id) ON DELETE CASCADE,
  schedule_date DATE NOT NULL,
  start_time TIME NOT NULL,
  end_time TIME NOT NULL,
  slot_minutes INTEGER NOT NULL,
  capacity_per_slot INTEGER NOT NULL DEFAULT 1,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT schedule_time_valid CHECK (end_time > start_time),
  CONSTRAINT schedule_slot_positive CHECK (slot_minutes > 0),
  CONSTRAINT schedule_capacity_positive CHECK (capacity_per_slot > 0)
);

CREATE TABLE appointments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  facility_id UUID NOT NULL REFERENCES facilities(id) ON DELETE CASCADE,
  service_unit_id UUID NOT NULL REFERENCES service_units(id) ON DELETE CASCADE,
  practitioner_id UUID REFERENCES practitioners(id) ON DELETE SET NULL,
  practitioner_schedule_id UUID REFERENCES practitioner_schedules(id) ON DELETE SET NULL,
  appointment_time TIMESTAMPTZ NOT NULL,
  status appointment_status NOT NULL DEFAULT 'scheduled',
  patient_display_name TEXT NOT NULL,
  patient_phone TEXT NOT NULL,
  checkin_code TEXT NOT NULL,
  queue_ticket_id UUID REFERENCES queue_tickets(id) ON DELETE SET NULL,
  checkin_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common admin and booking queries
CREATE INDEX idx_service_units_facility_id ON service_units (facility_id);
CREATE INDEX idx_practitioners_facility_id ON practitioners (facility_id);
CREATE INDEX idx_practitioner_schedules_facility_id ON practitioner_schedules (facility_id);
CREATE INDEX idx_practitioner_schedules_date ON practitioner_schedules (schedule_date);
CREATE INDEX idx_practitioner_schedules_service_unit_id ON practitioner_schedules (service_unit_id);
CREATE INDEX idx_appointments_facility_id ON appointments (facility_id);
CREATE INDEX idx_appointments_service_unit_id ON appointments (service_unit_id);
CREATE INDEX idx_appointments_practitioner_id ON appointments (practitioner_id);
CREATE INDEX idx_appointments_appointment_time ON appointments (appointment_time);
CREATE INDEX idx_appointments_status ON appointments (status);
CREATE INDEX idx_appointments_patient_phone ON appointments (patient_phone);
CREATE INDEX idx_appointments_queue_ticket_id ON appointments (queue_ticket_id);
CREATE INDEX idx_appointments_checkin_code ON appointments (checkin_code);

COMMENT ON TYPE appointment_status IS 'Lifecycle of an appointment: scheduled (initial), checked_in (patient arrived), queued (linked to queue ticket), completed (service done), cancelled (patient/staff cancelled), no_show (missed)';
COMMENT ON TABLE service_units IS 'Service units (poli/clinics) within a facility that offer appointments.';
COMMENT ON TABLE practitioners IS 'Healthcare practitioners or generic service providers assigned to a facility. No sensitive PII stored.';
COMMENT ON TABLE practitioner_schedules IS 'Specific-date schedules for practitioners on a given service unit. Weekly recurring logic is deferred to a later phase.';
COMMENT ON TABLE appointments IS 'Patient appointments. Patient data is stored inline (minimization) without referencing the patients table directly. Queue ticket link is optional until check-in completes.';
COMMENT ON COLUMN appointments.practitioner_schedule_id IS 'Links to the schedule that created the available slot. NULL if booked without a schedule template.';
COMMENT ON COLUMN appointments.checkin_code IS 'Short unique code for digital check-in. Generated at booking time. Not stored in audit metadata.';
