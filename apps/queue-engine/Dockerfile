# apps/queue-engine/Dockerfile
# Multi-stage Rust build - produces a very small final image
FROM rust:slim-bookworm AS builder

WORKDIR /app

# Install build deps (for sqlx etc if needed)
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config libssl-dev ca-certificates protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

# Cache deps (context is monorepo root).
# build.rs compiles ../../protos/sigap/queue_engine.proto relative to the
# crate dir, and the crate must be built from its own dir. Keep a repo-mirror
# layout: /app/protos + /app/apps/queue-engine (crate root).
COPY protos/ /app/protos/
COPY apps/queue-engine/Cargo.toml apps/queue-engine/Cargo.lock /app/apps/queue-engine/
WORKDIR /app/apps/queue-engine
RUN mkdir -p src && echo "fn main(){}" > src/main.rs && cargo build --release && rm -rf src target

# Copy full app source (protos already copied above for build.rs)
COPY apps/queue-engine/ /app/apps/queue-engine/
WORKDIR /app/apps/queue-engine

# Build the real binary
RUN cargo build --release

# Runtime image - distroless-ish for security
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates netcat-openbsd \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 10001 sigap

WORKDIR /home/sigap
USER sigap

COPY --from=builder /app/apps/queue-engine/target/release/sigap-queue-engine /usr/local/bin/sigap-queue-engine

EXPOSE 50051

ENV RUST_LOG=info

ENTRYPOINT ["sigap-queue-engine"]
