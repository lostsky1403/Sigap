package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

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

	// Rate limiter: initial anti-spam protection.
	// 10 generates per IP per minute is generous for citizens but stops naive bots/calo.
	rl := limiter.NewRateLimiter(10, time.Minute)

	// Use the fake service for now (self-contained for tests and demo).
	// Phase 3+ will replace with real gRPC client to Rust engine.
	svc := service.NewFakeQueueService()

	qh := handler.NewHandler(svc, rl)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"sigap-api"}`))
	})

	http.HandleFunc("/api/v1/queues/generate", qh.Generate)

	slog.Info("sigap-api listening", "port", port, "rate_limit", "10/min per IP (anti-spam)")
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
