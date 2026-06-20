package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/events"
	"github.com/sigap/sigap/apps/api/internal/grpc"
	"github.com/sigap/sigap/apps/api/internal/handler"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/router"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// enableCORS wraps handlers to allow browser clients from the SvelteKit web origin.
// Required for direct cross-origin fetch (POST /generate) and EventSource (SSE).
// Allows preflight OPTIONS. Reads allowed origin from SIGAP_WEB_ORIGIN env var
// with a safe localhost default. Production should set this explicitly.
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	allowed := os.Getenv("SIGAP_WEB_ORIGIN")
	if allowed == "" {
		allowed = "http://localhost:3005"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Allow exact match or the configured origin
		if origin == allowed || origin == "http://127.0.0.1:3005" && allowed == "http://localhost:3005" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else if origin == "" {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
		}
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
	// FakeQueueService fallback is gated behind SIGAP_ENGINE_FALLBACK=dev;
	// never auto-fallback silently — fail hard in production.
	svc, err := grpc.NewGRPCQueueService(engineAddr)
	if err != nil {
		fallback := os.Getenv("SIGAP_ENGINE_FALLBACK")
		if fallback == "dev" {
			slog.Warn("SIGAP_ENGINE_FALLBACK=dev set; using FakeQueueService for local development only",
				"err", err, "addr", engineAddr)
			svc = service.NewFakeQueueService()
		} else {
			slog.Error("failed to connect to rust engine; set SIGAP_ENGINE_FALLBACK=dev for local dev, or ensure engine is reachable",
				"err", err, "addr", engineAddr)
			os.Exit(1)
		}
	}

	qh := handler.NewHandler(svc, rl)

	// Audit service: best-effort append-only logging. Disabled when
	// SIGAP_DATABASE_URL is missing or the connection fails so the
	// server can start without a reachable PostgreSQL instance.
	var auditSvc *audit.Service
	var dbPool *pgxpool.Pool
	dbURL := os.Getenv("SIGAP_DATABASE_URL")
	if dbURL != "" {
		pool, err := pgxpool.New(context.Background(), dbURL)
		if err != nil {
			slog.Warn("SIGAP_DATABASE_URL set but failed to connect; audit logging disabled", "err", err)
		} else {
			dbPool = pool
			auditSvc = audit.NewService(pool)
			slog.Info("audit logging enabled")
		}
	} else {
		slog.Info("SIGAP_DATABASE_URL not set; audit logging disabled")
	}
	qh = qh.WithAudit(auditSvc)

	// Admin handler: queries facilities. Available only when DB is reachable.
	var adminH *handler.AdminHandler
	if dbPool != nil {
		adminH = handler.NewAdminHandler(dbPool).WithAudit(auditSvc)
		slog.Info("admin handler enabled")
	}

	// Load auth config and select provider.
	authCfg, err := auth.LoadConfigFromEnv()
	if err != nil {
		slog.Error("invalid auth configuration; refusing to start", "err", err)
		os.Exit(1)
	}
	provider := auth.NewProvider(authCfg)
	slog.Info("auth provider configured", "mode", authCfg.Mode)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"sigap-api"}`))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := svc.Probe(ctx); err != nil {
			slog.Warn("readyz probe failed", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable","service":"sigap-api","detail":"engine unreachable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready","service":"sigap-api"}`))
	})

	mux.HandleFunc("/api/v1/queues/generate", enableCORS(qh.Generate))

	// Real-time SSE endpoint for bed/queue updates (Langkah 2)
	// Wrapped for CORS so browser EventSource from localhost:3005 works when not using proxy
	mux.HandleFunc("/api/v1/events/beds", enableCORS(events.Bus.ServeSSE))

	// Super App: Smart Referral (mapcn peta rujukan) + Health Wallet records.
	// /facilities/nearby exposes only facility/bed info (non-PHI) and stays open.
	// The medical-records endpoints serve patient-shaped data with NO authn/authz,
	// so they are guarded: closed by default until real access control lands.
	mux.HandleFunc("/api/v1/facilities/nearby", enableCORS(facilitiesNearbyHandler))
	mux.HandleFunc("/api/v1/medical-records", enableCORS(guardDemoPHI(medicalRecordsHandler)))
	mux.HandleFunc("/api/v1/records/", enableCORS(guardDemoPHI(func(w http.ResponseWriter, r *http.Request) {
		phone := strings.TrimPrefix(r.URL.Path, "/api/v1/records/")
		if phone == "" {
			phone = r.URL.Query().Get("phone")
		}
		// Reuse the demo handler by setting query (for compatibility with existing medicalRecordsHandler)
		r.URL.RawQuery = "phone=" + phone
		medicalRecordsHandler(w, r)
	})))

	// Admin endpoints: protected by facility.read and facility.manage permissions via RequirePermission
	if adminH != nil {
		mux.HandleFunc("/api/v1/admin/facilities", enableCORS(adminH.ListFacilities))
		mux.HandleFunc("/api/v1/admin/facilities/", enableCORS(adminH.FacilitiesRouter))
		mux.HandleFunc("/api/v1/admin/queues", enableCORS(adminH.QueuesRouter))
		mux.HandleFunc("/api/v1/admin/queues/", enableCORS(adminH.QueuesRouter))
		mux.HandleFunc("/api/v1/admin/service-units", enableCORS(adminH.ServiceUnitsRouter))
		mux.HandleFunc("/api/v1/admin/service-units/", enableCORS(adminH.ServiceUnitsRouter))
		mux.HandleFunc("/api/v1/admin/schedules", enableCORS(adminH.SchedulesRouter))
		mux.HandleFunc("/api/v1/admin/schedules/", enableCORS(adminH.SchedulesRouter))
		mux.HandleFunc("/api/v1/admin/appointments", enableCORS(adminH.AppointmentsRouter))
		mux.HandleFunc("/api/v1/admin/appointments/", enableCORS(adminH.AppointmentsRouter))
		slog.Info("admin routes registered", "paths", []string{
			"/api/v1/admin/facilities",
			"/api/v1/admin/facilities/",
			"/api/v1/admin/queues",
			"/api/v1/admin/queues/",
			"/api/v1/admin/service-units",
			"/api/v1/admin/service-units/",
			"/api/v1/admin/schedules",
			"/api/v1/admin/schedules/",
			"/api/v1/admin/appointments",
			"/api/v1/admin/appointments/",
		})
		// Public booking endpoint (no auth required)
		bookingH := handler.NewBookingHandler(dbPool, rl)
		bookingH.WithAudit(auditSvc)
		bookingH.WithQueueService(svc)
		mux.HandleFunc("/api/v1/appointments", enableCORS(bookingH.PublicAppointmentsRouter))
		mux.HandleFunc("/api/v1/appointments/", enableCORS(bookingH.PublicAppointmentsRouter))
		slog.Info("booking routes registered", "paths", []string{
			"/api/v1/appointments",
			"/api/v1/appointments/",
		})
	} else {
		slog.Warn("admin handler skipped: no database connection")
	}

	slog.Info("sigap-api listening", "port", port,
		"rate_limit", "2 per hari per (HP + faskes)",
		"engine", engineAddr,
		"traceability", "processing_time from Rust gRPC in µs")

	// Deny-by-default: only routes declared in the registry (or allow-listed
	// probes) are reachable; everything else gets 401. This is the seam that
	// per-route RBAC and audit logging attach to in later phases.
	// RequirePermission enforces per-route RequiredPolicy using the actor
	// injected by the selected auth provider. The order is:
	//   DenyByDefault → AuthProvider → injectAudit → RequirePermission → mux.
	injectAudit := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.ContextWithAudit(r.Context(), auditSvc)))
		})
	}
	if err := http.ListenAndServe(":"+port, router.DenyByDefault(auth.Middleware(provider)(injectAudit(identity.RequirePermission(mux))))); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// --- Super App endpoints: mapcn Smart Routing + Health Wallet ---

// guardDemoPHI gates patient-shaped (PHI) demo endpoints that currently have NO
// authentication or authorization. They are closed by default and only served
// when SIGAP_ENABLE_DEMO_PHI=true is explicitly set for local development.
// This closes the live unauthenticated PHI exposure until real access control
// (RBAC) is wired in a later phase. Never enable this in production.
func guardDemoPHI(next http.HandlerFunc) http.HandlerFunc {
	enabled := strings.EqualFold(os.Getenv("SIGAP_ENABLE_DEMO_PHI"), "true")
	return func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"success": false,
				"error":   "Endpoint tidak tersedia: akses data medis dinonaktifkan hingga autentikasi tersedia.",
			})
			return
		}
		slog.Warn("serving demo PHI endpoint without authn/authz; dev only",
			"path", r.URL.Path, "flag", "SIGAP_ENABLE_DEMO_PHI")
		next(w, r)
	}
}


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
