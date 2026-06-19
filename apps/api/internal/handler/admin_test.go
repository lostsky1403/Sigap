package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/router"
)

// TestAdminBoundary_AuthScenarios exercises the full middleware chain
// (DenyByDefault → AuthProvider → RequirePermission) against the admin
// route with four expected outcomes:
//   1. Unauthenticated → 401 (DenyByDefault) or 403 (RequirePermission)
//   2. Wrong permission → 403
//   3. Correct permission → 200
//   4. Public route (/health) → 200
func TestAdminBoundary_AuthScenarios(t *testing.T) {
	// Build a tiny downstream handler that records whether it was reached.
	reached := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/facilities", downstream)
	mux.HandleFunc("/health", downstream)

	// --- 1. Unauthenticated → 403 (missing actor) ---
	t.Run("unauthenticated gets 403", func(t *testing.T) {
		// AuthProvider that always returns a zero actor (unauthenticated).
		provider := &staticActorProvider{actor: identity.Actor{}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if reached {
			t.Error("unauthenticated request reached downstream")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("unauthenticated: got %d want 403", rec.Code)
		}
		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["success"] != false {
			t.Errorf("body.success=%v want false", body["success"])
		}
	})

	// --- 2. Wrong permission → 403 ---
	t.Run("wrong permission gets 403", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-user",
			Permissions: []string{"queue.generate"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if reached {
			t.Error("wrong-permission request reached downstream")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("wrong permission: got %d want 403", rec.Code)
		}
	})

	// --- 3. Correct permission → 200 ---
	t.Run("correct permission gets 200", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-admin",
			Permissions: []string{"facility.manage"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !reached {
			t.Error("correct-permission request did not reach downstream")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("correct permission: got %d want 200", rec.Code)
		}
	})

	// --- 4. Public route (/health) → 200 ---
	t.Run("public route gets 200", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !reached {
			t.Error("public request did not reach downstream")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("public route: got %d want 200", rec.Code)
		}
	})
}

// TestAdminBoundary_DevIdentityHeader exercises the dev identity provider
// end-to-end: with SIGAP_DEV_IDENTITY=true and the X-Sigap-Dev-User-ID
// header present, the admin route should be reachable.
func TestAdminBoundary_DevIdentityHeader(t *testing.T) {
	// Enable dev identity.
	t.Setenv("SIGAP_DEV_IDENTITY", "true")
	provider := auth.NewDevIdentityProvider()

	reached := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/facilities", downstream)

	chain := router.DenyByDefault(
		auth.Middleware(provider)(
			identity.RequirePermission(mux)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	req.Header.Set("X-Sigap-Dev-User-ID", "integration-tester")
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	if !reached {
		t.Error("dev identity request did not reach downstream")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("dev identity: got %d want 200", rec.Code)
	}

	// Verify the actor had the facility.manage permission (dev identity grants all).
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["success"] != true {
		t.Errorf("body.success=%v want true", body["success"])
	}
}

// TestAdminBoundary_DevIdentityDisabled exercises that a request with the
// dev identity header but SIGAP_DEV_IDENTITY=false is treated as
// unauthenticated and gets 403.
func TestAdminBoundary_DevIdentityDisabled(t *testing.T) {
	t.Setenv("SIGAP_DEV_IDENTITY", "false")
	provider := auth.NewDevIdentityProvider()

	reached := false
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/facilities", downstream)

	chain := router.DenyByDefault(
		auth.Middleware(provider)(
			identity.RequirePermission(mux)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	req.Header.Set("X-Sigap-Dev-User-ID", "integration-tester")
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	if reached {
		t.Error("disabled dev identity request reached downstream")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("disabled dev identity: got %d want 403", rec.Code)
	}
}

// staticActorProvider is a test helper auth.Provider that always returns the
// same actor. It allows tests to bypass real auth mechanisms (headers, JWT)
// and directly control the actor passed to the authorization layer.
type staticActorProvider struct {
	actor identity.Actor
}

func (p *staticActorProvider) Authenticate(r *http.Request) (identity.Actor, error) {
	return p.actor, nil
}
