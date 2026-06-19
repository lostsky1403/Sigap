// Package router declares the API surface as data so access control can be
// reasoned about in one place. Each route records whether it serves PHI and the
// authorization policy that will gate it. A deny-by-default middleware rejects
// any path/method that is neither declared here nor explicitly allow-listed.
//
// Today the middleware enforces only that a route is declared; per-route policy
// enforcement (RBAC) and audit logging are layered on later without changing
// call sites. This is the prerequisite seam for those phases.
package router

import (
	"net/http"
	"strings"
)

// Route declares a single API endpoint and its security posture.
type Route struct {
	Method         string // HTTP method, e.g. http.MethodGet
	Path           string // exact path, or path prefix when Prefix is true
	Prefix         bool   // match Path as a prefix (for parameterized paths)
	PHI            bool   // true when the response may contain patient data
	RequiredPolicy string // authz policy name; MUST be non-empty when PHI is true
}

// AllowList paths bypass authorization entirely. Restricted to liveness and
// readiness probes, which expose no data.
var AllowList = []string{"/health", "/readyz"}

// Registry is the single source of truth for the declared API surface.
// Adding a route here is what makes it reachable through DenyByDefault.
var Registry = []Route{
	{Method: http.MethodPost, Path: "/api/v1/queues/generate"},
	{Method: http.MethodGet, Path: "/api/v1/events/beds"},
	{Method: http.MethodGet, Path: "/api/v1/facilities/nearby"},
	{Method: http.MethodGet, Path: "/api/v1/medical-records", PHI: true, RequiredPolicy: "medical_records:read"},
	{Method: http.MethodGet, Path: "/api/v1/records/", Prefix: true, PHI: true, RequiredPolicy: "medical_records:read"},
	{Method: http.MethodGet, Path: "/api/v1/admin/facilities", RequiredPolicy: "facility.manage"},
}

// IsAllowListed reports whether a path bypasses authorization.
func IsAllowListed(path string) bool {
	for _, p := range AllowList {
		if path == p {
			return true
		}
	}
	return false
}

// Match reports whether a method+path corresponds to a declared route.
func Match(method, path string) (Route, bool) {
	for _, rt := range Registry {
		if rt.Method != method {
			continue
		}
		if rt.Prefix {
			if strings.HasPrefix(path, rt.Path) {
				return rt, true
			}
			continue
		}
		if path == rt.Path {
			return rt, true
		}
	}
	return Route{}, false
}

// DenyByDefault rejects any request whose method+path is neither allow-listed
// nor present in the Registry, returning 401. CORS preflight (OPTIONS) carries
// no data and is forwarded so the CORS layer can answer it.
func DenyByDefault(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if IsAllowListed(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := Match(r.Method, r.URL.Path); ok {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"error":"Akses ditolak: rute tidak dikenali atau memerlukan otorisasi."}`))
	})
}
