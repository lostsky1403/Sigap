-- 0016_facilities_geo_demo.sql
-- Koordinat + penanda demo untuk tabel facilities (Fase 1: kejujuran data).
--
-- Konteks: /api/v1/facilities/nearby selama ini memakai daftar 6 faskes
-- hardcoded di Go (apps/api/cmd/server/main.go) lengkap dengan jumlah bed
-- fiktif — warga melihat faskes fiktif sebagai fakta. Mulai migrasi ini,
-- endpoint itu membaca tabel facilities: kolom latitude/longitude dipakai
-- untuk menghitung jarak, dan kolom is_demo menandai baris demo/sintetis
-- supaya tidak pernah disajikan sebagai faskes nyata.
--
-- Perubahan (aditif, forward-only, tidak mengubah 0001–0015):
--   1. Tambah kolom latitude/longitude DOUBLE PRECISION (boleh NULL: faskes
--      tanpa koordinat tidak bisa diurutkan berdasarkan jarak, jadi tidak
--      muncul di /facilities/nearby) + CHECK rentang koordinat yang valid.
--   2. Tambah kolom is_demo BOOLEAN NOT NULL DEFAULT FALSE. Faskes nyata
--      yang di-onboarding sesudahnya dibuat dengan is_demo = false (default
--      kolom + admin API); baris seed demo mengisi is_demo = true.
--   3. Backfill: semua baris yang SUDAH ADA ditandai is_demo = true. Alasan:
--      sampai migrasi ini dibuat, belum ada satu pun faskes nyata yang
--      di-onboarding di deployment mana pun — semua baris berasal dari seed
--      demo (dev.sql, demo.sql, regional_tenancy.sql). Tanpa backfill ini,
--      baris demo lama akan terbaca sebagai faskes nyata oleh endpoint baru.
--   4. Perbaiki COMMENT ON medical_records: signature bukan lagi "tamper-proof
--      proof" — kolom itu menyimpan HMAC-SHA256 dengan secret server
--      (SIGAP_SIGNING_SECRET), yaitu checksum integritas, bukan tanda tangan
--      digital. File 0002 sendiri TIDAK diubah (migrasi yang sudah diterapkan
--      tidak boleh disentuh — guard inventaris menolaknya); komentar yang
--      hidup di database diperbaiki di sini.

ALTER TABLE facilities ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION;
ALTER TABLE facilities ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;
ALTER TABLE facilities ADD COLUMN IF NOT EXISTS is_demo BOOLEAN NOT NULL DEFAULT FALSE;

DO $$ BEGIN
  ALTER TABLE facilities ADD CONSTRAINT facilities_latitude_range
    CHECK (latitude IS NULL OR (latitude BETWEEN -90 AND 90));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
  ALTER TABLE facilities ADD CONSTRAINT facilities_longitude_range
    CHECK (longitude IS NULL OR (longitude BETWEEN -180 AND 180));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Semua baris yang ada saat ini berasal dari seed demo (lihat butir 3 di atas).
UPDATE facilities SET is_demo = true;

COMMENT ON COLUMN facilities.latitude IS 'Lintang faskes (WGS84) untuk /facilities/nearby. NULL = tidak bisa diurutkan berdasarkan jarak.';
COMMENT ON COLUMN facilities.longitude IS 'Bujur faskes (WGS84) untuk /facilities/nearby. NULL = tidak bisa diurutkan berdasarkan jarak.';
COMMENT ON COLUMN facilities.is_demo IS 'TRUE = baris demo/sintetis (seed dev/demo/regional). Baris demo tidak pernah disajikan sebagai faskes nyata: /facilities/nearby dan katalog publik mengecualikannya secara default. Faskes nyata di-onboarding dengan is_demo = false.';

COMMENT ON TABLE medical_records IS 'Patient visit/queue history with HMAC-SHA256 record-integrity checksum (Health Wallet feature). The checksum is verifiable only with the server signing secret; it is NOT a digital signature and proves nothing to a third party.';
COMMENT ON COLUMN medical_records.signature IS 'HMAC-SHA256(phone | facility_id | queue_number | formatted | visit_time) with the server signing secret (SIGAP_SIGNING_SECRET), computed at creation in the Rust engine. Rows created before the HMAC migration carry a plain SHA-256 hash in the same column (no verification consumer exists, so no backfill).';
