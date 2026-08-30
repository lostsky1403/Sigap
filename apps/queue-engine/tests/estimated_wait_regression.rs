//! Regression test: `estimated_wait_minutes` in the queue response must equal 25.
//!
//! This is the current hardcoded behavior. When real estimation logic is added,
//! this test should be updated to reflect the new expected value.
//!
//! Requires PostgreSQL with sigap schema; skips gracefully if DB is unreachable.

use std::env;

use sigap_queue_engine::queue_engine::{GenerateQueueRequest, PatientInfo};

#[tokio::test]
async fn estimated_wait_minutes_is_25() {
    let database_url = match env::var("DATABASE_URL") {
        Ok(url) => url,
        Err(_) => {
            eprintln!(
                "SKIP: DATABASE_URL not set; estimated_wait regression requires a database connection"
            );
            return;
        }
    };

    let pool = match sqlx::PgPool::connect(&database_url).await {
        Ok(p) => p,
        Err(e) => {
            eprintln!(
                "SKIP: DB unreachable ({}); estimated_wait regression cannot run without DB",
                e
            );
            return;
        }
    };

    let facility_id = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11".to_string();

    let req = GenerateQueueRequest {
        facility_id,
        patient: Some(PatientInfo {
            full_name: "Regression Test Patient".to_string(),
            phone: "081299988877".to_string(),
            gender: Some("L".to_string()),
            date_of_birth: Some("1990-01-01".to_string()),
        }),
    };

    let resp = sigap_queue_engine::engine::queue::generate_queue_number_tx(&pool, req)
        .await
        .expect("generate_queue_number_tx should succeed when DB is available");

    assert_eq!(
        resp.estimated_wait_minutes, 25,
        "estimated_wait_minutes must be 25 (current hardcoded value)"
    );
}
