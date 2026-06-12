# apps/api/Dockerfile
# Multi-stage Go build for tiny secure image
FROM golang:alpine AS builder

WORKDIR /app

# Leverage cache (context is monorepo root)
COPY protos/ ./protos/
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
RUN cd /app/apps/api && go mod download

# Copy full source preserving module structure
COPY apps/api/ ./apps/api/

# Build static binary
RUN cd /app/apps/api && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /sigap-api ./cmd/server

# Final minimal image
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S sigap && adduser -S sigap -G sigap

WORKDIR /home/sigap
USER sigap

COPY --from=builder /sigap-api .

EXPOSE 8080

ENV SIGAP_API_PORT=8080
ENV SIGAP_ENGINE_ADDR=rust-engine:50051

ENTRYPOINT ["./sigap-api"]
