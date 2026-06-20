package handler

import (
	"encoding/json"
	"fmt"
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
	mux.HandleFunc("/api/v1/admin/facilities/", downstream)
	mux.HandleFunc("/api/v1/admin/queues", downstream)
	mux.HandleFunc("/api/v1/admin/queues/", downstream)
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
			Permissions: []string{"facility.read"},
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

	// --- 5. POST requires facility.manage ---
	t.Run("POST with read-only gets 403", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-user",
			Permissions: []string{"facility.read"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/facilities", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if reached {
			t.Error("read-only request reached downstream POST")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST read-only: got %d want 403", rec.Code)
		}
	})

	// --- 6. PATCH requires facility.manage ---
	t.Run("PATCH with read-only gets 403", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-user",
			Permissions: []string{"facility.read"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/facilities/550e8400-e29b-41d4-a716-446655440000", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if reached {
			t.Error("read-only request reached downstream PATCH")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("PATCH read-only: got %d want 403", rec.Code)
		}
	})

	// --- 7. GET detail with facility.read gets 200 ---
	t.Run("GET detail with facility.read gets 200", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-user",
			Permissions: []string{"facility.read"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities/550e8400-e29b-41d4-a716-446655440000", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !reached {
			t.Error("facility.read request did not reach downstream detail GET")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("GET detail: got %d want 200", rec.Code)
		}
	})

	// --- 8. POST with facility.manage gets 200 ---
	t.Run("POST with facility.manage gets 200", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-admin",
			Permissions: []string{"facility.manage"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/facilities", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !reached {
			t.Error("facility.manage request did not reach downstream POST")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("POST manage: got %d want 200", rec.Code)
		}
	})

	// --- 9. PATCH deactivate with facility.manage gets 200 ---
	t.Run("PATCH deactivate with facility.manage gets 200", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-admin",
			Permissions: []string{"facility.manage"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/facilities/550e8400-e29b-41d4-a716-446655440000/deactivate", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !reached {
			t.Error("facility.manage request did not reach downstream PATCH deactivate")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("PATCH deactivate: got %d want 200", rec.Code)
		}
	})

	// --- Queue permission tests ---

	// --- 10. GET queue list with queue.read gets 200 ---
	t.Run("GET queue list with queue.read gets 200", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-operator",
			Permissions: []string{"queue.read"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/queues", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !reached {
			t.Error("queue.read request did not reach downstream GET queues")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("GET queues: got %d want 200", rec.Code)
		}
	})

	// --- 11. GET queue detail with queue.read gets 200 ---
	t.Run("GET queue detail with queue.read gets 200", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-operator",
			Permissions: []string{"queue.read"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/queues/550e8400-e29b-41d4-a716-446655440000", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !reached {
			t.Error("queue.read request did not reach downstream GET queue detail")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("GET queue detail: got %d want 200", rec.Code)
		}
	})

	// --- 12. PATCH queue status without queue.manage gets 403 ---
	t.Run("PATCH queue status without queue.manage gets 403", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-viewer",
			Permissions: []string{"queue.read"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/queues/550e8400-e29b-41d4-a716-446655440000/status", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if reached {
			t.Error("read-only request reached downstream PATCH queue status")
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("PATCH queue status read-only: got %d want 403", rec.Code)
		}
	})

	// --- 13. PATCH queue status with queue.manage gets 200 ---
	t.Run("PATCH queue status with queue.manage gets 200", func(t *testing.T) {
		provider := &staticActorProvider{actor: identity.Actor{
			Type:        identity.ActorSystem,
			UserID:      "test-operator",
			Permissions: []string{"queue.manage"},
		}}
		chain := router.DenyByDefault(
			auth.Middleware(provider)(
				identity.RequirePermission(mux)))

		reached = false
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/queues/550e8400-e29b-41d4-a716-446655440000/status", nil)
		rec := httptest.NewRecorder()
		chain.ServeHTTP(rec, req)

		if !reached {
			t.Error("queue.manage request did not reach downstream PATCH queue status")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("PATCH queue status manage: got %d want 200", rec.Code)
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

func TestValidateCreateFacility(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := CreateFacilityRequest{
			Name: "RS Test", Type: "rumah_sakit",
			Address: "Jl. A", Kecamatan: "Kec", KabupatenKota: "Kab",
			Provinsi: "Prov", Phone: "081234567890", TotalBeds: 10,
			AvailableBeds: 5, ShortCode: "RST",
		}
		if err := validateCreateFacility(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("missing name", func(t *testing.T) {
		req := CreateFacilityRequest{Name: "   ", Type: "rumah_sakit", Address: "Jl. A", Kecamatan: "Kec", KabupatenKota: "Kab", Provinsi: "Prov", Phone: "0812", TotalBeds: 1, AvailableBeds: 0, ShortCode: "A"}
		if validateCreateFacility(req) == nil {
			t.Error("expected error for empty name")
		}
	})
	t.Run("invalid type", func(t *testing.T) {
		req := CreateFacilityRequest{Name: "RS Test", Type: "apotek", Address: "Jl. A", Kecamatan: "Kec", KabupatenKota: "Kab", Provinsi: "Prov", Phone: "0812", TotalBeds: 1, AvailableBeds: 0, ShortCode: "A"}
		if validateCreateFacility(req) == nil {
			t.Error("expected error for invalid type")
		}
	})
	t.Run("negative beds", func(t *testing.T) {
		req := CreateFacilityRequest{Name: "RS Test", Type: "rumah_sakit", Address: "Jl. A", Kecamatan: "Kec", KabupatenKota: "Kab", Provinsi: "Prov", Phone: "0812", TotalBeds: -1, AvailableBeds: 0, ShortCode: "A"}
		if validateCreateFacility(req) == nil {
			t.Error("expected error for negative total_beds")
		}
	})
	t.Run("available > total", func(t *testing.T) {
		req := CreateFacilityRequest{Name: "RS Test", Type: "rumah_sakit", Address: "Jl. A", Kecamatan: "Kec", KabupatenKota: "Kab", Provinsi: "Prov", Phone: "0812", TotalBeds: 5, AvailableBeds: 10, ShortCode: "A"}
		if validateCreateFacility(req) == nil {
			t.Error("expected error when available > total")
		}
	})
	t.Run("xss in phone", func(t *testing.T) {
		req := CreateFacilityRequest{Name: "RS Test", Type: "rumah_sakit", Address: "Jl. A", Kecamatan: "Kec", KabupatenKota: "Kab", Provinsi: "Prov", Phone: "<script>", TotalBeds: 1, AvailableBeds: 0, ShortCode: "A"}
		if validateCreateFacility(req) == nil {
			t.Error("expected error for XSS phone")
		}
	})
}

func TestValidateUpdateFacility(t *testing.T) {
	name := "RS"
	invalidType := "unknown"
	total := 10
	avail := 15
	t.Run("valid partial", func(t *testing.T) {
		req := UpdateFacilityRequest{Name: &name}
		if err := validateUpdateFacility(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid type", func(t *testing.T) {
		req := UpdateFacilityRequest{Type: &invalidType}
		if validateUpdateFacility(req) == nil {
			t.Error("expected error for invalid type")
		}
	})
	t.Run("available > total", func(t *testing.T) {
		req := UpdateFacilityRequest{TotalBeds: &total, AvailableBeds: &avail}
		if validateUpdateFacility(req) == nil {
			t.Error("expected error when available > total")
		}
	})
}

func TestExtractFacilityID(t *testing.T) {
	t.Run("valid id", func(t *testing.T) {
		id := "550e8400-e29b-41d4-a716-446655440000"
		got := extractFacilityID("/api/v1/admin/facilities/" + id)
		if got != id {
			t.Errorf("got %s want %s", got, id)
		}
	})
	t.Run("deactivate suffix", func(t *testing.T) {
		id := "550e8400-e29b-41d4-a716-446655440000"
		got := extractFacilityID("/api/v1/admin/facilities/" + id + "/deactivate")
		if got != id {
			t.Errorf("got %s want %s", got, id)
		}
	})
	t.Run("invalid uuid", func(t *testing.T) {
		if extractFacilityID("/api/v1/admin/facilities/not-a-uuid") != "" {
			t.Error("expected empty for invalid uuid")
		}
	})
	t.Run("empty", func(t *testing.T) {
		if extractFacilityID("/api/v1/admin/facilities/") != "" {
			t.Error("expected empty for trailing slash only")
		}
	})
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

func TestIsValidQueueTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{"waiting", "called", true},
		{"waiting", "cancelled", true},
		{"waiting", "in_service", false},
		{"called", "in_service", true},
		{"called", "cancelled", true},
		{"called", "skipped", true},
		{"called", "completed", false},
		{"in_service", "completed", true},
		{"in_service", "cancelled", false},
		{"completed", "called", false},
		{"cancelled", "called", false},
		{"skipped", "waiting", false},
		{"unknown", "called", false},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s→%s", c.from, c.to), func(t *testing.T) {
			got := isValidQueueTransition(c.from, c.to)
			if got != c.want {
				t.Errorf("isValidQueueTransition(%q,%q)=%v want %v", c.from, c.to, got, c.want)
			}
		})
	}
}

func TestExtractQueueTicketID(t *testing.T) {
	t.Run("valid id", func(t *testing.T) {
		id := "550e8400-e29b-41d4-a716-446655440000"
		got := extractQueueTicketID("/api/v1/admin/queues/" + id)
		if got != id {
			t.Errorf("got %s want %s", got, id)
		}
	})
	t.Run("status suffix", func(t *testing.T) {
		id := "550e8400-e29b-41d4-a716-446655440000"
		got := extractQueueTicketID("/api/v1/admin/queues/" + id + "/status")
		if got != id {
			t.Errorf("got %s want %s", got, id)
		}
	})
	t.Run("invalid uuid", func(t *testing.T) {
		if extractQueueTicketID("/api/v1/admin/queues/not-a-uuid") != "" {
			t.Error("expected empty for invalid uuid")
		}
	})
	t.Run("empty", func(t *testing.T) {
		if extractQueueTicketID("/api/v1/admin/queues/") != "" {
			t.Error("expected empty for trailing slash only")
		}
	})
}
