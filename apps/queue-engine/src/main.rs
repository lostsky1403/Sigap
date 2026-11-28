use sqlx::PgPool;
use tonic::transport::Server;

pub mod queue_engine {
    tonic::include_proto!("sigap.queue_engine");
}

pub mod engine;

use queue_engine::queue_engine_server::{QueueEngine, QueueEngineServer};
use queue_engine::{GenerateQueueRequest, GenerateQueueResponse};

pub struct SigapQueueEngine {
    pool: PgPool,
    signing_secret: Vec<u8>,
}

impl SigapQueueEngine {
    pub fn new(pool: PgPool, signing_secret: Vec<u8>) -> Self {
        Self {
            pool,
            signing_secret,
        }
    }
}

#[tonic::async_trait]
impl QueueEngine for SigapQueueEngine {
    async fn generate_queue_number(
        &self,
        request: tonic::Request<GenerateQueueRequest>,
    ) -> Result<tonic::Response<GenerateQueueResponse>, tonic::Status> {
        let req = request.into_inner();
        let resp =
            engine::queue::generate_queue_number_tx(&self.pool, req, &self.signing_secret)
                .await?;
        Ok(tonic::Response::new(resp))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let database_url =
        std::env::var("DATABASE_URL").expect("DATABASE_URL environment variable is required");

    // The record-integrity checksum is HMAC-SHA256 with this secret. The
    // engine refuses to start without it: a missing secret must never
    // silently downgrade signing to an unkeyed hash. Generate one with
    // `openssl rand -hex 32` and set it in .env (see .env.example).
    let signing_secret = std::env::var("SIGAP_SIGNING_SECRET")
        .ok()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .expect(
            "SIGAP_SIGNING_SECRET environment variable is required \
             (generate with: openssl rand -hex 32)",
        );

    let pool = PgPool::connect(&database_url).await?;

    let addr = "0.0.0.0:50051".parse()?;
    let engine = SigapQueueEngine::new(pool, signing_secret.into_bytes());

    println!(
        "sigap-queue-engine (real tx + atomic upsert) gRPC listening on {}",
        addr
    );

    Server::builder()
        .add_service(QueueEngineServer::new(engine))
        .serve(addr)
        .await?;

    Ok(())
}
