-- 0013_organizations.sql
-- Additive regional tenancy schema: an `organizations` grouping entity plus
-- nullable org attribution on `facilities` and `user_roles`.
--
-- This reuses the EXISTING roles/permissions/role_permissions model from
-- 0003 and the lifecycle columns from 0009. It introduces NO parallel identity
-- or RBAC table: an organization is an access-control grouping, and an
-- org-scoped role grants every facility of that organization (derived via
-- facilities.organization_id), never a second store.
--
-- Existing facilities (dev.sql 00000000-...e000..e005 + demo.sql d000) remain
-- orgless: organization_id stays NULL, so `facilityScopeQuery` and the grant
-- resolver produce byte-for-byte unchanged results for pre-existing grants.
--
-- Forward-only: creates one new table and two nullable FK columns, plus
-- indexes. Does not edit 0001-0012.

CREATE TABLE organizations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  -- Free-text region grouping (e.g. "Jawa Barat"). Regional scope is derived
  -- from organizations, mirroring how facilities.kabupaten_kota/provinsi are
  -- free-text attributes rather than registry keys.
  region TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- facilities.organization_id is the single forward edge facility -> org.
-- ON DELETE RESTRICT: an org with facilities cannot be dropped until its
-- facilities are re-parented or removed (no silent subtree cascade).
ALTER TABLE facilities
  ADD COLUMN IF NOT EXISTS organization_id UUID REFERENCES organizations(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_facilities_organization_id
  ON facilities (organization_id);

-- user_roles.organization_id marks an org-scoped role assignment. The
-- interpretation (mirroring the facility axis, see auth/org_scope.go) is:
--   organization_id NOT NULL AND facility_id IS NULL
--       => the role applies to EVERY facility of that organization.
--   organization_id IS NULL
--       => unchanged facility-scoped / global / zero-scope semantics.
ALTER TABLE user_roles
  ADD COLUMN IF NOT EXISTS organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_user_roles_organization_id
  ON user_roles (organization_id);

-- At most one ACTIVE, non-deleted assignment per (user, role, facility, org).
-- The existing 0009 index uq_user_roles_active_uniq already enforces
-- uniqueness over (user_id, role_id, facility_id); adding organization_id only
-- extends it for the org axis and cannot reject data the 0009 index accepted
-- (every existing row has organization_id NULL, which is NULLS-DISTINCT).
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_roles_active_org_uniq
  ON user_roles (user_id, role_id, facility_id, organization_id)
  WHERE status = 'active' AND deleted_at IS NULL;

COMMENT ON TABLE organizations IS 'Healthcare organization grouping facilities under a region (regional tenancy).';
COMMENT ON COLUMN organizations.region IS 'Free-text region grouping key; regional scope derives orgs from it.';
COMMENT ON COLUMN facilities.organization_id IS 'Optional org attribution. NULL for pre-existing/orgless facilities.';
COMMENT ON COLUMN user_roles.organization_id IS 'Org-scoped role assignment (facility_id NULL => applies to every facility of the org). NULL for facility/global grants.';