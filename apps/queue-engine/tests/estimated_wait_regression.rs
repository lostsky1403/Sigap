//! Regression tests for `estimated_wait_minutes` and the record signature.
//!
//! The old behaviour returned a hardcoded 25 minutes for every ticket and a
//! plain SHA-256 hash described as "tamper-proof proof". The engine now
//! computes the estimate from the real queue (tickets ahead x measured
//! average service time, -1 when there is no history) and signs records
//! with HMAC-SHA256 keyed by the server secret. These tests pin that
//! behaviour:
//!
//!   1. `estimated_wait_is_minus_one_without_history` — first ticket of a
//!      facility with no completed ticket yet: no history to average over,
//!      so the estimate is -1 ("no estimate"), never an invented number.
//!   2. `estimated_wait_is_ahead_times_average_service_time` — two completed
//!      tickets (10 minutes each) plus one waiting ticket ahead: the next
//!      ticket waits for 1 x 10 minutes.
//!   3. `signature_is_hmac_not_plain_sha256` — the stored signature equals
//!      HMAC-SHA256(secret, phone|facility|number|formatted|visit_time)
//!      recomputed with the test secret, and differs from the plain SHA-256
//!      of the same input.
//!
//! Requires PostgreSQL with sigap schema; skips gracefully if DB is unreachable.
//! Each test uses its own facility UUID so tests can run in parallel.

use std::env;

use hmac::{Hmac, Mac};
use sha2::{Digest, Sha256};
use sigap_queue_engine::queue_engine::{GenerateQueueRequest, PatientInfo};

/// Signing secret for these tests (same role as SIGAP_SIGNING_SECRET in
/// production). Never use this value outside tests.
const TEST_SIGNING_SECRET: &[u8] = b"test-signing-secret-do-not-use-in-production";

/// Deterministic facility UUID for the no-history test.
const NO_HISTORY_FACILITY_ID: &str = "e4eebc99-9c0b-4ef8-bb6d-6bb9bd380a14";
/// Deterministic facility UUID for the ahead-times-average test.
const AHEAD_FACILITY_ID: &str = "c2eebc99-9c0b-4ef8-bb6d-6bb9bd380a13";
/// Deterministic facility UUID for the signature test.
const SIGNATURE_FACILITY_ID: &str = "d3eebc99-9c0b-4ef8-bb6d-6bb9bd380a15";

async fn connect() -> Option<sqlx::PgPool> {
    let database_url = match env::var("DATABASE_URL") {
        Ok(url) => url,
        Err(_) => {
            eprintln!("SKIP: DATABASE_URL not set; estimated_wait regression requires a database connection");
            return None;
        }
    };
    match sqlx::PgPool::connect(&database_url).await {
        Ok(p) => Some(p),
        Err(e) => {
            eprintln!(
                "SKIP: DB unreachable ({}); estimated_wait regression cannot run without DB",
                e
            );
            None
        }
    }
}

async fn seed_facility(pool: &sqlx::PgPool, facility_id: uuid::Uuid, name: &str) {
    sqlx::query(
        "INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, is_active, short_code)
         VALUES ($1, $2, 'puskesmas', 'Jl. Test No. 3', 'TestKec', 'TestKota', 'TestProv', '021-000002', 100, 50, true, 'TST')
         ON CONFLICT (id) DO NOTHING",
    )
    .bind(facility_id)
    .bind(name)
    .execute(pool)
    .await
    .expect("failed to seed test facility");
}

async fn clean_facility(pool: &sqlx::PgPool, facility_id: uuid::Uuid) {
    for table in ["queue_tickets", "medical_records", "daily_queue_counters"] {
        sqlx::query(&format!("DELETE FROM {} WHERE facility_id = $1", table))
            .bind(facility_id)
            .execute(pool)
            .await
            .unwrap_or_else(|e| panic!("failed to clean test {}: {}", table, e));
    }
}

/// The WIB service day containing `now` (UTC+7, no DST). Mirrors the
/// engine's service_day_wib; the seed rows below must land on the same day
/// the engine assigns the generated ticket to.
fn wib_today() -> chrono::NaiveDate {
    (chrono::Utc::now() + chrono::Duration::hours(7)).date_naive()
}

async fn generate(
    pool: &sqlx::PgPool,
    facility_id: &str,
    phone: &str,
) -> sigap_queue_engine::queue_engine::GenerateQueueResponse {
    let req = GenerateQueueRequest {
        facility_id: facility_id.to_string(),
        patient: Some(PatientInfo {
            full_name: "Regression Test Patient".to_string(),
            phone: phone.to_string(),
            gender: Some("L".to_string()),
            date_of_birth: Some("1990-01-01".to_string()),
        }),
    };
    sigap_queue_engine::engine::queue::generate_queue_number_tx(pool, req, TEST_SIGNING_SECRET)
        .await
        .expect("generate_queue_number_tx should succeed when DB is available")
}

#[tokio::test]
async fn estimated_wait_is_minus_one_without_history() {
    let Some(pool) = connect().await else { return };
    let facility_uuid = uuid::Uuid::parse_str(NO_HISTORY_FACILITY_ID).expect("valid UUID");

    clean_facility(&pool, facility_uuid).await;
    seed_facility(&pool, facility_uuid, "Test Facility No History").await;

    let resp = generate(&pool, NO_HISTORY_FACILITY_ID, "081299988877").await;

    assert_eq!(
        resp.estimated_wait_minutes, -1,
        "with no completed ticket to average over, the estimate must be -1 (no estimate), not an invented number"
    );
    assert_eq!(resp.formatted_number, "TST-0001");
    assert_eq!(resp.signature.len(), 64, "signature must be a 64-char hex digest");
    assert!(resp.signature.chars().all(|c| c.is_ascii_hexdigit()));
}

#[tokio::test]
async fn estimated_wait_is_ahead_times_average_service_time() {
    let Some(pool) = connect().await else { return };
    let facility_uuid = uuid::Uuid::parse_str(AHEAD_FACILITY_ID).expect("valid UUID");

    clean_facility(&pool, facility_uuid).await;
    seed_facility(&pool, facility_uuid, "Test Facility Ahead").await;

    let today = wib_today();
    let now = chrono::Utc::now();

    // Two patients with completed tickets: each was called and completed
    // exactly 10 minutes apart, so the measured average service time is
    // 10 minutes. One more patient is still waiting ahead of the ticket
    // the test is about to generate.
    for (i, phone) in ["081299988881", "081299988882"].iter().enumerate() {
        let patient_id: uuid::Uuid = sqlx::query_scalar(
            "INSERT INTO patients (full_name, phone) VALUES ($1, $2)
             ON CONFLICT (phone) DO UPDATE SET full_name = EXCLUDED.full_name
             RETURNING id",
        )
        .bind(format!("Completed Patient {}", i + 1))
        .bind(*phone)
        .fetch_one(&pool)
        .await
        .expect("failed to seed completed patient");

        sqlx::query(
            "INSERT INTO queue_tickets (facility_id, patient_id, queue_number, formatted_number, status, service_date, registered_at, called_at, completed_at)
             VALUES ($1, $2, $3, $4, 'completed', $5, $6, $7, $8)",
        )
        .bind(facility_uuid)
        .bind(patient_id)
        .bind((i + 1) as i32)
        .bind(format!("TST-{:04}", i + 1))
        .bind(today)
        .bind(now - chrono::Duration::minutes(20))
        .bind(now - chrono::Duration::minutes(20))
        .bind(now - chrono::Duration::minutes(10))
        .execute(&pool)
        .await
        .expect("failed to seed completed ticket");
    }

    let waiting_patient: uuid::Uuid = sqlx::query_scalar(
        "INSERT INTO patients (full_name, phone) VALUES ('Waiting Patient', '081299988883')
         ON CONFLICT (phone) DO UPDATE SET full_name = EXCLUDED.full_name
         RETURNING id",
    )
    .fetch_one(&pool)
    .await
    .expect("failed to seed waiting patient");

    sqlx::query(
        "INSERT INTO queue_tickets (facility_id, patient_id, queue_number, formatted_number, status, service_date, registered_at)
         VALUES ($1, $2, 3, 'TST-0003', 'waiting', $3, $4)",
    )
    .bind(facility_uuid)
    .bind(waiting_patient)
    .bind(today)
    .bind(now - chrono::Duration::minutes(5))
    .execute(&pool)
    .await
    .expect("failed to seed waiting ticket");

    // The counter must agree with the seeded tickets, or the generated
    // ticket would collide with number 1..=3 under migration 0015's UNIQUE.
    sqlx::query(
        "INSERT INTO daily_queue_counters (facility_id, date, last_number) VALUES ($1, $2, 3)
         ON CONFLICT (facility_id, date) DO UPDATE SET last_number = 3",
    )
    .bind(facility_uuid)
    .bind(today)
    .execute(&pool)
    .await
    .expect("failed to seed counter");

    let resp = generate(&pool, AHEAD_FACILITY_ID, "081299988884").await;

    assert_eq!(
        resp.formatted_number, "TST-0004",
        "the generated ticket follows the three seeded ones"
    );
    assert_eq!(
        resp.estimated_wait_minutes, 10,
        "1 ticket ahead x 10-minute average service time = 10 minutes"
    );
}

#[tokio::test]
async fn signature_is_hmac_not_plain_sha256() {
    let Some(pool) = connect().await else { return };
    let facility_uuid = uuid::Uuid::parse_str(SIGNATURE_FACILITY_ID).expect("valid UUID");

    clean_facility(&pool, facility_uuid).await;
    seed_facility(&pool, facility_uuid, "Test Facility Signature").await;

    let phone = "081299988885";
    let resp = generate(&pool, SIGNATURE_FACILITY_ID, phone).await;

    // The response's registered_at is the exact RFC3339 string the engine
    // signed (visit_time.to_rfc3339()), and the queue number is the suffix
    // of the formatted number — so the test can recompute the signature
    // input byte-for-byte without guessing the engine's clock.
    let queue_number: i32 = resp
        .formatted_number
        .strip_prefix("TST-")
        .expect("formatted number must start with TST-")
        .parse()
        .expect("formatted number suffix must be numeric");
    let sig_input = format!(
        "{}|{}|{}|{}|{}",
        phone, SIGNATURE_FACILITY_ID, queue_number, resp.formatted_number, resp.registered_at
    );

    let mut mac = Hmac::<Sha256>::new_from_slice(TEST_SIGNING_SECRET)
        .expect("HMAC accepts a key of any length");
    mac.update(sig_input.as_bytes());
    let expected_hmac = format!("{:x}", mac.finalize().into_bytes());

    assert_eq!(
        resp.signature, expected_hmac,
        "signature must be HMAC-SHA256 with the server secret"
    );

    let mut hasher = Sha256::new();
    hasher.update(sig_input.as_bytes());
    let plain_sha256 = format!("{:x}", hasher.finalize());

    assert_ne!(
        resp.signature, plain_sha256,
        "signature must NOT be the plain SHA-256 anyone could recompute"
    );
}
