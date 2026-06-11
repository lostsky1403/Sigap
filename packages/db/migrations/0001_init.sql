-- 0001_init.sql
-- Initial schema for Sigap: facilities (RS/Puskesmas), patients, queue_tickets, daily_queue_counters
-- Single source of truth. Used by Go (pgx/sqlc) and Rust (sqlx).
-- Run with: psql $DATABASE_URL -f packages/db/migrations/0001_init.sql

CREATE TYPE facility_type AS ENUM ('rumah_sakit', 'puskesmas');
CREATE TYPE queue_status AS ENUM ('waiting', 'called', 'in_service', 'completed', 'cancelled', 'skipped');

CREATE TABLE facilities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  type facility_type NOT NULL,
  address TEXT NOT NULL,
  kecamatan TEXT NOT NULL,
  kabupaten_kota TEXT NOT NULL,
  provinsi TEXT NOT NULL,
  phone TEXT NOT NULL,
  total_beds INTEGER NOT NULL DEFAULT 0,
  available_beds INTEGER NOT NULL DEFAULT 0,
  last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  short_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE patients (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  full_name TEXT NOT NULL,
  phone TEXT NOT NULL UNIQUE,
  gender TEXT CHECK (gender IN ('L', 'P')),
  date_of_birth TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE queue_tickets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  facility_id UUID NOT NULL REFERENCES facilities(id) ON DELETE RESTRICT,
  patient_id UUID NOT NULL REFERENCES patients(id) ON DELETE RESTRICT,
  queue_number INTEGER NOT NULL,
  formatted_number TEXT NOT NULL,
  status queue_status NOT NULL DEFAULT 'waiting',
  registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  called_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);

CREATE TABLE daily_queue_counters (
  facility_id UUID NOT NULL REFERENCES facilities(id) ON DELETE CASCADE,
  date DATE NOT NULL,
  last_number INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (facility_id, date)
);

-- Performance indexes (critical for queue generation queries)
CREATE INDEX idx_queue_tickets_facility_registered ON queue_tickets (facility_id, registered_at);
CREATE INDEX idx_queue_tickets_status ON queue_tickets (status);

COMMENT ON TABLE facilities IS 'Rumah Sakit dan Puskesmas (fasilitas kesehatan daerah)';
COMMENT ON TABLE daily_queue_counters IS 'Atomic daily counter per facility for nomor antrean reset at midnight';
