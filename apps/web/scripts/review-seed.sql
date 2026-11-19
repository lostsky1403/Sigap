-- Local review seed: realistic operators, units, schedules and appointments.
--
-- WHY THIS EXISTS
--
-- The review stack's own seed (`packages/db/seed/dev.sql`) gives four real
-- facilities and one synthetic demo facility, but only the demo facility has
-- service units, practitioners and schedules — so five of the seven facilities
-- can be booked in the catalog and then have nothing to book INTO, and the
-- admin appointment and schedule screens render empty tables. An empty table
-- cannot be visually reviewed: there is no way to tell a correct table with no
-- rows from a broken table.
--
-- This is LOCAL DEV DATA ONLY. It is applied to the throwaway Postgres that
-- `docker compose up postgres` creates for the review, never to a shared or
-- production database, and it is not part of the migration history. The ids are
-- in the same `00000000-...-000e*` range the existing dev seed uses, so it is
-- obvious at a glance that a row came from a seed rather than from a person.
--
-- Idempotent: every insert is `ON CONFLICT DO NOTHING` on a fixed id, so
-- re-running it is a no-op rather than a duplicate.

BEGIN;

-- One service unit per real facility, named as the frames name them.
INSERT INTO service_units (id, facility_id, name) VALUES
  ('00000000-0000-0000-0000-00000000e101', '00000000-0000-0000-0000-00000000e000', 'Poli Umum'),
  ('00000000-0000-0000-0000-00000000e102', '00000000-0000-0000-0000-00000000e000', 'Poli Gigi'),
  ('00000000-0000-0000-0000-00000000e103', '00000000-0000-0000-0000-00000000e001', 'Poli Umum'),
  ('00000000-0000-0000-0000-00000000e104', '00000000-0000-0000-0000-00000000e002', 'Poli Umum'),
  ('00000000-0000-0000-0000-00000000e105', '00000000-0000-0000-0000-00000000e003', 'Poli Umum'),
  ('00000000-0000-0000-0000-00000000e106', '00000000-0000-0000-0000-00000000e005', 'Poli Umum')
ON CONFLICT (id) DO NOTHING;

-- One practitioner per unit.
INSERT INTO practitioners (id, facility_id, display_name, role) VALUES
  ('00000000-0000-0000-0000-00000000e111', '00000000-0000-0000-0000-00000000e000', 'dr. Anita Wijaya', 'dokter_umum'),
  ('00000000-0000-0000-0000-00000000e112', '00000000-0000-0000-0000-00000000e000', 'drg. Bayu Nugroho', 'dokter_gigi'),
  ('00000000-0000-0000-0000-00000000e113', '00000000-0000-0000-0000-00000000e001', 'dr. Citra Halim', 'dokter_umum'),
  ('00000000-0000-0000-0000-00000000e114', '00000000-0000-0000-0000-00000000e002', 'dr. Dian Puspita', 'dokter_umum'),
  ('00000000-0000-0000-0000-00000000e115', '00000000-0000-0000-0000-00000000e003', 'dr. Eko Saputra', 'dokter_umum'),
  ('00000000-0000-0000-0000-00000000e116', '00000000-0000-0000-0000-00000000e005', 'dr. Fitri Andini', 'dokter_umum')
ON CONFLICT (id) DO NOTHING;

-- Today's clinic hours, 09:00-12:00 in 30-minute slots, matching the frames.
INSERT INTO practitioner_schedules
  (id, facility_id, practitioner_id, service_unit_id, schedule_date, start_time, end_time, slot_minutes, capacity_per_slot)
VALUES
  ('00000000-0000-0000-0000-00000000e121', '00000000-0000-0000-0000-00000000e000', '00000000-0000-0000-0000-00000000e111', '00000000-0000-0000-0000-00000000e101', CURRENT_DATE, '09:00', '12:00', 30, 1),
  ('00000000-0000-0000-0000-00000000e122', '00000000-0000-0000-0000-00000000e000', '00000000-0000-0000-0000-00000000e112', '00000000-0000-0000-0000-00000000e102', CURRENT_DATE, '09:00', '11:00', 30, 1),
  ('00000000-0000-0000-0000-00000000e123', '00000000-0000-0000-0000-00000000e001', '00000000-0000-0000-0000-00000000e113', '00000000-0000-0000-0000-00000000e103', CURRENT_DATE, '08:00', '11:00', 30, 1),
  ('00000000-0000-0000-0000-00000000e124', '00000000-0000-0000-0000-00000000e002', '00000000-0000-0000-0000-00000000e114', '00000000-0000-0000-0000-00000000e104', CURRENT_DATE, '09:00', '12:00', 30, 1),
  ('00000000-0000-0000-0000-00000000e125', '00000000-0000-0000-0000-00000000e003', '00000000-0000-0000-0000-00000000e115', '00000000-0000-0000-0000-00000000e105', CURRENT_DATE, '08:00', '11:00', 30, 1),
  ('00000000-0000-0000-0000-00000000e126', '00000000-0000-0000-0000-00000000e005', '00000000-0000-0000-0000-00000000e116', '00000000-0000-0000-0000-00000000e106', CURRENT_DATE, '09:00', '12:00', 30, 1)
ON CONFLICT (id) DO NOTHING;

-- A day of appointments across all four statuses, so every badge in the
-- vocabulary renders at least once and the transition controls have something
-- to act on.
--
-- Times are built in the CLINIC's timezone, not the server's. The admin UI
-- formats through `Intl` in the viewer's locale, so a naive
-- `(CURRENT_DATE + TIME '09:00')::timestamptz` on a UTC container stores 09:00
-- UTC and renders as 16.00 in Jakarta — a review row that reads as a broken
-- formatter when the data is what is wrong. `AT TIME ZONE 'Asia/Jakarta'` makes
-- a 09:00 slot a 09:00 slot on screen.
INSERT INTO appointments
  (id, facility_id, service_unit_id, practitioner_id, practitioner_schedule_id,
   appointment_time, status, patient_display_name, patient_phone, checkin_code, checkin_at, completed_at)
VALUES
  ('00000000-0000-0000-0000-00000000e201', '00000000-0000-0000-0000-00000000e000', '00000000-0000-0000-0000-00000000e101', '00000000-0000-0000-0000-00000000e111', '00000000-0000-0000-0000-00000000e121', ((CURRENT_DATE + TIME '09:00') AT TIME ZONE 'Asia/Jakarta'), 'scheduled',  'Siti Rahmawati',  '081200000021', 'K3M9PQ', NULL, NULL),
  ('00000000-0000-0000-0000-00000000e202', '00000000-0000-0000-0000-00000000e000', '00000000-0000-0000-0000-00000000e101', '00000000-0000-0000-0000-00000000e111', '00000000-0000-0000-0000-00000000e121', ((CURRENT_DATE + TIME '09:30') AT TIME ZONE 'Asia/Jakarta'), 'checked_in', 'Budi Santoso',    '081200000022', 'R7T2WX', now(), NULL),
  ('00000000-0000-0000-0000-00000000e203', '00000000-0000-0000-0000-00000000e000', '00000000-0000-0000-0000-00000000e102', '00000000-0000-0000-0000-00000000e112', '00000000-0000-0000-0000-00000000e122', ((CURRENT_DATE + TIME '10:00') AT TIME ZONE 'Asia/Jakarta'), 'completed',  'Dewi Lestari',    '081200000023', 'B5N8HK', now(), now()),
  ('00000000-0000-0000-0000-00000000e204', '00000000-0000-0000-0000-00000000e000', '00000000-0000-0000-0000-00000000e101', '00000000-0000-0000-0000-00000000e111', '00000000-0000-0000-0000-00000000e121', ((CURRENT_DATE + TIME '10:30') AT TIME ZONE 'Asia/Jakarta'), 'no_show',    'Agus Prasetyo',   '081200000024', 'Z4C6VD', NULL, NULL),
  ('00000000-0000-0000-0000-00000000e205', '00000000-0000-0000-0000-00000000e001', '00000000-0000-0000-0000-00000000e103', '00000000-0000-0000-0000-00000000e113', '00000000-0000-0000-0000-00000000e123', ((CURRENT_DATE + TIME '08:30') AT TIME ZONE 'Asia/Jakarta'), 'scheduled',  'Rina Marlina',    '081200000025', 'Q2F7LG', NULL, NULL),
  ('00000000-0000-0000-0000-00000000e206', '00000000-0000-0000-0000-00000000e001', '00000000-0000-0000-0000-00000000e103', '00000000-0000-0000-0000-00000000e113', '00000000-0000-0000-0000-00000000e123', ((CURRENT_DATE + TIME '09:00') AT TIME ZONE 'Asia/Jakarta'), 'queued',     'Joko Susilo',     '081200000026', 'M8P3RT', now(), NULL),
  ('00000000-0000-0000-0000-00000000e207', '00000000-0000-0000-0000-00000000e002', '00000000-0000-0000-0000-00000000e104', '00000000-0000-0000-0000-00000000e114', '00000000-0000-0000-0000-00000000e124', ((CURRENT_DATE + TIME '09:30') AT TIME ZONE 'Asia/Jakarta'), 'scheduled',  'Ani Yuliana',     '081200000027', 'V6K1DH', NULL, NULL),
  ('00000000-0000-0000-0000-00000000e208', '00000000-0000-0000-0000-00000000e002', '00000000-0000-0000-0000-00000000e104', '00000000-0000-0000-0000-00000000e114', '00000000-0000-0000-0000-00000000e124', ((CURRENT_DATE + TIME '11:00') AT TIME ZONE 'Asia/Jakarta'), 'cancelled',  'Hendra Gunawan',  '081200000028', 'T9S5BN', NULL, NULL),
  ('00000000-0000-0000-0000-00000000e209', '00000000-0000-0000-0000-00000000e003', '00000000-0000-0000-0000-00000000e105', '00000000-0000-0000-0000-00000000e115', '00000000-0000-0000-0000-00000000e125', ((CURRENT_DATE + TIME '08:30') AT TIME ZONE 'Asia/Jakarta'), 'scheduled',  'Maya Kusuma',     '081200000029', 'G4H7JC', NULL, NULL),
  ('00000000-0000-0000-0000-00000000e210', '00000000-0000-0000-0000-00000000e005', '00000000-0000-0000-0000-00000000e106', '00000000-0000-0000-0000-00000000e116', '00000000-0000-0000-0000-00000000e126', ((CURRENT_DATE + TIME '10:00') AT TIME ZONE 'Asia/Jakarta'), 'checked_in', 'Fajar Ramadhan',  '081200000030', 'Y1D8KP', now(), NULL)
ON CONFLICT (id) DO NOTHING;

-- A few outbox rows so the notification screen has real states to render.
--
-- The outbox stores a MASKED contact and a SHA-256 of the real one, never the
-- number itself — so the seed writes the same shape the worker does: a masked
-- display value and a 32-byte hash. Writing the plain number into
-- `recipient_contact_masked` would put a phone number in a table whose whole
-- design is that it does not hold one.
INSERT INTO notification_outbox
  (id, facility_id, channel, template_key, subject, body_template,
   recipient_type, recipient_contact_masked, recipient_contact_hash,
   status, attempt_count, last_error_code, related_resource_type, related_resource_id, created_at)
VALUES
  ('00000000-0000-0000-0000-00000000e301', '00000000-0000-0000-0000-00000000e000', 'whatsapp', 'appointment_reminder', 'Pengingat janji temu', 'Janji temu Anda pada {time} di {facility}.', 'patient', '0812****0021', sha256('081200000021'::bytea), 'pending',    0, NULL,                    'appointment', '00000000-0000-0000-0000-00000000e201', now()),
  ('00000000-0000-0000-0000-00000000e302', '00000000-0000-0000-0000-00000000e000', 'whatsapp', 'checkin_confirmed',    'Check-in berhasil',   'Nomor antrean Anda {queue_number}.',        'patient', '0812****0022', sha256('081200000022'::bytea), 'delivered',  1, NULL,                    'appointment', '00000000-0000-0000-0000-00000000e202', now()),
  ('00000000-0000-0000-0000-00000000e303', '00000000-0000-0000-0000-00000000e000', 'sms',      'appointment_reminder', 'Pengingat janji temu', 'Janji temu Anda pada {time} di {facility}.', 'patient', '0812****0023', sha256('081200000023'::bytea), 'failed',     3, 'provider_timeout',      'appointment', '00000000-0000-0000-0000-00000000e203', now()),
  ('00000000-0000-0000-0000-00000000e304', '00000000-0000-0000-0000-00000000e001', 'whatsapp', 'appointment_reminder', 'Pengingat janji temu', 'Janji temu Anda pada {time} di {facility}.', 'patient', '0812****0025', sha256('081200000025'::bytea), 'processing', 1, NULL,                    'appointment', '00000000-0000-0000-0000-00000000e205', now()),
  ('00000000-0000-0000-0000-00000000e305', '00000000-0000-0000-0000-00000000e002', 'sms',      'appointment_cancelled','Janji temu dibatalkan','Janji temu Anda pada {time} telah dibatalkan.', 'patient', '0812****0028', sha256('081200000028'::bytea), 'cancelled', 0, NULL,             'appointment', '00000000-0000-0000-0000-00000000e208', now())
ON CONFLICT (id) DO NOTHING;

COMMIT;
