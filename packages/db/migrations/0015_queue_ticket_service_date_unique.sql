-- 0015_queue_ticket_service_date_unique.sql
-- Backstop ganda untuk nomor antrean: UNIQUE (facility_id, service_date, queue_number)
-- pada queue_tickets, plus kolom service_date (hari layanan WIB) sebagai kunci
-- hari yang eksplisit.
--
-- Konteks: nomor antrean selama ini unik hanya berkat lock counter di
-- daily_queue_counters (apps/queue-engine/src/engine/queue.rs). Bila dua
-- transaksi lolos bersamaan — atau bila ada jalur tulis lain (insert manual)
-- DB tidak menolak duplikat. Migrasi ini membuat DB sendiri yang menolak
-- nomor ganda untuk satu fasilitas di satu hari layanan. Perubahan pendamping
-- di Rust (branch yang sama): counter naik via satu INSERT ... ON CONFLICT
-- DO UPDATE ... RETURNING, dan ticket insert mengisi service_date eksplisit.
--
-- Perubahan (aditif, forward-only, tidak mengubah 0001–0014):
--   1. Tambah kolom service_date DATE (tanpa DEFAULT dulu).
--   2. Backfill baris lama dari registered_at dengan rumus WIB yang sama
--      seperti 0012 (sesi-timezone-independent: UTC + 7 jam).
--   3. service_date diberi DEFAULT hari WIB saat ini, lalu NOT NULL.
--   4. UNIQUE INDEX uq_queue_ticket_facility_day_number
--      ON queue_tickets (facility_id, service_date, queue_number).
--
-- Pemeriksaan duplikat SEBELUM apply (wajib): jalankan query ini di DB target.
-- Bila mengembalikan baris, selesaikan duplikat secara manual dulu — migrasi
-- ini GAGAL dengan RAISE EXCEPTION bila duplikat masih ada. Sengaja gagal
-- keras: backstop yang diam-diam tidak ada lebih berbahaya daripada migrasi
-- yang gagal (pola no-op-diam-diam 0012 sudah dicatat sebagai temuan di
-- laporan eksplorasi §10.4.25). Setelah duplikat dibersihkan, migrasi bisa
-- dijalankan ulang — semua langkah idempotent (IF NOT EXISTS).
--   SELECT facility_id,
--          ((registered_at AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date AS wib_day,
--          queue_number, count(*)
--     FROM queue_tickets
--    GROUP BY facility_id, wib_day, queue_number
--   HAVING count(*) > 1;
--
-- Run with: psql $DATABASE_URL -f packages/db/migrations/0015_queue_ticket_service_date_unique.sql
-- (atau lewat runner: make db-migrate-all — runner mencatat checksum di schema_migrations)

DO $migration$
DECLARE
  duplicate_count bigint;
BEGIN
  SELECT count(*) INTO duplicate_count
  FROM (
    SELECT facility_id,
           ((registered_at AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date AS wib_day,
           queue_number
      FROM queue_tickets
     GROUP BY facility_id, wib_day, queue_number
    HAVING count(*) > 1
  ) AS duplicates;

  IF duplicate_count > 0 THEN
    RAISE EXCEPTION '0015_queue_ticket_service_date_unique: % grup (facility_id, hari WIB, queue_number) duplikat di queue_tickets; selesaikan duplikat secara manual dulu, lalu jalankan ulang migrasi ini.', duplicate_count;
  END IF;

  ALTER TABLE queue_tickets
    ADD COLUMN IF NOT EXISTS service_date DATE;

  UPDATE queue_tickets
     SET service_date = ((registered_at AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date
   WHERE service_date IS NULL;

  ALTER TABLE queue_tickets
    ALTER COLUMN service_date SET DEFAULT ((NOW() AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date;

  ALTER TABLE queue_tickets
    ALTER COLUMN service_date SET NOT NULL;

  CREATE UNIQUE INDEX IF NOT EXISTS uq_queue_ticket_facility_day_number
    ON queue_tickets (facility_id, service_date, queue_number);

  RAISE NOTICE '0015_queue_ticket_service_date_unique: service_date terisi dan UNIQUE (facility_id, service_date, queue_number) aktif.';
END $migration$;

COMMENT ON COLUMN queue_tickets.service_date IS
  'Hari layanan WIB (Asia/Jakarta, UTC+7) dari tiket ini — hari yang sama dengan kunci daily_queue_counters.date. Diisi eksplisit oleh queue-engine (service_day_wib); DEFAULT menjaga penulis lain tetap jalan. Bersama queue_number membentuk UNIQUE per fasilitas per hari.';
