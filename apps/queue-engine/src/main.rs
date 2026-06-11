use tonic::transport::Server;

pub mod queue_engine {
    tonic::include_proto!("sigap.queue_engine");
}

use queue_engine::queue_engine_server::{QueueEngine, QueueEngineServer};
use queue_engine::{GenerateQueueRequest, GenerateQueueResponse};

#[derive(Default)]
pub struct SigapQueueEngine;

#[tonic::async_trait]
impl QueueEngine for SigapQueueEngine {
    async fn generate_queue_number(
        &self,
        _request: tonic::Request<GenerateQueueRequest>,
    ) -> Result<tonic::Response<GenerateQueueResponse>, tonic::Status> {
        // Real implementation (atomic tx + FOR UPDATE + sqlx) comes in the next sub-step of Phase 3.
        // For now this compiles and the gRPC server starts.
        Err(tonic::Status::unimplemented(
            "Rust engine logic not yet wired (see next commit in this Phase 3)",
        ))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let addr = "0.0.0.0:50051".parse()?;
    let engine = SigapQueueEngine::default();

    println!("sigap-queue-engine gRPC listening on {}", addr);

    Server::builder()
        .add_service(QueueEngineServer::new(engine))
        .serve(addr)
        .await?;

    Ok(())
}

