//! Concurrency guardrail: N concurrent queue requests must produce
//! unique sequential numbers (zero double-booking).
//!
//! Run with: `cargo test --test concurrency_guardrail -- --test-threads=16`
//!
//! This test requires a running PostgreSQL with the sigap schema.
//! DATABASE_URL must be set; the test skips cleanly when it is not.
//!
//! Skipped gracefully if the database is unreachable so CI without DB still passes.
//!
//! The second test (`first_ticket_of_the_day_is_race_free_from_empty_counter`)
//! fires parallel first-ticket requests from an empty counter — the exact
//! case the old SELECT ... FOR UPDATE + INSERT pattern lost — and probes
//! migration 0015's UNIQUE backstop with a direct duplicate insert.

use std::collections::HashSet;
use std::env;

// Re-export the engine module path via the crate
use sigap_queue_engine::queue_engine::{GenerateQueueRequest, PatientInfo};

/// Deterministic facility UUID for this test. The INSERT uses
/// ON CONFLICT DO NOTHING so repeated runs are idempotent.
const TEST_FACILITY_ID: &str = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11";

/// Deterministic facility UUID for the first-ticket race test. Different from
/// TEST_FACILITY_ID so the two tests can run in parallel in one binary
/// without sharing any counter, ticket or patient row.
const FIRST_DAY_FACILITY_ID: &str = "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a12";

/// Signing secret for the HMAC record-integrity checksum in these tests.
/// The engine takes the secret as an argument, so tests supply their own.
/// Never use this value outside tests.
const TEST_SIGNING_SECRET: &[u8] = b"test-signing-secret-do-not-use-in-production";

#[tokio::test]
async fn concurrent_queue_requests_produce_unique_numbers() {
    let database_url = match env::var("DATABASE_URL") {
        Ok(url) => url,
        Err(_) => {
            eprintln!(
                "SKIP: DATABASE_URL not set; concurrency guardrail requires a database connection"
            );
            return;
        }
    };

    let pool = match sqlx::PgPool::connect(&database_url).await {
        Ok(p) => p,
        Err(e) => {
            eprintln!(
                "SKIP: DB unreachable ({}); concurrency guardrail cannot run without DB",
                e
            );
            return; // graceful skip when DB is not available
        }
    };

    // Ensure a deterministic test facility exists. Using ON CONFLICT
    // DO NOTHING makes this idempotent across repeated test runs.
    sqlx::query(
        "INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, is_active, short_code)
         VALUES ($1, 'Test Facility Concurrency', 'puskesmas', 'Jl. Test No. 1', 'TestKec', 'TestKota', 'TestProv', '021-000000', 100, 50, true, 'TST')
         ON CONFLICT (id) DO NOTHING",
    )
    .bind(uuid::Uuid::parse_str(TEST_FACILITY_ID).expect("valid UUID"))
    .execute(&pool)
    .await
    .expect("failed to seed test facility");

    let facility_id = TEST_FACILITY_ID.to_string();

    let n = 10usize;
    let mut handles = Vec::with_capacity(n);

    for i in 0..n {
        let pool = pool.clone();
        let fid = facility_id.clone();
        let handle = tokio::spawn(async move {
            let req = GenerateQueueRequest {
                facility_id: fid,
                patient: Some(PatientInfo {
                    full_name: format!("Pasien Concurrent {}", i),
                    phone: format!("081234567{:03}", i),
                    gender: Some("L".to_string()),
                    date_of_birth: Some("1990-01-01".to_string()),
                }),
            };
            let resp =
                sigap_queue_engine::engine::queue::generate_queue_number_tx(&pool, req, TEST_SIGNING_SECRET).await;
            resp
        });
        handles.push(handle);
    }

    let mut numbers = HashSet::new();
    for handle in handles {
        let resp = handle
            .await
            .expect("task join failed")
            .expect("generate_queue_number_tx returned Err");
        let num = resp.formatted_number;
        assert!(
            numbers.insert(num.clone()),
            "duplicate queue number detected: {}. Zero-double-booking violated!",
            num
        );
    }

    assert_eq!(
        numbers.len(),
        n,
        "expected {} unique numbers, got {}",
        n,
        numbers.len()
    );
}

#[tokio::test]
async fn first_ticket_of_the_day_is_race_free_from_empty_counter() {
    let database_url = match env::var("DATABASE_URL") {
        Ok(url) => url,
        Err(_) => {
            eprintln!(
                "SKIP: DATABASE_URL not set; first-ticket race test requires a database connection"
            );
            return;
        }
    };

    let pool = match sqlx::PgPool::connect(&database_url).await {
        Ok(p) => p,
        Err(e) => {
            eprintln!(
                "SKIP: DB unreachable ({}); first-ticket race test cannot run without DB",
                e
            );
            return; // graceful skip when DB is not available
        }
    };

    let facility_id = FIRST_DAY_FACILITY_ID.to_string();
    let facility_uuid = uuid::Uuid::parse_str(&facility_id).expect("valid UUID");

    // Start from a genuinely empty counter: delete this facility's tickets,
    // medical records and counter row first. Scoped to the deterministic test
    // facility only, so repeated runs are idempotent. Without this cleanup a
    // second run would continue the old counter instead of testing the
    // first-ticket-of-the-day race.
    sqlx::query("DELETE FROM queue_tickets WHERE facility_id = $1")
        .bind(facility_uuid)
        .execute(&pool)
        .await
        .expect("failed to clean test tickets");
    sqlx::query("DELETE FROM medical_records WHERE facility_id = $1")
        .bind(facility_uuid)
        .execute(&pool)
        .await
        .expect("failed to clean test medical records");
    sqlx::query("DELETE FROM daily_queue_counters WHERE facility_id = $1")
        .bind(facility_uuid)
        .execute(&pool)
        .await
        .expect("failed to clean test counter");

    // Seed the facility (same shape as the sibling test).
    sqlx::query(
        "INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, is_active, short_code)
         VALUES ($1, 'Test Facility First Day', 'puskesmas', 'Jl. Test No. 2', 'TestKec', 'TestKota', 'TestProv', '021-000001', 100, 50, true, 'TST')
         ON CONFLICT (id) DO NOTHING",
    )
    .bind(facility_uuid)
    .execute(&pool)
    .await
    .expect("failed to seed test facility");

    // Fire N parallel requests at a counter that does not exist yet. Before
    // the fix, the first two requests both saw "no row" and both INSERTed,
    // so one failed with a PK violation (Status::internal).
    let n = 16usize;
    let mut handles = Vec::with_capacity(n);

    for i in 0..n {
        let pool = pool.clone();
        let fid = facility_id.clone();
        let handle = tokio::spawn(async move {
            let req = GenerateQueueRequest {
                facility_id: fid,
                patient: Some(PatientInfo {
                    full_name: format!("Pasien First Day {}", i),
                    phone: format!("081234568{:03}", i),
                    gender: Some("L".to_string()),
                    date_of_birth: Some("1990-01-01".to_string()),
                }),
            };
            sigap_queue_engine::engine::queue::generate_queue_number_tx(&pool, req, TEST_SIGNING_SECRET).await
        });
        handles.push(handle);
    }

    // The numbers must be exactly 1..=N: the counter started empty, so the
    // first ticket is 1 and every request gets the next number — no gaps from
    // failed attempts, no duplicates.
    let mut numbers: Vec<i32> = Vec::with_capacity(n);
    for handle in handles {
        let resp = handle
            .await
            .expect("task join failed")
            .expect("generate_queue_number_tx returned Err on the first ticket of the day");
        let suffix = resp
            .formatted_number
            .strip_prefix("TST-")
            .expect("formatted number must start with TST-");
        numbers.push(
            suffix
                .parse::<i32>()
                .expect("formatted number suffix must be numeric"),
        );
    }
    numbers.sort_unstable();
    let expected: Vec<i32> = (1..=n as i32).collect();
    assert_eq!(
        numbers, expected,
        "expected the exact sequence 1..={} from an empty counter, got {:?}",
        n, numbers
    );

    // Backstop probe: a direct duplicate insert for the same facility, day
    // and number must be rejected by migration 0015's UNIQUE index. Read the
    // service day back from a ticket the engine just wrote, so the probe uses
    // the engine's own day (no clock math in the test).
    let service_date: chrono::NaiveDate = sqlx::query_scalar(
        "SELECT service_date FROM queue_tickets WHERE facility_id = $1 LIMIT 1",
    )
    .bind(facility_uuid)
    .fetch_one(&pool)
    .await
    .expect("expected at least one ticket for the test facility");

    let patient_id: uuid::Uuid =
        sqlx::query_scalar("SELECT id FROM patients WHERE phone = $1")
            .bind("081234568000")
            .fetch_one(&pool)
            .await
            .expect("expected the first test patient to exist");

    let duplicate = sqlx::query(
        r#"
        INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status, service_date)
        VALUES ($1, $2, $3, 1, 'TST-0001', 'waiting', $4)
        "#,
    )
    .bind(uuid::Uuid::new_v4())
    .bind(facility_uuid)
    .bind(patient_id)
    .bind(service_date)
    .execute(&pool)
    .await;

    match duplicate {
        Ok(_) => panic!(
            "duplicate ticket (facility, service_date, queue_number=1) was ACCEPTED — migration 0015's UNIQUE index uq_queue_ticket_facility_day_number is not applied in this database"
        ),
        Err(e) => assert!(
            e.to_string()
                .contains("uq_queue_ticket_facility_day_number"),
            "duplicate insert failed, but not with the expected unique violation (got: {})",
            e
        ),
    }
}
