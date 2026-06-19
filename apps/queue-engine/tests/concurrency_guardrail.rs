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

#[tokio::test]
async fn concurrent_queue_requests_produce_unique_numbers() {
    let database_url = env::var("DATABASE_URL")
        .unwrap_or_else(|_| "postgresql://sigap:sigap@localhost:5432/sigap".to_string());

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

    // Use a known seeded facility from migrations (RSUD Kota Sehat — uuid from seeds).
    // If the seed data is absent, the test will fail fast with a clear error.
    let facility_id = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11".to_string();

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
