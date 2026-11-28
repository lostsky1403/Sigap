-- packages/db/seed/regional_tenancy.sql
-- [synthetic] Regional tenancy seed: two fictional healthcare organizations,
-- four fictional facilities, two service units per facility, and the role /
-- permission / assignment rows needed to exercise org- and region-scoped RBAC,
-- plus one CITIZEN app_user (internal patient identity; no staff role).
--
-- SYNTHETIC ONLY. Every name, facility, organization, region, and user below
-- is fictional. No real patient data, no real PII, no real NIK/phone. Phone
-- numbers reuse the reserved-for-testing +62-555-01xx (ITU-T E.164) range.
--
-- Run AFTER packages/db/migrations/0013_organizations.sql and the existing
-- packages/db/seed/rbac.sql (which defines super_admin/facility_admin/
-- operator/viewer). Idempotent: deterministic UUIDs + ON CONFLICT + a
-- DELETE-then-reinsert guard for user_roles mirroring demo.sql.

BEGIN;

-- ============================================================
-- Organizations (2, distinct regions)
-- ============================================================
INSERT INTO organizations (id, name, region, is_active)
VALUES
    ('00000000-0000-0000-0000-00000000c001'::uuid, 'RS Sehat Nusantara Group', 'Jawa Barat', true),
    ('00000000-0000-0000-0000-00000000c002'::uuid, 'Puskesmas Prima Sejahtera Network', 'DKI Jakarta', true)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    region = EXCLUDED.region,
    is_active = EXCLUDED.is_active;

-- ============================================================
-- Facilities: 2 per organization (4 total), all synthetic
-- Suffix r001/r002 = org A; r003/r004 = org B
-- ============================================================
INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, short_code, is_active, organization_id, latitude, longitude, is_demo)
VALUES
    ('00000000-0000-0000-0000-00000000c101'::uuid, 'RS Sehat Nusantara Cimahi', 'rumah_sakit', 'Jl. Fiksi Nusantara No. 1', 'Cimahi Utara', 'Kota Cimahi', 'Jawa Barat', '+62-555-0101', 120, 40, 'RSNCM', true, '00000000-0000-0000-0000-00000000c001'::uuid, -6.8721, 107.5420, true),
    ('00000000-0000-0000-0000-00000000c102'::uuid, 'RS Sehat Nusantara Bogor',   'rumah_sakit', 'Jl. Fiksi Nusantara No. 2', 'Bogor Timur',  'Kota Bogor',   'Jawa Barat', '+62-555-0102', 90,  25, 'RSNBG', true, '00000000-0000-0000-0000-00000000c001'::uuid, -6.5971, 106.8060, true),
    ('00000000-0000-0000-0000-00000000c103'::uuid, 'Puskesmas Prima Tangerang',  'puskesmas',   'Jl. Fiksi Prima No. 1',     'Cipondoh',     'Kota Tangerang', 'Banten',     '+62-555-0103', 30,  12, 'PPTGR', true, '00000000-0000-0000-0000-00000000c002'::uuid, -6.1783, 106.6319, true),
    ('00000000-0000-0000-0000-00000000c104'::uuid, 'Puskesmas Prima Depok',       'puskesmas',   'Jl. Fiksi Prima No. 2',     'Sukmajaya',    'Kota Depok',  'Jawa Barat', '+62-555-0104', 28,  9,  'PPDPK', true, '00000000-0000-0000-0000-00000000c002'::uuid, -6.4025, 106.7942, true)
ON CONFLICT (id) DO UPDATE SET
    name            = EXCLUDED.name,
    type            = EXCLUDED.type,
    address         = EXCLUDED.address,
    kecamatan       = EXCLUDED.kecamatan,
    kabupaten_kota  = EXCLUDED.kabupaten_kota,
    provinsi        = EXCLUDED.provinsi,
    phone           = EXCLUDED.phone,
    total_beds      = EXCLUDED.total_beds,
    available_beds  = EXCLUDED.available_beds,
    short_code      = EXCLUDED.short_code,
    is_active       = EXCLUDED.is_active,
    organization_id = EXCLUDED.organization_id,
    latitude        = EXCLUDED.latitude,
    longitude       = EXCLUDED.longitude,
    is_demo         = EXCLUDED.is_demo;

-- ============================================================
-- Service units: 2 per facility (8 total)
-- NOTE: service_units schema depends on the actual columns. This seed inserts
-- only the columns that are ubiquitous across 0005 evolution; a later column
-- mismatch surfaces as a loud SQL error (desired) rather than silent success.
-- ============================================================
INSERT INTO service_units (id, facility_id, name, code, description, is_active)
SELECT v.id, v.facility_id, v.name, v.code, '', true
FROM (VALUES
    ('00000000-0000-0000-0000-00000000c201'::uuid, '00000000-0000-0000-0000-00000000c101'::uuid, 'Poli Umum Fiksi A1', 'PU-A1'),
    ('00000000-0000-0000-0000-00000000c202'::uuid, '00000000-0000-0000-0000-00000000c101'::uuid, 'Poli Gigi Fiksi A1', 'PG-A1'),
    ('00000000-0000-0000-0000-00000000c203'::uuid, '00000000-0000-0000-0000-00000000c102'::uuid, 'Poli Umum Fiksi A2', 'PU-A2'),
    ('00000000-0000-0000-0000-00000000c204'::uuid, '00000000-0000-0000-0000-00000000c102'::uuid, 'Poli Anak Fiksi A2', 'PA-A2'),
    ('00000000-0000-0000-0000-00000000c205'::uuid, '00000000-0000-0000-0000-00000000c103'::uuid, 'Poli Umum Fiksi B1', 'PU-B1'),
    ('00000000-0000-0000-0000-00000000c206'::uuid, '00000000-0000-0000-0000-00000000c103'::uuid, 'Poli KIA Fiksi B1',  'KIA-B1'),
    ('00000000-0000-0000-0000-00000000c207'::uuid, '00000000-0000-0000-0000-00000000c104'::uuid, 'Poli Umum Fiksi B2', 'PU-B2'),
    ('00000000-0000-0000-0000-00000000c208'::uuid, '00000000-0000-0000-0000-00000000c104'::uuid, 'Poli Lansia Fiksi B2', 'PL-B2')
) AS v(id, facility_id, name, code)
LEFT JOIN service_units existing ON existing.id = v.id
WHERE existing.id IS NULL;

-- ============================================================
-- New roles + permissions (reuse the existing rbac model)
-- ============================================================
INSERT INTO roles (name, description, is_system)
VALUES
    ('organization_admin', 'Manage one healthcare organization''s facilities and support staff', true),
    ('regional_admin', 'Manage tenancy structure and read operational stats across a region', true)
ON CONFLICT (name) DO NOTHING;

INSERT INTO permissions (key, description)
VALUES
    ('organization.read', 'Read organization directory and non-sensitive operational overview'),
    ('organization.manage', 'Create/update organizations and their facility membership')
ON CONFLICT (key) DO NOTHING;

-- organization_admin: org-scoped, operational manage across its org + org dir.
WITH role_ids AS (
  SELECT id FROM roles WHERE name = 'organization_admin'
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN (
    'organization.read', 'organization.manage',
    'facility.read', 'facility.manage',
    'queue.read', 'queue.manage', 'queue.generate',
    'appointment.read', 'appointment.manage',
    'schedule.read', 'schedule.manage',
    'notification.read'
  )
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r JOIN perm_ids p ON true
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- regional_admin: operational STATS ONLY (read), including the org directory.
-- Deliberately NO .manage keys at all (organization.manage included) and NO
-- medical_record.read (that permission does not exist). audit.read stays
-- super_admin-only, unchanged. This matches the no-.manage invariant declared
-- in auth/facility_scope.go and auth/rbac_resolver.go.
WITH role_ids AS (
  SELECT id FROM roles WHERE name = 'regional_admin'
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN (
    'organization.read',
    'facility.read',
    'queue.read',
    'appointment.read',
    'schedule.read',
    'notification.read'
  )
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r JOIN perm_ids p ON true
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- super_admin: full system access. Add the two new organization permissions so
-- the system administrator can view and manage the org directory (the preview
-- actor typically runs as the global super admin).
WITH role_ids AS (
  SELECT id FROM roles WHERE name = 'super_admin'
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('organization.read', 'organization.manage')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r JOIN perm_ids p ON true
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- ============================================================
-- Synthetic app users (identity only; authorization comes from roles below)
-- ============================================================
INSERT INTO app_users (id, email, display_name, status, subject, deleted_at)
VALUES
    ('00000000-0000-0000-0000-00000000c301'::uuid, 'org-admin-a@sigap.local', 'Org A Admin (synthetic)', 'active', 'local-org-admin-a', NULL),
    ('00000000-0000-0000-0000-00000000c302'::uuid, 'org-admin-b@sigap.local', 'Org B Admin (synthetic)', 'active', 'local-org-admin-b', NULL),
    ('00000000-0000-0000-0000-00000000c303'::uuid, 'regional-admin@sigap.local', 'Regional Admin (synthetic)', 'active', 'local-regional-admin', NULL),
    ('00000000-0000-0000-0000-00000000c304'::uuid, 'facility-admin-a1@sigap.local', 'Facility Admin A1 (synthetic)', 'active', 'local-facility-admin-a1', NULL),
    ('00000000-0000-0000-0000-00000000c305'::uuid, 'operator-a1@sigap.local', 'Operator A1 (synthetic)', 'active', 'local-operator-a1', NULL),
    ('00000000-0000-0000-0000-00000000c306'::uuid, 'operator-b1@sigap.local', 'Operator B1 (synthetic)', 'active', 'local-operator-b1', NULL),
    ('00000000-0000-0000-0000-00000000c307'::uuid, 'patient-p1@sigap.local', 'Citizen Patient P1 (synthetic)', 'active', 'local-patient-p1', NULL)
ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    display_name = EXCLUDED.display_name,
    status = EXCLUDED.status,
    subject = EXCLUDED.subject,
    deleted_at = NULL,
    updated_at = NOW();

-- ============================================================
-- user_roles: org-scoped, regional, and facility-scoped assignments
-- ============================================================
DELETE FROM user_roles
WHERE user_id IN (
    '00000000-0000-0000-0000-00000000c301'::uuid,
    '00000000-0000-0000-0000-00000000c302'::uuid,
    '00000000-0000-0000-0000-00000000c303'::uuid,
    '00000000-0000-0000-0000-00000000c304'::uuid,
    '00000000-0000-0000-0000-00000000c305'::uuid,
    '00000000-0000-0000-0000-00000000c306'::uuid
);

INSERT INTO user_roles (user_id, role_id, facility_id, organization_id, status, deleted_at)
SELECT
    seeded.user_id,
    r.id,
    seeded.facility_id,
    seeded.organization_id,
    'active',
    NULL
FROM (
    VALUES
        -- Org A admin: organization_admin scoped to org A (facility_id NULL).
        ('00000000-0000-0000-0000-00000000c301'::uuid, 'organization_admin', NULL::uuid, '00000000-0000-0000-0000-00000000c001'::uuid),
        -- Org B admin: organization_admin scoped to org B.
        ('00000000-0000-0000-0000-00000000c302'::uuid, 'organization_admin', NULL::uuid, '00000000-0000-0000-0000-00000000c002'::uuid),
        -- Regional admin: unscoped regional_admin (organization_id NULL, facility_id NULL).
        ('00000000-0000-0000-0000-00000000c303'::uuid, 'regional_admin', NULL::uuid, NULL::uuid),
        -- Facility admin at facility A1.
        ('00000000-0000-0000-0000-00000000c304'::uuid, 'facility_admin', '00000000-0000-0000-0000-00000000c101'::uuid, NULL::uuid),
        -- Operator at facility A1.
        ('00000000-0000-0000-0000-00000000c305'::uuid, 'operator', '00000000-0000-0000-0000-00000000c101'::uuid, NULL::uuid),
        -- Operator at facility B1 (Org B).
        ('00000000-0000-0000-0000-00000000c306'::uuid, 'operator', '00000000-0000-0000-0000-00000000c103'::uuid, NULL::uuid)
) AS seeded(user_id, role_name, facility_id, organization_id)
JOIN roles r ON r.name = seeded.role_name
ON CONFLICT (user_id, role_id) DO UPDATE SET
    facility_id     = EXCLUDED.facility_id,
    organization_id = EXCLUDED.organization_id,
    status          = EXCLUDED.status,
    deleted_at      = EXCLUDED.deleted_at;

-- ============================================================
-- Self-verification: the regional_admin seed must NOT confer any clinical or
-- manage-scoped operational permission, and organization_admin must be org-
-- scoped (not global). Fails loudly if a future rbac edit widens these.
-- ============================================================
DO $$
BEGIN
    -- organization_admin must carry organization.read (its directory view).
    IF NOT EXISTS (
        SELECT 1
        FROM roles r
        JOIN role_permissions rp ON rp.role_id = r.id
        JOIN permissions p ON p.id = rp.permission_id
        WHERE r.name = 'organization_admin' AND p.key = 'organization.read'
    ) THEN
        RAISE EXCEPTION 'regional_tenancy seed verification failed: organization_admin missing organization.read';
    END IF;

    -- regional_admin must be read-only over operational stats: no queue.manage
    -- and no facility.manage and NO medical_record.read (which does not exist).
    IF EXISTS (
        SELECT 1
        FROM roles r
        JOIN role_permissions rp ON rp.role_id = r.id
        JOIN permissions p ON p.id = rp.permission_id
        WHERE r.name = 'regional_admin'
          AND p.key IN ('organization.manage', 'queue.manage', 'facility.manage', 'appointment.manage', 'schedule.manage', 'medical_record.read')
    ) THEN
        RAISE EXCEPTION 'regional_tenancy seed verification failed: regional_admin must remain stats read-only';
    END IF;

    -- No medical_record.read permission may exist at all.
    IF EXISTS (SELECT 1 FROM permissions WHERE key = 'medical_record.read') THEN
        RAISE EXCEPTION 'regional_tenancy seed verification failed: medical_record.read must not exist';
    END IF;
END $$;

-- Self-verification: the citizen app_user must carry NO active staff role
-- assignment, so identity.Actor.PatientAccountID() resolves it as the owner
-- of patient appointments (fail-closed for every staff actor).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM user_roles ur
        WHERE ur.user_id = '00000000-0000-0000-0000-00000000c307'::uuid
          AND ur.status = 'active'
          AND ur.deleted_at IS NULL
    ) THEN
        RAISE EXCEPTION 'regional_tenancy seed verification failed: citizen patient must hold no staff role assignment';
    END IF;
END $$;

COMMIT;