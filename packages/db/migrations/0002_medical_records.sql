-- 0002_medical_records.sql
-- Immutable medical records / health wallet for Sigap.
-- Each record has cryptographic signature (SHA-256 computed in Rust engine) for tamper-proof audit trail.
-- Linked to patients by phone (simple for MVP; could use patient_id).

CREATE TABLE medical_records (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  patient_phone TEXT NOT NULL,
  facility_id UUID NOT NULL REFERENCES facilities(id) ON DELETE RESTRICT,
  queue_number INTEGER,
  formatted_number TEXT,
  signature TEXT NOT NULL,           -- hex SHA-256 of core visit data (immutable proof)
  visit_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_medical_records_phone_time ON medical_records (patient_phone, visit_time DESC);

COMMENT ON TABLE medical_records IS 'Immutable patient visit/queue history with cryptographic signature for Health Wallet feature';
COMMENT ON COLUMN medical_records.signature IS 'SHA-256( phone | facility_id | queue_number | formatted | visit_time ) computed at creation in Rust engine';
