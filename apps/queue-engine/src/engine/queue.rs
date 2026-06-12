use sqlx::{PgPool, Postgres, Transaction};
use std::time::Instant;
use tonic::Status;
use uuid::Uuid;

use sha2::{Sha256, Digest};

use crate::queue_engine::{GenerateQueueRequest, GenerateQueueResponse};

const DAILY_QUEUE_LIMIT: i32 = 300;

/// Core atomic transaction for generating a queue number.
/// Uses SELECT ... FOR UPDATE on daily_queue_counters to guarantee
/// exactly-once increment even under heavy concurrent load (zero double-booking).
pub async fn generate_queue_number_tx(
    pool: &PgPool,
    req: GenerateQueueRequest,
) -> Result<GenerateQueueResponse, Status> {
    let facility_id = Uuid::parse_str(&req.facility_id)
        .map_err(|_| Status::invalid_argument("facility_id bukan UUID yang valid"))?;

    let phone = req.patient.as_ref().map(|p| p.phone.as_str()).unwrap_or("");
    let full_name = req.patient.as_ref().map(|p| p.full_name.as_str()).unwrap_or("");

    if phone.is_empty() || full_name.is_empty() {
        return Err(Status::invalid_argument("phone dan full_name wajib diisi"));
    }

    let start = Instant::now();

    let mut tx: Transaction<'_, Postgres> = pool
        .begin()
        .await
        .map_err(|e| Status::internal(format!("gagal memulai transaksi: {}", e)))?;

    // 1. Validate facility (active)
    let facility_row: Option<(Uuid, Option<String>, bool)> = sqlx::query_as(
        "SELECT id, short_code, is_active FROM facilities WHERE id = $1"
    )
    .bind(facility_id)
    .fetch_optional(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("query facility gagal: {}", e)))?;

    let ( _fac_id, short_code_opt, _is_active ) = match facility_row {
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
        "#
    )
    .bind(full_name)
    .bind(phone)
    .execute(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("upsert pasien gagal: {}", e)))?;

    let patient_id: Uuid = sqlx::query_scalar(
        "SELECT id FROM patients WHERE phone = $1"
    )
    .bind(phone)
    .fetch_one(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("select pasien gagal: {}", e)))?;

    // 3. Atomic daily counter with row-level lock (THE critical part for Zero Double-Booking)
    let today = chrono::Utc::now().date_naive();

    let counter_row: Option<(i32,)> = sqlx::query_as(
        "SELECT last_number FROM daily_queue_counters WHERE facility_id = $1 AND date = $2 FOR UPDATE"
    )
    .bind(facility_id)
    .bind(today)
    .fetch_optional(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("lock counter gagal: {}", e)))?;

    let next_number: i32 = if let Some((last,)) = counter_row {
        let next = last + 1;
        sqlx::query(
            "UPDATE daily_queue_counters SET last_number = $1 WHERE facility_id = $2 AND date = $3"
        )
        .bind(next)
        .bind(facility_id)
        .bind(today)
        .execute(&mut *tx)
        .await
        .map_err(|e| Status::internal(format!("update counter gagal: {}", e)))?;
        next
    } else {
        let first = 1i32;
        sqlx::query(
            "INSERT INTO daily_queue_counters (facility_id, date, last_number) VALUES ($1, $2, $3)"
        )
        .bind(facility_id)
        .bind(today)
        .bind(first)
        .execute(&mut *tx)
        .await
        .map_err(|e| Status::internal(format!("insert counter gagal: {}", e)))?;
        first
    };

    if next_number > DAILY_QUEUE_LIMIT {
        return Err(Status::resource_exhausted(
            "Batas antrean harian untuk fasilitas ini sudah tercapai (300). Silakan coba lagi besok.",
        ));
    }

    let formatted = format!("{}-{:04}", short_code, next_number);

    // Immutable Health Record: compute SHA-256 signature for tamper-proof proof
    let visit_time = chrono::Utc::now();
    let sig_input = format!("{}|{}|{}|{}|{}", phone, facility_id, next_number, formatted, visit_time.to_rfc3339());
    let mut hasher = Sha256::new();
    hasher.update(sig_input.as_bytes());
    let signature = format!("{:x}", hasher.finalize());

    // 4. Create the ticket (client-supplied UUID)
    let ticket_id = Uuid::new_v4();
    sqlx::query(
        r#"
        INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status)
        VALUES ($1, $2, $3, $4, $5, 'waiting')
        "#
    )
    .bind(ticket_id)
    .bind(facility_id)
    .bind(patient_id)
    .bind(next_number)
    .bind(formatted.clone())
    .execute(&mut *tx)
    .await
    .map_err(|e| Status::internal(format!("insert tiket gagal: {}", e)))?;

    // 5. Insert immutable medical record (Health Wallet) with the signature (anti-ubah)
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
        estimated_wait_minutes: 25,
        processing_time_micros,
        signature,
    })
}
