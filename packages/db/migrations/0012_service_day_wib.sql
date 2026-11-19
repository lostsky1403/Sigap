-- 0012_service_day_wib.sql
-- Make the service day genuinely WIB for the booking-per-day guard.
--
-- Context (design/service-day-queue-policy.md §8): the read path (queue board +
-- Ringkasan) already scopes tickets to the WIB calendar day. The write path
-- previously used the UTC day (chrono::Utc in the Rust queue engine and
-- `registered_at::date = CURRENT_DATE` in the two Go fallback allocators, where
-- the session TimeZone is UTC), and the guard below used the session-tz-dependent
-- `::date` cast (declared IMMUTABLE but actually STABLE). This migration closes the
-- divergence for the one DB-enforced invariant WITHOUT rewriting any historical
-- `appointment_time`/`registered_at` instant.
--
--   1. `appointment_day(timestamptz)` becomes a genuinely immutable WIB day:
--      `(($1 AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date`. The `AT TIME ZONE
--      'UTC'` makes it session-timezone-independent (WIB = UTC+7, no DST).
--   2. `uq_active_booking_per_patient_day` is recreated on the new expression.
--   3. Before recreating, a collision guard counts groups that would violate the
--      unique index under the WIB-day definition. If any exist, NOTHING is changed
--      and a NOTICE reports the count; an operator must resolve by hand first.
--
-- Additive and non-destructive: no stored instant is rewritten. The allocation-side
-- change is not in the database at all (it is the Go `serviceDayRange` helper and the
-- Rust `service_day_wib` helper). Forward-only; does not edit 0007.
--
-- Run with:
--   psql $DATABASE_URL -v ON_ERROR_STOP=1 -f packages/db/migrations/0012_service_day_wib.sql

DO $migration$
DECLARE
  collision_count bigint;
BEGIN
  SELECT count(*) INTO collision_count
  FROM (
    SELECT facility_id, patient_phone,
           (appointment_time AT TIME ZONE 'UTC' + INTERVAL '7 hours')::date AS wib_day
    FROM appointments
    WHERE status NOT IN ('cancelled', 'no_show')
    GROUP BY facility_id, patient_phone,
             (appointment_time AT TIME ZONE 'UTC' + INTERVAL '7 hours')::date
    HAVING count(*) > 1
  ) AS collisions;

  IF collision_count = 0 THEN
    EXECUTE $fn$
      CREATE OR REPLACE FUNCTION appointment_day(timestamptz) RETURNS date AS $body$
        SELECT (($1 AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date
      $body$ LANGUAGE sql IMMUTABLE PARALLEL SAFE
    $fn$;

    EXECUTE $drop$
      DROP INDEX IF EXISTS uq_active_booking_per_patient_day
    $drop$;

    EXECUTE $idx$
      CREATE UNIQUE INDEX uq_active_booking_per_patient_day
        ON appointments (facility_id, patient_phone, appointment_day(appointment_time))
        WHERE status NOT IN ('cancelled', 'no_show')
    $idx$;

    RAISE NOTICE '0012_service_day_wib: no WIB-day collisions; appointment_day redefined to WIB and uq_active_booking_per_patient_day recreated.';
  ELSE
    RAISE NOTICE '0012_service_day_wib: % group(s) would violate uq_active_booking_per_patient_day under the WIB-day definition; leaving appointment_day and the index unchanged.', collision_count;
  END IF;
END $migration$;