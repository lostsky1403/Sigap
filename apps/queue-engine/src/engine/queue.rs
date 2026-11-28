use sqlx::{PgPool, Postgres, Transaction};
use std::time::Instant;
use tonic::Status;
use uuid::Uuid;

use hmac::{Hmac, Mac};
use sha2::Sha256;

use crate::queue_engine::{GenerateQueueRequest, GenerateQueueResponse};

const DAILY_QUEUE_LIMIT: i32 = 300;

/// The WIB (Asia/Jakarta, UTC+7) fixed offset. Indonesia runs no daylight-saving
/// transition, so a fixed offset is exact and avoids a runtime tzdata dependency.
/// This is the single queue-engine definition of the service day; it must match
/// the Go `serviceDayWIB` (apps/api/internal/handler/admin.go).
fn service_day_wib_offset() -> chrono::FixedOffset {
    chrono::FixedOffset::east_opt(7 * 3600).expect("WIB offset (UTC+7) is always valid")
}

/// Returns the WIB calendar day containing the given UTC instant. The service day
/// keys `daily_queue_counters` and bounds `DAILY_QUEUE_LIMIT`, so the counter and
/// cap day is the facility-local WIB day — not the UTC day — matching the read path.
/// The stored `visit_time`/`registered_at` stays the unmodified UTC instant.
fn service_day_wib(instant: chrono::DateTime<chrono::Utc>) -> chrono::NaiveDate {
    instant.with_timezone(&service_day_wib_offset()).date_naive()
}

/// Core atomic transaction for generating a queue number.
/// Uses a single atomic INSERT ... ON CONFLICT DO UPDATE ... RETURNING on
/// daily_queue_counters (PK (facility_id, date)) to guarantee exactly-once
/// increment even under heavy concurrent load (zero double-booking).
///
/// `signing_secret` is the server HMAC key (SIGAP_SIGNING_SECRET) used for
/// the medical-record integrity checksum. It is passed in, never read from
/// the environment here, so tests can supply their own key.
pub async fn generate_queue_number_tx(
    pool: &PgPool,
    req: GenerateQueueRequest,
    signing_secret: &[u8],
) -> Result<GenerateQueueResponse, Status> {
    let facility_id = Uuid::parse_str(&req.facility_id)
        .map_err(|_| Status::invalid_argument("facility_id bukan UUID yang valid"))?;

    let phone = req.patient.as_ref().map(|p| p.phone.as_str()).unwrap_or("");
    let full_name = req
        .patient
        .as_ref()
        .map(|p| p.full_name.as_str())
        .unwrap_or("");

    if phone.is_empty() || full_name.is_empty() {
        return Err(Status::invalid_argument("phone dan full_name wajib diisi"));
    }

    let start = Instant::now();

    let mut tx: Transaction<'_, Postgres> = pool
        .begin()
        .await
        .map_err(|e| Status::internal(format!("gagal memulai transaksi: {}", e)))?;

    // 1. Validate facility (active)
    let facility_row: Option<(Uuid, Option<String>, bool)> =
        sqlx::query_as("SELECT id, short_code, is_active FROM facilities WHERE id = $1")
            .bind(facility_id)
            .fetch_optional(&mut *tx)
            .await
            .map_err(|e| Status::internal(format!("query facility gagal: {}", e)))?;

    let (_fac_id, short_code_opt, _is_active) = match facility_row {
        Some(row) if row.2 => row,
        Some(_) => return Err(Status::not_found("Fasilitas tidak aktif")),
        None => return Err(Status::not_found("Fasilitas tidak ditemukan")),
    };
    let short_code = short_code_opt.unwrap_or_else(|| "FSK".to_string());

    // 2. Get or create patient by phone (unique)
    sqlx::query(
        r#"
        INSERT INTO patients (full_name, phone)
        VALUES ($1, $2)
        ON CONFLICT (phone) DO UPDATE SET full_name = EXCLUDED.full_name
        "#,
    )
    .bind(full_name)
    .bind(phone)
    .execute(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("upsert pasien gagal: {}", e)))?;

    let patient_id: Uuid = sqlx::query_scalar("SELECT id FROM patients WHERE phone = $1")
        .bind(phone)
        .fetch_one(&mut *tx)
        .await
        .map_err(|e| Status::internal(format!("select pasien gagal: {}", e)))?;

    // 3. Atomic daily counter (THE critical part for Zero Double-Booking).
    // One INSERT ... ON CONFLICT DO UPDATE ... RETURNING creates-or-increments
    // the day's counter under the row lock Postgres takes on the conflicting
    // (facility_id, date) row. The old SELECT ... FOR UPDATE + INSERT pattern
    // could not lock a row that did not exist yet, so the first two concurrent
    // requests of a facility-day both INSERTed and one failed with a PK
    // violation (Status::internal). The counter/cap day is the WIB service
    // day (Asia/Jakarta, UTC+7), so the sequence resets at WIB midnight
    // (00:00 WIB), not at UTC midnight (07:00 WIB).
    let today = service_day_wib(chrono::Utc::now());

    let next_number: i32 = sqlx::query_scalar(
        r#"
        INSERT INTO daily_queue_counters (facility_id, date, last_number)
        VALUES ($1, $2, 1)
        ON CONFLICT (facility_id, date)
        DO UPDATE SET last_number = daily_queue_counters.last_number + 1
        RETURNING last_number
        "#,
    )
    .bind(facility_id)
    .bind(today)
    .fetch_one(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("counter antrean gagal: {}", e)))?;

    if next_number > DAILY_QUEUE_LIMIT {
        return Err(Status::resource_exhausted(
            "Batas antrean harian untuk fasilitas ini sudah tercapai (300). Silakan coba lagi besok.",
        ));
    }

    let formatted = format!("{}-{:04}", short_code, next_number);

    // Record-integrity checksum: HMAC-SHA256 keyed by the server signing
    // secret (SIGAP_SIGNING_SECRET). Only the server can produce or verify
    // it — that is what makes the checksum unforgeable by clients, and the
    // absence of a public key is what makes it NOT a digital signature.
    // It replaces the old plain SHA-256 hash, which anyone could recompute
    // and which was wrongly described as "tamper-proof proof".
    let visit_time = chrono::Utc::now();
    let sig_input = format!(
        "{}|{}|{}|{}|{}",
        phone,
        facility_id,
        next_number,
        formatted,
        visit_time.to_rfc3339()
    );
    let signature = compute_record_signature(signing_secret, &sig_input);

    // 4. Create the ticket (client-supplied UUID). service_date is the same
    // WIB day the counter just used, so the ticket and its counter row can
    // never disagree about which day they belong to (migration 0015 enforces
    // UNIQUE (facility_id, service_date, queue_number) as a backstop).
    let ticket_id = Uuid::new_v4();
    sqlx::query(
        r#"
        INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status, service_date)
        VALUES ($1, $2, $3, $4, $5, 'waiting', $6)
        "#
    )
    .bind(ticket_id)
    .bind(facility_id)
    .bind(patient_id)
    .bind(next_number)
    .bind(formatted.clone())
    .bind(today)
    .execute(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("insert tiket gagal: {}", e)))?;

    // 4b. Wait estimate from the real queue: how many tickets are still
    // ahead of this one today, times the facility's measured average
    // service time. When no completed ticket exists yet there is no
    // history to average over, so the engine reports -1 ("no estimate")
    // instead of inventing a number. The new ticket is 'waiting' with
    // number `next_number`, so the ahead-count (numbers < next_number)
    // never includes it, and the average excludes it too (its called_at
    // is NULL).
    let estimated_wait_minutes =
        estimate_wait_minutes(&mut tx, facility_id, today, next_number).await?;

    // 5. Insert the medical record (Health Wallet) with the HMAC
    // record-integrity checksum. The checksum proves the row was written
    // by this server; it is not a tamper-proof proof for third parties.
    sqlx::query(
        r#"
        INSERT INTO medical_records (patient_phone, facility_id, queue_number, formatted_number, signature, visit_time)
        VALUES ($1, $2, $3, $4, $5, $6)
        "#
    )
    .bind(phone)
    .bind(facility_id)
    .bind(next_number)
    .bind(formatted.clone())
    .bind(&signature)
    .bind(visit_time)
    .execute(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("insert medical record gagal: {}", e)))?;

    // Commit
    tx.commit()
        .await
        .map_err(|e| Status::internal(format!("commit transaksi gagal: {}", e)))?;

    let processing_time_micros = start.elapsed().as_micros() as i64;

    Ok(GenerateQueueResponse {
        ticket_id: ticket_id.to_string(),
        formatted_number: formatted,
        status: "waiting".to_string(),
        registered_at: visit_time.to_rfc3339(),
        estimated_wait_minutes,
        processing_time_micros,
        signature,
    })
}

/// HMAC-SHA256 record-integrity checksum for one medical record. The key is
/// the server signing secret (SIGAP_SIGNING_SECRET); verification needs the
/// same secret, so the checksum is meaningful only to this server.
fn compute_record_signature(signing_secret: &[u8], input: &str) -> String {
    let mut mac = Hmac::<Sha256>::new_from_slice(signing_secret)
        .expect("HMAC accepts a key of any length");
    mac.update(input.as_bytes());
    format!("{:x}", mac.finalize().into_bytes())
}

/// Computes the wait estimate for a ticket that just took `ticket_number`
/// today: the number of tickets still ahead of it (waiting, called or in
/// service with a smaller number) times the facility's average measured
/// service time in minutes, rounded up. The average comes from tickets
/// that were both called and completed in the last 30 service days; a
/// ticket that was never called or never completed has no measured service
/// time and is excluded. Returns -1 when no completed ticket exists, so
/// the caller reports "no estimate" instead of a fabricated number.
//
// The AVG is cast to float8 in SQL: Postgres AVG over an interval-derived
// expression returns NUMERIC, which sqlx cannot decode into f64.
#[allow(clippy::result_large_err)]
async fn estimate_wait_minutes(
    tx: &mut Transaction<'_, Postgres>,
    facility_id: Uuid,
    today: chrono::NaiveDate,
    ticket_number: i32,
) -> Result<i32, Status> {
    let ahead: i64 = sqlx::query_scalar(
        r#"
        SELECT COUNT(*)
        FROM queue_tickets
        WHERE facility_id = $1
          AND service_date = $2
          AND queue_number < $3
          AND status IN ('waiting', 'called', 'in_service')
        "#,
    )
    .bind(facility_id)
    .bind(today)
    .bind(ticket_number)
    .fetch_one(&mut **tx)
    .await
    .map_err(|e| Status::internal(format!("hitung antrean di depan gagal: {}", e)))?;

    let avg_minutes: Option<f64> = sqlx::query_scalar(
        r#"
        SELECT AVG(EXTRACT(EPOCH FROM (completed_at - called_at)) / 60.0)::float8
        FROM queue_tickets
        WHERE facility_id = $1
          AND service_date >= $2 - INTERVAL '30 days'
          AND called_at IS NOT NULL
          AND completed_at IS NOT NULL
          AND completed_at > called_at
        "#,
    )
    .bind(facility_id)
    .bind(today)
    .fetch_one(&mut **tx)
    .await
    .map_err(|e| Status::internal(format!("hitung rata-rata layanan gagal: {}", e)))?;

    let Some(avg) = avg_minutes.filter(|v| *v > 0.0) else {
        return Ok(-1);
    };
    Ok((ahead as f64 * avg).ceil() as i32)
}

#[cfg(test)]
mod tests {
    use super::{service_day_wib, service_day_wib_offset};
    use chrono::{NaiveDate, TimeZone, Utc};

    #[test]
    fn service_day_wib_does_not_flip_at_utc_midnight() {
        // UTC 23:59:59 and 00:00:01 straddle UTC midnight (07:00 WIB). Both map
        // to the same WIB calendar day, so the counter key and DAILY_QUEUE_LIMIT
        // must NOT reset mid-morning WIB. This is the closed UTC-vs-WIB divergence.
        let before = Utc.with_ymd_and_hms(2026, 11, 8, 23, 59, 59).unwrap();
        let after = Utc.with_ymd_and_hms(2026, 11, 9, 0, 0, 1).unwrap();

        let day_before = service_day_wib(before);
        let day_after = service_day_wib(after);

        assert_eq!(
            day_before, day_after,
            "UTC midnight must not shift the WIB service day"
        );
        assert_eq!(day_before, NaiveDate::from_ymd_opt(2026, 11, 9).unwrap());
    }

    #[test]
    fn service_day_wib_flips_at_wib_midnight() {
        // WIB midnight (00:00 WIB = 17:00 UTC the prior day) is the true service-day
        // boundary: 23:59:59 WIB on 2026-11-08 and 00:00:01 WIB on 2026-11-09 are two
        // different service days.
        let offset = service_day_wib_offset();
        let before = offset
            .with_ymd_and_hms(2026, 11, 8, 23, 59, 59)
            .unwrap()
            .with_timezone(&Utc);
        let after = offset
            .with_ymd_and_hms(2026, 11, 9, 0, 0, 1)
            .unwrap()
            .with_timezone(&Utc);

        let day_before = service_day_wib(before);
        let day_after = service_day_wib(after);

        assert_ne!(
            day_before, day_after,
            "WIB midnight must flip the service day"
        );
        assert_eq!(day_before, NaiveDate::from_ymd_opt(2026, 11, 8).unwrap());
        assert_eq!(day_after, NaiveDate::from_ymd_opt(2026, 11, 9).unwrap());
    }

    #[test]
    fn service_day_wib_midnight_exact_boundary() {
        // Pin the exact boundary: 23:59:59 WIB vs 00:00:00 WIB (the first instant
        // of the next day) are two different counter days. The flip test above used
        // 00:00:01; this proves 00:00:00 itself already belongs to the new day.
        let offset = service_day_wib_offset();
        let before = offset
            .with_ymd_and_hms(2026, 11, 8, 23, 59, 59)
            .unwrap()
            .with_timezone(&Utc);
        let after = offset
            .with_ymd_and_hms(2026, 11, 9, 0, 0, 0)
            .unwrap()
            .with_timezone(&Utc);

        assert_ne!(
            service_day_wib(before),
            service_day_wib(after),
            "00:00:00 WIB must already be the next service day"
        );
        assert_eq!(
            service_day_wib(before),
            NaiveDate::from_ymd_opt(2026, 11, 8).unwrap()
        );
        assert_eq!(
            service_day_wib(after),
            NaiveDate::from_ymd_opt(2026, 11, 9).unwrap()
        );
    }

    #[test]
    fn service_day_wib_utc_2300_is_next_wib_day() {
        // 2026-11-08 23:00:00 UTC is 2026-11-09 06:00:00 WIB: the WIB calendar day
        // is the NEXT day relative to the UTC civil day, so the counter key is
        // 2026-11-09, never 2026-11-08.
        let instant = Utc.with_ymd_and_hms(2026, 11, 8, 23, 0, 0).unwrap();

        let day = service_day_wib(instant);

        assert_eq!(day, NaiveDate::from_ymd_opt(2026, 11, 9).unwrap());
        assert_ne!(day, NaiveDate::from_ymd_opt(2026, 11, 8).unwrap());
    }

    #[test]
    fn service_day_wib_multiple_facilities_share_day() {
        // All facilities run WIB; the engine's day is computed from the instant
        // alone (no per-facility timezone is in scope today). Two facilities seeing
        // the same instant must derive the same counter day, even at the sharpest
        // boundary. This guards against a future accidental facility_id branch.
        let instant = Utc.with_ymd_and_hms(2026, 11, 8, 23, 59, 59).unwrap(); // 06:59:59 WIB Nov 9

        let facility_day = |_facility_id: &str| service_day_wib(instant);

        let first = facility_day("facility-a");
        let second = facility_day("facility-b");

        assert_eq!(first, second);
        assert_eq!(first, NaiveDate::from_ymd_opt(2026, 11, 9).unwrap());
    }

    #[test]
    fn service_day_wib_historical_ticket_own_day() {
        // A ticket minted days earlier must resolve to its OWN WIB day, not today.
        let past = Utc.with_ymd_and_hms(2026, 11, 6, 3, 15, 30).unwrap(); // 10:15:30 WIB
        let today_ref = Utc.with_ymd_and_hms(2026, 11, 9, 3, 0, 0).unwrap();

        let day = service_day_wib(past);

        assert_eq!(day, NaiveDate::from_ymd_opt(2026, 11, 6).unwrap());
        assert_ne!(day, service_day_wib(today_ref));
    }

    #[test]
    fn service_day_wib_agrees_across_utc_and_wib_construction() {
        // The same instant expressed once in UTC (23:00Z) and once via the WIB
        // offset (06:00 WIB the next day) must map to one service day. This mirrors
        // the Go allocation↔listing invariant: the counter and the read path key off
        // a single WIB date, so otherwise-expressed instants must not drift.
        let offset = service_day_wib_offset();
        let as_utc = Utc.with_ymd_and_hms(2026, 11, 8, 23, 0, 0).unwrap(); // 06:00 WIB Nov 9
        let as_wib = offset
            .with_ymd_and_hms(2026, 11, 9, 6, 0, 0)
            .unwrap()
            .with_timezone(&Utc);

        assert_eq!(as_utc, as_wib);
        assert_eq!(
            service_day_wib(as_utc),
            NaiveDate::from_ymd_opt(2026, 11, 9).unwrap()
        );
    }
}
