-- dev.sql seed for Sigap (realistic but fictional data for local dev)
-- Run after 0001_init.sql
--
-- Idempotent: each facility uses a deterministic UUID so re-running this
-- file never creates duplicate rows. ON CONFLICT (id) DO UPDATE refreshes
-- the canonical row instead of inserting a new one.

INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, short_code, is_active)
VALUES
    ('00000000-0000-0000-0000-00000000e000'::uuid, 'RSUD Kota Sehat', 'rumah_sakit', 'Jl. Kesehatan No. 1', 'Sukamaju', 'Kota Bandung', 'Jawa Barat', '022-123456', 180, 42, 'RSK', true),
    ('00000000-0000-0000-0000-00000000e001'::uuid, 'Puskesmas Sukajaya', 'puskesmas', 'Jl. Melati No. 7', 'Sukajaya', 'Kab. Bandung', 'Jawa Barat', '022-654321', 28, 19, 'PKM', true),
    ('00000000-0000-0000-0000-00000000e002'::uuid, 'RS Mitra Sehat', 'rumah_sakit', 'Jl. Sudirman No. 45', 'Menteng', 'Jakarta Pusat', 'DKI Jakarta', '021-987654', 95, 11, 'RSM', true),
    ('00000000-0000-0000-0000-00000000e003'::uuid, 'Puskesmas Melati Indah', 'puskesmas', 'Jl. Anggrek No. 12', 'Cilandak', 'Jakarta Selatan', 'DKI Jakarta', '021-555123', 35, 27, 'PMI', true),
    ('00000000-0000-0000-0000-00000000e004'::uuid, 'RSUD Sejahtera', 'rumah_sakit', 'Jl. Merdeka No. 88', 'Cibadak', 'Kab. Sukabumi', 'Jawa Barat', '0266-212121', 120, 68, 'RSJ', true),
    ('00000000-0000-0000-0000-00000000e005'::uuid, 'Puskesmas Harapan Baru', 'puskesmas', 'Jl. Raya No. 3', 'Parung', 'Kab. Bogor', 'Jawa Barat', '0251-876543', 22, 5, 'PHB', true)
ON CONFLICT (id) DO UPDATE SET
    name           = EXCLUDED.name,
    type           = EXCLUDED.type,
    address        = EXCLUDED.address,
    kecamatan      = EXCLUDED.kecamatan,
    kabupaten_kota = EXCLUDED.kabupaten_kota,
    provinsi       = EXCLUDED.provinsi,
    phone          = EXCLUDED.phone,
    total_beds     = EXCLUDED.total_beds,
    available_beds = EXCLUDED.available_beds,
    short_code     = EXCLUDED.short_code,
    is_active      = EXCLUDED.is_active;

-- Synthetic dev identity for local smoke/dev audit logging.
-- The UUID matches the default DevUserId in smoke scripts (d999).
INSERT INTO app_users (id, display_name, status)
VALUES (
    '00000000-0000-0000-0000-00000000d999'::uuid,
    'dev-smoke (synthetic)',
    'active'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO app_users (id, email, display_name, status, subject, deleted_at)
VALUES
    ('00000000-0000-0000-0000-00000000d990'::uuid, 'global-admin@sigap.local', 'Local Global Super Admin (synthetic)', 'active', 'local-global-super-admin', NULL),
    ('00000000-0000-0000-0000-00000000d991'::uuid, 'facility-admin@sigap.local', 'Local Facility Admin (synthetic)', 'active', 'local-facility-admin', NULL),
    ('00000000-0000-0000-0000-00000000d992'::uuid, 'zero-scope-admin@sigap.local', 'Local Zero-Scope Admin (synthetic)', 'active', 'local-zero-scope-admin', NULL)
ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    display_name = EXCLUDED.display_name,
    status = EXCLUDED.status,
    subject = EXCLUDED.subject,
    deleted_at = NULL,
    updated_at = NOW();

-- ============================================================
-- Phase 3B5.0 — local DB-backed mutation-test actors.
--
-- The dev identity deliberately lacks schedule.manage, so the ScheduleEditor
-- E2E needs a real operator who holds it. These two app_users exist so the
-- local test identity selector (X-Sigap-Local-Test-Subject, armed only under
-- SIGAP_ENV=local) can resolve genuine, DB-seeded permissions.
--
-- Nothing here invents a permission or a role. Both actors take their
-- authorization entirely from the EXISTING system roles in rbac.sql via the
-- user_roles rows seeded in demo.sql — this file only creates the identities.
-- The roles are assigned in demo.sql so that facility scoping lives with the
-- other demo RBAC assertions.
--
-- d993  e2e-schedule-manager   — facility_admin at the canonical demo
--                                facility, which carries schedule.manage.
--                                The "authorized" actor: the options endpoint
--                                returns its facility.
-- d994  e2e-schedule-mixed     — operator (schedule.read, NO schedule.manage)
--                                at facility A and facility_admin
--                                (schedule.manage) at facility B. The options
--                                endpoint must return B and never A. This is
--                                the actor that proves no Cartesian
--                                authorization regression sneaks back in.
--
-- d995  e2e-schedule-reader    — operator (schedule.read, NO schedule.manage)
--                                at ONE facility and NOWHERE else. This is the
--                                only actor that reaches the capability-refusal
--                                state: it can READ schedules, so the page is
--                                not in the class-1 empty scope, yet it manages
--                                schedules nowhere, so
--                                `/api/v1/admin/schedules/options` answers 200
--                                with `facilities: []`.
--
--                                WHY THIS ACTOR CANNOT BE FAKED BY ANOTHER.
--                                The three pre-existing actors each fail to
--                                produce the state for a structural reason, not
--                                an incidental one:
--                                  - e2e-schedule-manager manages a facility, so
--                                    options is non-empty and the editor opens.
--                                  - e2e-schedule-mixed ALSO holds
--                                    schedule.manage (at B), so options is
--                                    non-empty. Its read/manage divergence proves
--                                    the options list is provenance-filtered; it
--                                    cannot prove the EMPTY case.
--                                  - local-zero-scope-admin has no facility scope
--                                    at all, so `isEmptyScope(facilities)` is true
--                                    and the page renders the class-1 empty state.
--                                    That is a DIFFERENT state with a different
--                                    message, and conflating the two would tell a
--                                    reader who can see schedules that they have
--                                    no facilities — which is false.
--                                So the empty-options-with-readable-scope state
--                                needs its own actor, and the seed below
--                                self-verifies that it exists.
--
-- These are LOCAL TEST DATA. They are not a migration, they are not applied in
-- any shared environment, and access derives from seeded roles rather than
-- from any hardcoded grant in application code.
-- ============================================================
INSERT INTO app_users (id, email, display_name, status, subject, deleted_at)
VALUES
    ('00000000-0000-0000-0000-00000000d993'::uuid, 'e2e-schedule-manager@sigap.local', 'Local E2E Schedule Manager (synthetic)', 'active', 'e2e-schedule-manager', NULL),
    ('00000000-0000-0000-0000-00000000d994'::uuid, 'e2e-schedule-mixed@sigap.local', 'Local E2E Mixed-Provenance Schedule Actor (synthetic)', 'active', 'e2e-schedule-mixed', NULL),
    ('00000000-0000-0000-0000-00000000d995'::uuid, 'e2e-schedule-reader@sigap.local', 'Local E2E Schedule Reader (synthetic)', 'active', 'e2e-schedule-reader', NULL)
ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    display_name = EXCLUDED.display_name,
    status = EXCLUDED.status,
    subject = EXCLUDED.subject,
    deleted_at = NULL,
    updated_at = NOW();
