package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

// Minimal bootstrap server for Phase 0.
// Full queue handler + gRPC client + TDD implemented in Phase 2.
func main() {
	port := os.Getenv("SIGAP_API_PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"sigap-api"}`))
	})

	// Placeholder — real /api/v1/queues/generate added in Phase 2 with TDD
	http.HandleFunc("/api/v1/queues/generate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = fmt.Fprint(w, `{"success":false,"error":"Endpoint belum diimplementasikan — lihat Phase 2 TDD"}`)
	})

	slog.Info("sigap-api listening", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
