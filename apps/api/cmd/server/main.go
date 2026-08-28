package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/config"
	"github.com/sigap/sigap/apps/api/internal/events"
	"github.com/sigap/sigap/apps/api/internal/grpc"
	"github.com/sigap/sigap/apps/api/internal/handler"
	"github.com/sigap/sigap/apps/api/internal/migrate"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/notification"
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
	// The loopback exception (127.0.0.1 / localhost) only applies when the
	// configured origin is itself a loopback URL.  In production this must
	// not apply — only the exact configured origin is allowed.
	loopbackOrigin := strings.HasPrefix(allowed, "http://localhost") || strings.HasPrefix(allowed, "http://127.0.0.1")
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Allow exact match, or the loopback counterpart when in loopback mode.
		allow := origin == allowed
		if !allow && loopbackOrigin {
			// Match the other loopback form: localhost ↔ 127.0.0.1
			if (strings.HasPrefix(origin, "http://localhost:") && strings.HasPrefix(allowed, "http://127.0.0.1:")) ||
				(strings.HasPrefix(origin, "http://127.0.0.1:") && strings.HasPrefix(allowed, "http://localhost:")) {
				allow = true
			}
		}
		if allow {
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
	// Fail-fast: reject dev-only capabilities in non-local environments.
	if err := config.GuardDevCapabilities(); err != nil {
		slog.Error("environment guard failed", "err", err)
		fmt.Fprintln(os.Stderr, "FATAL:", err)
		os.Exit(1)
	}

	// Fail-fast: require TLS termination confirmation outside local.
	if err := config.GuardTLS(); err != nil {
		slog.Error("TLS guard failed", "err", err)
		fmt.Fprintln(os.Stderr, "FATAL:", err)
		os.Exit(1)
	}

	// Boot banner: show auth mode + environment for operational visibility.
	sigapEnv := os.Getenv("SIGAP_ENV")
	authMode := os.Getenv("SIGAP_AUTH_MODE")
	if authMode == "" {
		authMode = "disabled"
	}
	slog.Info("sigap-api starting",
		"env", sigapEnv,
		"auth_mode", authMode,
		"dev_identity", os.Getenv("SIGAP_DEV_IDENTITY"),

		"engine_fallback", os.Getenv("SIGAP_ENGINE_FALLBACK"),
	)

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

	// Run tracked migrations on startup when SIGAP_AUTO_MIGRATE=true.
	// Each migration is applied in its own transaction and recorded in
	// schema_migrations.  This replaces the manual psql invocation.
	if dbPool != nil && strings.EqualFold(os.Getenv("SIGAP_AUTO_MIGRATE"), "true") {
		migDir, err := migrate.MigrateDir()
		if err != nil {
		slog.Error("migration directory not found", "err", err)
		os.Exit(1)
		}
		applied, err := migrate.Run(context.Background(), dbPool, migDir)
		if err != nil {
			slog.Error("migration failed; refusing to start", "err", err)
			os.Exit(1)
		}
		if applied > 0 {
			slog.Info("migrations applied", "count", applied)
		}
	} else if dbPool != nil {
		slog.Info("SIGAP_AUTO_MIGRATE not set; skipping auto-migration")
	}
	qh = qh.WithAudit(auditSvc)

	// Admin handler: queries facilities. Available only when DB is reachable.
	var adminH *handler.AdminHandler
	if dbPool != nil {
		adminH = handler.NewAdminHandler(dbPool).WithAudit(auditSvc).WithFacilityScopeResolver(auth.NewDBFacilityScope(dbPool))
		slog.Info("admin handler enabled")
	}

	// Load auth config and select provider.
	authCfg, err := auth.LoadConfigFromEnv()
	if err != nil {
		slog.Error("invalid auth configuration; refusing to start", "err", err)
		os.Exit(1)
	}
	// Construct the auth provider. In jwt mode, authorization permissions are
	// resolved server-side from the trusted DB RBAC state (AUDIT-101): the
	// token is authoritative for identity only. If the DB pool is unavailable
	// in jwt mode the provider fails closed (zero permissions), never falling
	// back to token-claimed permissions.
	var provider auth.Provider
	if authCfg.Mode == auth.AuthModeJWT {
		if dbPool != nil {
			resolver := auth.NewRBACResolver(dbPool)
			provider = auth.NewJWTProviderWithResolver(*authCfg, resolver)
			slog.Info("jwt provider wired with server-side RBAC resolver")
		} else {
			provider = auth.NewJWTProvider(*authCfg)
			slog.Warn("jwt mode without DB-backed RBAC resolver; authz fails closed (zero permissions)")
		}
	} else {
		provider = auth.NewProvider(authCfg)
	}
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

		var details []string
		ready := true

		// Check engine connectivity.
		if err := svc.Probe(ctx); err != nil {
			slog.Warn("readyz: engine probe failed", "err", err)
			details = append(details, "engine unreachable")
			ready = false
		}

		// Check database connectivity (AUDIT-1202).
		if dbPool != nil {
			if err := dbPool.Ping(ctx); err != nil {
				slog.Warn("readyz: database ping failed", "err", err)
				details = append(details, "database unreachable")
				ready = false
			}
		}

		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"unavailable","service":"sigap-api","detail":"%s"}`, strings.Join(details, "; "))))
			return
		}

		auditState := "disabled"
		if auditSvc != nil {
			auditState = "enabled"
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ready","service":"sigap-api","audit":"%s"}`, auditState)))
	})

	mux.HandleFunc("/api/v1/queues/generate", enableCORS(qh.Generate))

	// Real-time SSE endpoint for bed/queue updates (Langkah 2)
	// Wrapped for CORS so browser EventSource from localhost:3005 works when not using proxy
	mux.HandleFunc("/api/v1/events/beds", enableCORS(events.Bus.ServeSSE))

	// Super App: facilities/nearby exposes only facility/bed info (non-PHI).
	mux.HandleFunc("/api/v1/facilities/nearby", enableCORS(facilitiesNearbyHandler))

	// Public catalog endpoints: no auth required; return minimal fields for
	// the patient booking flow (AUDIT-1801).
	if dbPool != nil {
		catalogH := handler.NewCatalogHandler(dbPool)
		mux.HandleFunc("/api/v1/public/facilities", enableCORS(catalogH.ListPublicFacilities))
		mux.HandleFunc("/api/v1/public/service-units", enableCORS(catalogH.ListPublicServiceUnits))
		slog.Info("public catalog routes registered")
	}

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
		// AUDIT-302: check-in brute-force protection — 5 attempts per5 min per (IP, appointment).
		bookingH.WithCheckinLimiter(limiter.NewRateLimiter(5, 5*time.Minute))
		mux.HandleFunc("/api/v1/appointments", enableCORS(bookingH.PublicAppointmentsRouter))
		mux.HandleFunc("/api/v1/appointments/", enableCORS(bookingH.PublicAppointmentsRouter))
		slog.Info("booking routes registered", "paths", []string{
			"/api/v1/appointments",
			"/api/v1/appointments/",
		})

		// Patient portal: public status lookup (no auth required)
		patientRL := limiter.NewRateLimiter(30, 1*time.Minute)
		patientH := handler.NewPatientHandler(dbPool, patientRL)
		mux.HandleFunc("/api/v1/patient/status", enableCORS(patientH.PatientStatusLookup))
		slog.Info("patient portal routes registered", "paths", []string{"/api/v1/patient/status"})

		// Notification outbox admin routes. Always require dev identity
		// (the same X-Sigap-Dev-User-ID pattern as other admin routes).
		// The notifications package is imported above; the service is
		// constructed here so it shares the same DB pool and audit
		// service as the rest of the admin surface.
		notifySvc := notification.NewService(dbPool)
		notifyH := handler.NewNotificationsHandler(notifySvc).WithAudit(auditSvc).WithFacilityScopeResolver(auth.NewDBFacilityScope(dbPool))
		bookingH.WithNotificationService(notifySvc)
		mux.HandleFunc("/api/v1/admin/notifications", enableCORS(notifyH.NotificationsRouter))
		mux.HandleFunc("/api/v1/admin/notifications/", enableCORS(notifyH.NotificationsRouter))
		slog.Info("notification routes registered", "paths", []string{
			"/api/v1/admin/notifications",
			"/api/v1/admin/notifications/",
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
	//   LimitBody → SecurityHeaders → TrustedProxy → DenyByDefault → Auth → Audit → RBAC → mux.
	injectAudit := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.ContextWithAudit(r.Context(), auditSvc)))
		})
	}
	handler := router.SecurityHeaders(router.TrustedProxy(router.DenyByDefault(auth.Middleware(provider)(injectAudit(identity.RequirePermission(mux))))))
	handler = router.LimitRequestBody(256<<10)(handler) // AUDIT-1001: 256KB max request body

	// AUDIT-1001: configured http.Server with timeouts to prevent
	// slowloris and resource exhaustion.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// Graceful shutdown on SIGTERM/SIGINT.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		slog.Info("shutdown signal received, draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful shutdown failed", "err", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
