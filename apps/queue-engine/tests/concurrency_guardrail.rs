//! Concurrency guardrail: N concurrent queue requests must produce
//! unique sequential numbers (zero double-booking).
//!
//! Run with: `cargo test --test concurrency_guardrail -- --test-threads=16`
//!
//! This test requires a running PostgreSQL with the sigap schema.
//! Set DATABASE_URL or it defaults to the local docker-compose URL.
//!
//! Skipped gracefully if the database is unreachable so CI without DB still passes.

use std::collections::HashSet;
use std::env;

// Re-export the engine module path via the crate
use sigap_queue_engine::queue_engine::{GenerateQueueRequest, PatientInfo};

/// Deterministic facility UUID for this test. The INSERT uses
/// ON CONFLICT DO NOTHING so repeated runs are idempotent.
const TEST_FACILITY_ID: &str = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11";

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
                sigap_queue_engine::engine::queue::generate_queue_number_tx(&pool, req).await;
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
