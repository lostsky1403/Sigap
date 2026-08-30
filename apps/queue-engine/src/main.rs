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
}

impl SigapQueueEngine {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }
}

#[tonic::async_trait]
impl QueueEngine for SigapQueueEngine {
    async fn generate_queue_number(
        &self,
        request: tonic::Request<GenerateQueueRequest>,
    ) -> Result<tonic::Response<GenerateQueueResponse>, tonic::Status> {
        let req = request.into_inner();
        let resp = engine::queue::generate_queue_number_tx(&self.pool, req).await?;
        Ok(tonic::Response::new(resp))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let database_url =
        std::env::var("DATABASE_URL").expect("DATABASE_URL environment variable is required");

    let pool = PgPool::connect(&database_url).await?;

    let addr = "0.0.0.0:50051".parse()?;
    let engine = SigapQueueEngine::new(pool);

    println!(
        "sigap-queue-engine (real tx + FOR UPDATE) gRPC listening on {}",
        addr
    );

    Server::builder()
        .add_service(QueueEngineServer::new(engine))
        .serve(addr)
        .await?;

    Ok(())
}
