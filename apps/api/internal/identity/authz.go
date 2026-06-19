package identity

import (
	"context"
	"net/http"

	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/router"
)

// authzDeniedReasonKey is used to carry a structured denial reason forward to
// the audit middleware so that permission-denied events can be logged without
// re-parsing the response.
type authzDeniedReasonKey struct{}

// auditKey is the context key for the audit service.
type auditKey struct{}

// ContextWithAudit returns ctx with the given audit.Service attached.
func ContextWithAudit(ctx context.Context, a *audit.Service) context.Context {
	return context.WithValue(ctx, auditKey{}, a)
}

// AuditFromContext returns the audit.Service stored in ctx, or nil if none.
func AuditFromContext(ctx context.Context) *audit.Service {
	v, _ := ctx.Value(auditKey{}).(*audit.Service)
	return v
}

// deniedReason holds the pieces an audit logger needs when authz rejects a request.
type deniedReason struct {
	Route  router.Route
	Actor  Actor
	Reason string
}

// RequirePermission enforces per-route access control using the router registry.
//
// Execution order:
//  1. Allow-listed paths bypass authorization entirely.
//  2. Unregistered routes are forwarded; DenyByDefault handles them later.
//  3. Registered routes with RequiredPolicy=="" are allowed (backward
//     compatible until the route is explicitly gated).
//  4. Registered routes with RequiredPolicy!="" require an Actor in context
//     that holds that exact permission key.
//
// On denial the middleware writes a 403 JSON response and stores a
// deniedReason in context for an upstream audit logger.
func RequirePermission(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if router.IsAllowListed(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		route, ok := router.Match(r.Method, r.URL.Path)
		if !ok {
			// Unknown route — let DenyByDefault reject it.
			next.ServeHTTP(w, r)
			return
		}

		if route.RequiredPolicy == "" {
			// No policy declared yet; allow for backward compatibility.
			next.ServeHTTP(w, r)
			return
		}

		actor := ActorFromContext(r.Context())
		if actor.IsZero() {
			ctx := context.WithValue(r.Context(), authzDeniedReasonKey{}, deniedReason{
				Route:  route,
				Actor:  actor,
				Reason: "missing actor",
			})
			r = r.WithContext(ctx)
			logAuthzDenied(ctx, route, actor, "missing actor")
			writeAuthzError(w, http.StatusForbidden, "Akses ditolak: autentikasi diperlukan.")
			return
		}

		if !actor.HasPermission(route.RequiredPolicy) {
			ctx := context.WithValue(r.Context(), authzDeniedReasonKey{}, deniedReason{
				Route:  route,
				Actor:  actor,
				Reason: "missing permission: " + route.RequiredPolicy,
			})
			r = r.WithContext(ctx)
			logAuthzDenied(ctx, route, actor, "missing permission: "+route.RequiredPolicy)
			writeAuthzError(w, http.StatusForbidden, "Akses ditolak: izin tidak mencukupi.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// writeAuthzError writes a consistent JSON error for authorization failures.
func writeAuthzError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"success":false,"error":"` + msg + `"}`))
}

// DeniedReason extracts the structured denial reason from ctx if one was
// previously stored by RequirePermission.
func DeniedReason(ctx context.Context) (deniedReason, bool) {
	v, ok := ctx.Value(authzDeniedReasonKey{}).(deniedReason)
	return v, ok
}

// logAuthzDenied writes an audit event for authorization rejections.
// It is best-effort: failures are silently ignored.
func logAuthzDenied(ctx context.Context, route router.Route, actor Actor, reason string) {
	auditSvc := AuditFromContext(ctx)
	if auditSvc == nil {
		return
	}
	resourceID := route.Method + " " + route.Path
	auditSvc.LogEvent(ctx, audit.Event{
		Action:       "authz.denied",
		ResourceType: "authz",
		ResourceID:   resourceID,
		ActorType:    string(actor.Type),
		ActorUserID:  actor.UserID,
		RequestID:    RequestIDFromContext(ctx),
		Metadata: audit.SanitizeMetadata(map[string]any{
			"reason":   reason,
			"required": route.RequiredPolicy,
			"path":     route.Path,
			"method":   route.Method,
		}),
	})
}
