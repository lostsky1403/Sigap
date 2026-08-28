-- 0009_user_roles_lifecycle.sql
-- Add lifecycle columns to user_roles so facility-scope resolution can
-- exclude disabled / soft-deleted role assignments (AUDIT-202).
-- Forward-only: adds columns and a partial unique index. Does not edit 0001-0008.
-- Run with: psql $DATABASE_URL -f packages/db/migrations/0009_user_roles_lifecycle.sql

-- status tracks whether a user-role assignment is currently effective.
-- Existing rows default to 'active' (no data loss). Only 'active' assignments
-- contribute to facility scope.
ALTER TABLE user_roles
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'
  CHECK (status IN ('active', 'inactive'));

-- Soft-delete column for role assignment lifecycle.
-- Existing rows are NULL (treated as not deleted). Only non-NULL deleted_at
-- rows are excluded from facility scope.
ALTER TABLE user_roles
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- At most one active, non-deleted role assignment per (user, role, facility).
-- Preserves existing behavior while preventing duplicate active grants.
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_roles_active_uniq
  ON user_roles (user_id, role_id, facility_id)
  WHERE status = 'active' AND deleted_at IS NULL;

-- Indexes supporting facility-scope resolution queries.
CREATE INDEX IF NOT EXISTS idx_user_roles_status_deleted
  ON user_roles (status, deleted_at)
  WHERE status = 'active' AND deleted_at IS NULL;

COMMENT ON COLUMN user_roles.status IS 'Lifecycle status of the role assignment: active or inactive. Only active assignments grant facility scope.';
COMMENT ON COLUMN user_roles.deleted_at IS 'Soft-delete timestamp. NULL means active. Non-NULL rows are excluded from facility-scope resolution.';
