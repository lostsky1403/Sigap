package main

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/sigap/sigap/apps/api/internal/events"
	"github.com/sigap/sigap/apps/api/internal/grpc"
	"github.com/sigap/sigap/apps/api/internal/handler"
	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// enableCORS wraps handlers to allow browser clients from the SvelteKit web origin.
// Required for direct cross-origin fetch (POST /generate) and EventSource (SSE).
// Allows preflight OPTIONS. Specific origin for dev; tighten in prod behind proxy.
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Support localhost and 127.0.0.1 on port 3000 (SvelteKit default in compose)
		origin := r.Header.Get("Origin")
		allowed := "http://localhost:3000"
		if origin == "http://127.0.0.1:3000" {
			allowed = origin
		}
		w.Header().Set("Access-Control-Allow-Origin", allowed)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

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

	http.HandleFunc("/api/v1/queues/generate", enableCORS(qh.Generate))

	// Real-time SSE endpoint for bed/queue updates (Langkah 2)
	// Wrapped for CORS so browser EventSource from localhost:3000 works when not using proxy
	http.HandleFunc("/api/v1/events/beds", enableCORS(events.Bus.ServeSSE))

	// Super App: Smart Referral (mapcn peta rujukan) + Health Wallet records
	http.HandleFunc("/api/v1/facilities/nearby", enableCORS(facilitiesNearbyHandler))
	http.HandleFunc("/api/v1/medical-records", enableCORS(medicalRecordsHandler))

	slog.Info("sigap-api listening", "port", port,
		"rate_limit", "2 per hari per (HP + faskes)",
		"engine", engineAddr,
		"traceability", "processing_time from Rust gRPC in µs")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// --- Super App endpoints: mapcn Smart Routing + Health Wallet ---

// facilitiesNearbyHandler returns alternative facilities sorted by distance + availability (for rujukan when target penuh).
// Uses same coords as UI samples for consistency. Real version would query DB with PostGIS.
func facilitiesNearbyHandler(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	exclude := r.URL.Query().Get("exclude")

	lat, _ := strconv.ParseFloat(latStr, 64)
	lon, _ := strconv.ParseFloat(lonStr, 64)
	if lat == 0 || lon == 0 {
		lat, lon = -6.2088, 106.8456 // default Jakarta
	}

	// Hardcoded master list (sync with UI + previous geo task). In prod: SELECT ... from facilities + ST_Distance.
	type f struct {
		ID      string  `json:"id"`
		Name    string  `json:"name"`
		Short   string  `json:"short_code"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		Avail   int     `json:"available_beds"`
		Total   int     `json:"total_beds"`
	}
	all := []f{
		{"f1", "RSUD Kota Sehat", "RSK", -6.9175, 107.6191, 42, 180},
		{"f2", "Puskesmas Sukajaya", "PKM", -6.9820, 107.6820, 19, 28},
		{"f3", "RS Mitra Sehat", "RSM", -6.1751, 106.8270, 11, 95},
		{"f4", "Puskesmas Melati Indah", "PMI", -6.2658, 106.7814, 27, 35},
		{"f5", "RSUD Sejahtera", "RSJ", -6.9197, 106.9270, 68, 120},
		{"f6", "Puskesmas Harapan Baru", "PHB", -6.5950, 106.8000, 5, 22},
	}

	type out struct {
		ID      string  `json:"id"`
		Name    string  `json:"name"`
		Short   string  `json:"short_code"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		Avail   int     `json:"available_beds"`
		Total   int     `json:"total_beds"`
		DistKm  float64 `json:"dist_km"`
	}
	var res []out
	for _, fac := range all {
		if fac.ID == exclude {
			continue
		}
		if fac.Avail <= 0 {
			continue
		}
		d := haversine(lat, lon, fac.Lat, fac.Lon)
		res = append(res, out{fac.ID, fac.Name, fac.Short, fac.Lat, fac.Lon, fac.Avail, fac.Total, d})
	}
	// sort by dist asc
	for i := 0; i < len(res); i++ {
		for j := i + 1; j < len(res); j++ {
			if res[j].DistKm < res[i].DistKm {
				res[i], res[j] = res[j], res[i]
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": res})
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// medicalRecordsHandler for Health Wallet UI (/wallet page).
// Returns patient history with signature (immutable SHA-256 proof from Rust engine).
// In real: SELECT from medical_records WHERE patient_phone = ? ORDER BY visit_time DESC
func medicalRecordsHandler(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		phone = "081234567890" // demo default
	}

	// Demo data (populated by Rust inserts on real queue success; signatures are SHA-256 style)
	// Real query would use the DB + join facilities for names.
	type rec struct {
		VisitTime       string `json:"visit_time"`
		FacilityName    string `json:"facility_name"`
		FormattedNumber string `json:"formatted_number"`
		Signature       string `json:"signature"`
	}
	demo := []rec{
		{"2026-06-12T10:15:00Z", "Puskesmas Melati Indah", "PMI-0042", "a1b2c3d4e5f67890123456789abcdef0123456789abcdef0123456789abcdef01"},
		{"2026-06-11T09:05:00Z", "RSUD Kota Sehat", "RSK-0039", "b2c3d4e5f67890123456789abcdef0123456789abcdef0123456789abcdef0123"},
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": demo})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// init superapp routes (called implicitly on package load via main registration below)
func init() {
	// registered after main handlers in main() body via edits; see HandleFunc calls added below if needed
}

// Note: the HandleFunc registrations for /nearby and /medical-records are added at the end of main() setup in this file.
