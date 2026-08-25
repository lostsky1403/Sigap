-- 0008_identity_subject.sql
-- Map JWT/OIDC external subjects to application users so that authorization
-- can be resolved from trusted server-side RBAC state (AUDIT-101).
-- Forward-only: adds a column and a partial unique index to an existing table.
-- Run with: psql $DATABASE_URL -f packages/db/migrations/0008_identity_subject.sql

-- Link an app user to an external identity provider subject (JWT `sub`).
-- A subject is the stable identity the token proves; it is never itself an
-- authorization source. Multiple subjects may share an email, but only one
-- active app user may claim a given subject.
ALTER TABLE app_users
  ADD COLUMN IF NOT EXISTS subject TEXT;

-- At most one active (non-soft-deleted) app user per non-null external subject.
CREATE UNIQUE INDEX IF NOT EXISTS uq_app_users_subject_active
  ON app_users (subject)
  WHERE subject IS NOT NULL AND deleted_at IS NULL;

COMMENT ON COLUMN app_users.subject IS 'External identity provider subject (JWT sub). Used to map a validated token to server-side RBAC state.';