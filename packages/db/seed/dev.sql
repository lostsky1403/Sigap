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
