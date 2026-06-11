# apps/api/Dockerfile
# Multi-stage Go build for tiny secure image
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Leverage cache
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /sigap-api ./cmd/server

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
