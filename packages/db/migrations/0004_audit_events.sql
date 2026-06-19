-- 0004_audit_events.sql
-- Append-only audit event log for Sigap.
-- Every operation that mutates data or accesses PHI should leave a record here.
-- hash -- previous_hash and event_hash prepare for local tamper-evident chain.
-- Forward-only: creates a new table, does not touch existing schema.
-- Run with: psql $DATABASE_URL -f packages/db/migrations/0004_audit_events.sql

CREATE TABLE audit_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  actor_user_id UUID REFERENCES app_users(id) ON DELETE SET NULL,
  actor_type TEXT NOT NULL DEFAULT 'system',
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  facility_id UUID REFERENCES facilities(id) ON DELETE SET NULL,
  request_id TEXT,
  ip_hash TEXT,
  user_agent_hash TEXT,
  metadata JSONB NOT NULL DEFAULT '{}',
  previous_hash TEXT,
  event_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common audit queries (compliance, forensics, debugging)
CREATE INDEX idx_audit_events_occurred_at ON audit_events (occurred_at DESC);
CREATE INDEX idx_audit_events_actor_user_id ON audit_events (actor_user_id);
CREATE INDEX idx_audit_events_action ON audit_events (action);
CREATE INDEX idx_audit_events_resource_type ON audit_events (resource_type);
CREATE INDEX idx_audit_events_facility_id ON audit_events (facility_id);
CREATE INDEX idx_audit_events_request_id ON audit_events (request_id);

COMMENT ON TABLE audit_events IS 'Append-only audit log for all security-relevant actions. Never store PII/PHI in metadata.';
COMMENT ON COLUMN audit_events.actor_type IS 'system, user, service, or bot';
COMMENT ON COLUMN audit_events.ip_hash IS 'Hashed client IP for privacy. Store SHA-256(ip+salt), not raw IP.';
COMMENT ON COLUMN audit_events.user_agent_hash IS 'Hashed user-agent for privacy.';
COMMENT ON COLUMN audit_events.metadata IS 'JSONB allowlist of sanitized, non-PII context. Keys must be explicitly allowlisted by audit sanitizer.';
COMMENT ON COLUMN audit_events.previous_hash IS 'Hash of the previous audit event for tamper-evident chain. Full chain linking is future work.';
COMMENT ON COLUMN audit_events.event_hash IS 'SHA-256 hash of canonical event payload (without PII). Full chain linking is future work.';
