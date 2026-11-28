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

// registerAdminRoutes registers every /api/v1/admin route on the mux.
//
// It is a named function rather than inline registration in main() for one
// reason: the facility routing below is a SHADOWING hazard that only exists at
// the ServeMux level, and a test cannot reach a statement buried inside main().
// Extracting the registrations is what lets facilities_routing_test.go assert
// against the real wiring rather than a hand-built imitation of it — an earlier
// version of that test built its own mux and passed even with the bug
// reintroduced, which proved nothing about the code it claimed to protect.
//
// Authorization is unaffected by this extraction: RequirePermission still runs
// upstream of these handlers, and each handler still authorizes independently.
func registerAdminRoutes(mux *http.ServeMux, adminH *handler.AdminHandler) {
	// BOTH facilities paths go to FacilitiesRouter, which dispatches on method
	// and already handles the bare path for GET.
	//
	// Registering ListFacilities on the bare path as well (as this once did) is
	// NOT a harmless duplicate. net/http resolves patterns by specificity, not by
	// method, so the more specific pattern "/api/v1/admin/facilities" matches
	// EVERY method — it shadowed the dispatcher for POST, CreateFacility became
	// unreachable, and the create answered 200 with the facility LIST. That is
	// the worst shape of failure available: a create that reports success,
	// returns rows, and persists nothing.
	//
	// Found by the Phase 3B5 real mutation E2E rather than by a unit test: the
	// handler was correct and the route manifest (internal/router) already
	// declared POST, so nothing except the mux registration was wrong.
	//
	// Every sibling below uses one dispatcher for both the bare and the
	// prefixed path, which is the arrangement that cannot shadow.
	mux.HandleFunc("/api/v1/admin/facilities", enableCORS(adminH.FacilitiesRouter))
	mux.HandleFunc("/api/v1/admin/facilities/", enableCORS(adminH.FacilitiesRouter))
	mux.HandleFunc("/api/v1/admin/queues", enableCORS(adminH.QueuesRouter))
	mux.HandleFunc("/api/v1/admin/queues/", enableCORS(adminH.QueuesRouter))
	mux.HandleFunc("/api/v1/admin/service-units", enableCORS(adminH.ServiceUnitsRouter))
	mux.HandleFunc("/api/v1/admin/service-units/", enableCORS(adminH.ServiceUnitsRouter))
	mux.HandleFunc("/api/v1/admin/schedules", enableCORS(adminH.SchedulesRouter))
	mux.HandleFunc("/api/v1/admin/schedules/", enableCORS(adminH.SchedulesRouter))
	mux.HandleFunc("/api/v1/admin/appointments", enableCORS(adminH.AppointmentsRouter))
	mux.HandleFunc("/api/v1/admin/appointments/", enableCORS(adminH.AppointmentsRouter))
	mux.HandleFunc("/api/v1/admin/organizations", enableCORS(adminH.OrganizationsRouter))
	mux.HandleFunc("/api/v1/admin/organizations/", enableCORS(adminH.OrganizationsRouter))
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
		"/api/v1/admin/organizations",
		"/api/v1/admin/organizations/",
	})
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

	// Fail-fast: when dev-only capabilities are active, the server must
	// bind to a loopback address only. This is defence-in-depth against
	// the dev identity header bypass being reachable from the network.
	if err := config.GuardDevNetworkBinding(); err != nil {
		slog.Error("dev network binding guard failed", "err", err)
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
		adminH = handler.NewAdminHandler(dbPool).WithAudit(auditSvc).WithFacilityScopeResolver(auth.NewDBFacilityScope(dbPool)).WithQueueService(svc)
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

	// Phase 3B5.0: the local DB-backed test identity selector.
	//
	// This is NOT a fallback and NOT a weakening of the provider above. It is
	// a separate, explicitly armed provider that resolves a named subject
	// through the SAME real RBAC resolver the JWT path uses, so local E2E can
	// exercise genuine facility-scoped schedule.manage without the dev
	// identity gaining a permission it deliberately lacks.
	//
	// Two independent gates keep it out of production, and both must open:
	// SIGAP_ENV=local (checked inside the constructor) and
	// SIGAP_LOCAL_RBAC_TEST_IDENTITY=true (also registered with
	// GuardDevCapabilities above, so a non-local deploy with the flag set
	// refuses to start rather than running armed).
	//
	// It takes precedence when armed, because a request that names a local
	// test subject is a test request by definition. In every other case the
	// configured provider stands unchanged.
	if armed := os.Getenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY") == "true"; armed {
		if dbPool == nil {
			// Fail closed and loudly. Without the pool there is no resolver,
			// and a selector with no resolver could not report anything
			// trustworthy — so it authenticates nobody.
			slog.Warn("local RBAC test identity requested but no DB pool is available; " +
				"the selector stays disarmed and will authenticate nobody")
		} else {
			localProvider := auth.NewLocalRBACTestIdentityProvider(auth.NewRBACResolver(dbPool))
			if localProvider.Armed() {
				provider = localProvider
				slog.Warn("local RBAC test identity SELECTOR ACTIVE — requests may choose a " +
					"DB-seeded identity via X-Sigap-Local-Test-Subject. Local only.")
			} else {
				slog.Warn("local RBAC test identity requested outside SIGAP_ENV=local; " +
					"the selector stays disarmed")
			}
		}
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

	// Real-time SSE endpoint for queue updates (Langkah 2). The stream carries
	// queue events only — there is no live bed feed, so nothing here claims
	// to push bed availability.
	// Wrapped for CORS so browser EventSource from localhost:3005 works when not using proxy
	mux.HandleFunc("/api/v1/events/beds", enableCORS(events.Bus.ServeSSE))

	// Super App: facilities/nearby exposes only facility/bed info (non-PHI),
	// read from the facilities table. Bed counts are operator-maintained
	// values with a beds_updated_at timestamp — never presented as live.
	mux.HandleFunc("/api/v1/facilities/nearby", enableCORS(facilitiesNearbyHandler(dbPool)))

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
		registerAdminRoutes(mux, adminH)

		// Shared per-IP limiter for the two code-bearing public surfaces: the
		// patient status lookup and the citizen self-cancellation endpoint. One
		// instance means a code cannot be probed at double the rate by
		// alternating between the two endpoints.
		patientRL := limiter.NewRateLimiter(30, 1*time.Minute)

		// Public booking endpoint (no auth required)
		bookingH := handler.NewBookingHandler(dbPool, rl)
		bookingH.WithAudit(auditSvc)
		bookingH.WithQueueService(svc)
		// AUDIT-302: check-in brute-force protection — 5 attempts per5 min per (IP, appointment).
		bookingH.WithCheckinLimiter(limiter.NewRateLimiter(5, 5*time.Minute))
		// Citizen cancellation shares the status lookup's 30/min/IP budget.
		bookingH.WithCancelLimiter(patientRL)
		mux.HandleFunc("/api/v1/appointments", enableCORS(bookingH.PublicAppointmentsRouter))
		mux.HandleFunc("/api/v1/appointments/", enableCORS(bookingH.PublicAppointmentsRouter))
		slog.Info("booking routes registered", "paths", []string{
			"/api/v1/appointments",
			"/api/v1/appointments/",
		})

		// Patient portal: public status lookup (no auth required)
		patientH := handler.NewPatientHandler(dbPool, patientRL)
		mux.HandleFunc("/api/v1/patient/status", enableCORS(patientH.PatientStatusLookup))
		// Patient portal: the signed-in citizen's own appointments. Authenticated
		// by account (identity.Actor.PatientAccountID), NOT by check-in code.
		mux.HandleFunc("/api/v1/patient/appointments", enableCORS(patientH.ListMyAppointments))
		slog.Info("patient portal routes registered", "paths", []string{
			"/api/v1/patient/status",
			"/api/v1/patient/appointments",
		})

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
	//   RequestID → LimitBody → SecurityHeaders → TrustedProxy → DenyByDefault → Auth → Audit → RBAC → mux.
	injectAudit := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.ContextWithAudit(r.Context(), auditSvc)))
		})
	}
	handler := router.SecurityHeaders(router.TrustedProxy(router.DenyByDefault(auth.Middleware(provider)(injectAudit(identity.RequirePermission(mux))))))
	handler = router.LimitRequestBody(256<<10)(handler) // AUDIT-1001: 256KB max request body
	handler = identity.RequestIDMiddleware(handler)

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

// facilitiesNearbyHandler returns alternative facilities sorted by distance +
// availability (for rujukan when target penuh). It reads the facilities
// table — never a hardcoded list — and returns only rows an operator has
// onboarded as real facilities: is_active = true, is_demo = false, with
// coordinates. Bed counts are operator-maintained values, so each row
// carries beds_updated_at: callers must show how stale the number is
// instead of presenting it as live. Demo rows are excluded by default;
// pass include_demo=true to see them (labelled is_demo in the response).
// When the database is unavailable the endpoint fails closed with 503
// instead of serving invented data.
func facilitiesNearbyHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		latStr := r.URL.Query().Get("lat")
		lonStr := r.URL.Query().Get("lon")
		exclude := r.URL.Query().Get("exclude")
		includeDemo := r.URL.Query().Get("include_demo") == "true"

		lat, errLat := strconv.ParseFloat(latStr, 64)
		lon, errLon := strconv.ParseFloat(lonStr, 64)
		if errLat != nil || errLon != nil || lat == 0 || lon == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   "lat and lon query parameters are required and must be valid coordinates",
			})
			return
		}
		if db == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"success": false,
				"error":   "facility data is unavailable",
			})
			return
		}

		rows, err := db.Query(r.Context(), `
			SELECT id, name, short_code, latitude, longitude,
			       available_beds, total_beds, last_updated, is_demo
			FROM facilities
			WHERE is_active = true
			  AND latitude IS NOT NULL AND longitude IS NOT NULL
			  AND available_beds > 0
			  AND ($1 OR NOT is_demo)
			ORDER BY (
			  6371.0 * 2 * ASIN(SQRT(
			    POWER(SIN(RADIANS(latitude - $2) / 2), 2) +
			    COS(RADIANS($2)) * COS(RADIANS(latitude)) *
			    POWER(SIN(RADIANS(longitude - $3) / 2), 2)
			  ))
			) ASC
			LIMIT 20
		`, includeDemo, lat, lon)
		if err != nil {
			slog.Error("facilities/nearby query failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"success": false,
				"error":   "facility data is unavailable",
			})
			return
		}
		defer rows.Close()

		type out struct {
			ID        string  `json:"id"`
			Name      string  `json:"name"`
			Short     string  `json:"short_code"`
			Lat       float64 `json:"lat"`
			Lon       float64 `json:"lon"`
			Avail     int     `json:"available_beds"`
			Total     int     `json:"total_beds"`
			DistKm    float64 `json:"dist_km"`
			BedsUpdAt string  `json:"beds_updated_at"`
			IsDemo    bool    `json:"is_demo"`
		}
		res := make([]out, 0, 20)
		for rows.Next() {
			var o out
			var updAt time.Time
			if err := rows.Scan(&o.ID, &o.Name, &o.Short, &o.Lat, &o.Lon, &o.Avail, &o.Total, &updAt, &o.IsDemo); err != nil {
				slog.Error("facilities/nearby row scan failed", "error", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"success": false,
					"error":   "facility data is unavailable",
				})
				return
			}
			if o.ID == exclude {
				continue
			}
			o.DistKm = haversine(lat, lon, o.Lat, o.Lon)
			o.BedsUpdAt = updAt.UTC().Format(time.RFC3339)
			res = append(res, o)
		}
		if err := rows.Err(); err != nil {
			slog.Error("facilities/nearby rows failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"success": false,
				"error":   "facility data is unavailable",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": res})
	}
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
