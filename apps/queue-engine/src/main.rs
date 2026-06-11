use std::net::TcpListener;

fn main() {
    // Phase 0 stub — full tonic gRPC + sqlx engine in Phase 3 (TDD first).
    // Listens so `make dev-engine` succeeds for verification.
    let addr = "127.0.0.1:50051";
    let listener = TcpListener::bind(addr).expect("bind engine port");
    println!("sigap-queue-engine stub listening on {}", addr);

    // Accept loop (minimal, real gRPC later)
    for stream in listener.incoming() {
        if let Ok(_) = stream {
            // In real impl: handle tonic requests here
        }
    }
}
