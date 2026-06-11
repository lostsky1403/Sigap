package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/sigap/sigap/apps/api/internal/events"
	"github.com/sigap/sigap/apps/api/internal/grpc"
	"github.com/sigap/sigap/apps/api/internal/handler"
	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// main wires the production-ready (for scaffold) queue endpoint with
// early anti-spam rate limiting + service layer.
// The real heavy logic will be delegated to the Rust gRPC engine in later phases.
func main() {
	port := os.Getenv("SIGAP_API_PORT")
	if port == "" {
		port = "8080"
	}

	// Rate limiter (enhanced anti-spam):
	// 1 nomor HP maksimal 2 antrean per hari per fasilitas.
	// This protects citizens even when using shared/public WiFi (kelurahan, kampus, etc).
	// The key is built in the handler as "YYYY-MM-DD:phone:facilityId".
	rl := limiter.NewDailyLimiter(2)

	engineAddr := os.Getenv("SIGAP_ENGINE_ADDR")
	if engineAddr == "" {
		engineAddr = "localhost:50051"
	}

	// Real gRPC client to Rust engine (with micro-second traceability).
	// Falls back to localhost:50051 for docker-compose / local dev.
	svc, err := grpc.NewGRPCQueueService(engineAddr)
	if err != nil {
		slog.Error("failed to connect to rust engine, using fake for demo", "err", err, "addr", engineAddr)
		svc = service.NewFakeQueueService()
	}

	qh := handler.NewHandler(svc, rl)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"sigap-api"}`))
	})

	http.HandleFunc("/api/v1/queues/generate", qh.Generate)

	// Real-time SSE endpoint for bed/queue updates (Langkah 2)
	http.HandleFunc("/api/v1/events/beds", events.Bus.ServeSSE)

	slog.Info("sigap-api listening", "port", port,
		"rate_limit", "2 per hari per (HP + faskes)",
		"engine", engineAddr,
		"traceability", "processing_time from Rust gRPC in µs")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
