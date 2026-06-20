package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// --- Service unit tests ---

func TestListServiceUnits(t *testing.T) {
	// Auth boundary is exercised by TestAdminBoundary_AuthScenarios.
	// This test validates that the handler method returns an error when DB is nil.
	// In practice integration tests need a real DB; here we verify structural correctness.
	h := NewAdminHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-units", nil)
	rec := httptest.NewRecorder()

	h.ListServiceUnits(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("ListServiceUnits with nil pool: got %d want 500", rec.Code)
	}
}

func TestGetServiceUnit(t *testing.T) {
	h := NewAdminHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-units/550e8400-e29b-41d4-a716-446655440000", nil)
	rec := httptest.NewRecorder()

	h.GetServiceUnit(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GetServiceUnit with nil pool: got %d want 500", rec.Code)
	}
}

func TestCreateServiceUnit(t *testing.T) {
	h := NewAdminHandler(nil)
	body := map[string]any{"name": "Poli Umum", "facility_id": "550e8400-e29b-41d4-a716-446655440000"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-units", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CreateServiceUnit(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("CreateServiceUnit with nil pool: got %d want 500", rec.Code)
	}
}

func TestUpdateServiceUnit(t *testing.T) {
	h := NewAdminHandler(nil)
	body := map[string]any{"name": "Poli Umum Updated"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-units/550e8400-e29b-41d4-a716-446655440000", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.UpdateServiceUnit(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("UpdateServiceUnit with nil pool: got %d want 500", rec.Code)
	}
}

func TestValidateCreateServiceUnit(t *testing.T) {
	validFacility := "550e8400-e29b-41d4-a716-446655440000"
	t.Run("valid", func(t *testing.T) {
		req := CreateServiceUnitRequest{Name: "Poli Umum", FacilityID: validFacility}
		if err := validateCreateServiceUnit(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("valid with code", func(t *testing.T) {
		req := CreateServiceUnitRequest{Name: "Poli Umum", FacilityID: validFacility, Code: "PU001"}
		if err := validateCreateServiceUnit(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("missing name", func(t *testing.T) {
		req := CreateServiceUnitRequest{Name: "   ", FacilityID: validFacility}
		if validateCreateServiceUnit(req) == nil {
			t.Error("expected error for empty name")
		}
	})
	t.Run("name too long", func(t *testing.T) {
		req := CreateServiceUnitRequest{Name: strings.Repeat("a", 101), FacilityID: validFacility}
		if validateCreateServiceUnit(req) == nil {
			t.Error("expected error for name > 100")
		}
	})
	t.Run("invalid facility_id", func(t *testing.T) {
		req := CreateServiceUnitRequest{Name: "Poli Umum", FacilityID: "not-a-uuid"}
		if validateCreateServiceUnit(req) == nil {
			t.Error("expected error for invalid facility_id")
		}
	})
	t.Run("code too long", func(t *testing.T) {
		req := CreateServiceUnitRequest{Name: "Poli Umum", FacilityID: validFacility, Code: strings.Repeat("a", 21)}
		if validateCreateServiceUnit(req) == nil {
			t.Error("expected error for code > 20")
		}
	})
	t.Run("empty code ok", func(t *testing.T) {
		req := CreateServiceUnitRequest{Name: "Poli Umum", FacilityID: validFacility, Code: ""}
		if err := validateCreateServiceUnit(req); err != nil {
			t.Errorf("unexpected error for empty code: %v", err)
		}
	})
}

func TestValidateUpdateServiceUnit(t *testing.T) {
	validFacility := "550e8400-e29b-41d4-a716-446655440000"
	name := "Poli Gigi"
	code := "PG"
	invalidFacility := "bad-uuid"
	longCode := strings.Repeat("a", 21)
	t.Run("valid partial", func(t *testing.T) {
		req := UpdateServiceUnitRequest{Name: &name}
		if err := validateUpdateServiceUnit(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("valid facility_id", func(t *testing.T) {
		req := UpdateServiceUnitRequest{FacilityID: &validFacility}
		if err := validateUpdateServiceUnit(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid facility_id", func(t *testing.T) {
		req := UpdateServiceUnitRequest{FacilityID: &invalidFacility}
		if validateUpdateServiceUnit(req) == nil {
			t.Error("expected error for invalid facility_id")
		}
	})
	t.Run("empty name", func(t *testing.T) {
		empty := ""
		req := UpdateServiceUnitRequest{Name: &empty}
		if validateUpdateServiceUnit(req) == nil {
			t.Error("expected error for empty name")
		}
	})
	t.Run("name too long", func(t *testing.T) {
		long := strings.Repeat("a", 101)
		req := UpdateServiceUnitRequest{Name: &long}
		if validateUpdateServiceUnit(req) == nil {
			t.Error("expected error for name > 100")
		}
	})
	t.Run("code too long", func(t *testing.T) {
		req := UpdateServiceUnitRequest{Code: &longCode}
		if validateUpdateServiceUnit(req) == nil {
			t.Error("expected error for code > 20")
		}
	})
	t.Run("valid code", func(t *testing.T) {
		req := UpdateServiceUnitRequest{Code: &code}
		if err := validateUpdateServiceUnit(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("empty code ok", func(t *testing.T) {
		empty := ""
		req := UpdateServiceUnitRequest{Code: &empty}
		if err := validateUpdateServiceUnit(req); err != nil {
			t.Errorf("unexpected error for empty code: %v", err)
		}
	})
}

func TestExtractServiceUnitID(t *testing.T) {
	t.Run("valid id", func(t *testing.T) {
		id := "550e8400-e29b-41d4-a716-446655440000"
		got := extractServiceUnitID("/api/v1/admin/service-units/" + id)
		if got != id {
			t.Errorf("got %s want %s", got, id)
		}
	})
	t.Run("invalid uuid", func(t *testing.T) {
		if extractServiceUnitID("/api/v1/admin/service-units/not-a-uuid") != "" {
			t.Error("expected empty for invalid uuid")
		}
	})
	t.Run("empty", func(t *testing.T) {
		if extractServiceUnitID("/api/v1/admin/service-units/") != "" {
			t.Error("expected empty for trailing slash only")
		}
	})
}

// --- Schedule handler tests ---

func TestListSchedules(t *testing.T) {
	h := NewAdminHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules", nil)
	rec := httptest.NewRecorder()

	h.ListSchedules(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("ListSchedules with nil pool: got %d want 500", rec.Code)
	}
}

func TestGetSchedule(t *testing.T) {
	h := NewAdminHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/550e8400-e29b-41d4-a716-446655440000", nil)
	rec := httptest.NewRecorder()

	h.GetSchedule(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("GetSchedule with nil pool: got %d want 500", rec.Code)
	}
}

func TestCreateSchedule(t *testing.T) {
	h := NewAdminHandler(nil)
	body := map[string]any{
		"facility_id":       "550e8400-e29b-41d4-a716-446655440000",
		"service_unit_id":   "550e8400-e29b-41d4-a716-446655440001",
		"schedule_date":     "2026-12-01",
		"start_time":        "08:00",
		"end_time":          "16:00",
		"slot_minutes":      30,
		"capacity_per_slot": 5,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/schedules", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CreateSchedule(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("CreateSchedule with nil pool: got %d want 500", rec.Code)
	}
}

func TestUpdateSchedule(t *testing.T) {
	h := NewAdminHandler(nil)
	body := map[string]any{"slot_minutes": 60}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/schedules/550e8400-e29b-41d4-a716-446655440000", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.UpdateSchedule(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("UpdateSchedule with nil pool: got %d want 500", rec.Code)
	}
}

func TestValidateCreateSchedule(t *testing.T) {
	validFacility := "550e8400-e29b-41d4-a716-446655440000"
	validUnit := "550e8400-e29b-41d4-a716-446655440001"
	t.Run("valid", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "08:00", EndTime: "10:00",
			SlotMinutes: 30, CapacityPerSlot: 5,
		}
		if err := validateCreateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("valid with practitioner", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, PractitionerID: "550e8400-e29b-41d4-a716-446655440002",
			ServiceUnitID: validUnit, ScheduleDate: "2026-12-01",
			StartTime: "08:00", EndTime: "10:00", SlotMinutes: 30, CapacityPerSlot: 1,
		}
		if err := validateCreateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("valid long time format", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "08:00:00", EndTime: "10:00:00",
			SlotMinutes: 60, CapacityPerSlot: 5,
		}
		if err := validateCreateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid facility", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: "bad", ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "08:00", EndTime: "10:00",
			SlotMinutes: 30, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for invalid facility_id")
		}
	})
	t.Run("invalid practitioner", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, PractitionerID: "bad",
			ServiceUnitID: validUnit, ScheduleDate: "2026-12-01",
			StartTime: "08:00", EndTime: "10:00", SlotMinutes: 30, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for invalid practitioner_id")
		}
	})
	t.Run("missing date", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "", StartTime: "08:00", EndTime: "10:00",
			SlotMinutes: 30, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for missing schedule_date")
		}
	})
	t.Run("invalid date format", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "12-01-2026", StartTime: "08:00", EndTime: "10:00",
			SlotMinutes: 30, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for invalid date format")
		}
	})
	t.Run("missing start time", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "", EndTime: "10:00",
			SlotMinutes: 30, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for missing start_time")
		}
	})
	t.Run("invalid time format", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "8:00", EndTime: "10:00",
			SlotMinutes: 30, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for invalid time format")
		}
	})
	t.Run("end before start", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "10:00", EndTime: "08:00",
			SlotMinutes: 30, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for end_time <= start_time")
		}
	})
	t.Run("slot not dividing", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "08:00", EndTime: "09:00",
			SlotMinutes: 25, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for slot_minutes not dividing range")
		}
	})
	t.Run("slot too small", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "08:00", EndTime: "10:00",
			SlotMinutes: 3, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for slot_minutes < 5")
		}
	})
	t.Run("slot too large", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "08:00", EndTime: "10:00",
			SlotMinutes: 200, CapacityPerSlot: 5,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for slot_minutes > 180")
		}
	})
	t.Run("zero capacity", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "08:00", EndTime: "10:00",
			SlotMinutes: 30, CapacityPerSlot: 0,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for capacity_per_slot <= 0")
		}
	})
	t.Run("capacity too large", func(t *testing.T) {
		req := CreateScheduleRequest{
			FacilityID: validFacility, ServiceUnitID: validUnit,
			ScheduleDate: "2026-12-01", StartTime: "08:00", EndTime: "10:00",
			SlotMinutes: 30, CapacityPerSlot: 101,
		}
		if validateCreateSchedule(req) == nil {
			t.Error("expected error for capacity_per_slot > 100")
		}
	})
}

func TestValidateUpdateSchedule(t *testing.T) {
	validFacility := "550e8400-e29b-41d4-a716-446655440000"
	validUnit := "550e8400-e29b-41d4-a716-446655440001"
	validPrac := "550e8400-e29b-41d4-a716-446655440002"
	date := "2026-12-01"
	start := "09:00"
	end := "11:00"
	slot := 60
	cap := 10
	active := true

	t.Run("valid partial", func(t *testing.T) {
		req := UpdateScheduleRequest{SlotMinutes: &slot}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("valid facility_id", func(t *testing.T) {
		req := UpdateScheduleRequest{FacilityID: &validFacility}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid facility_id", func(t *testing.T) {
		bad := "bad"
		req := UpdateScheduleRequest{FacilityID: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for invalid facility_id")
		}
	})
	t.Run("set null practitioner", func(t *testing.T) {
		empty := ""
		req := UpdateScheduleRequest{PractitionerID: &empty}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("valid practitioner", func(t *testing.T) {
		req := UpdateScheduleRequest{PractitionerID: &validPrac}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid practitioner", func(t *testing.T) {
		bad := "not-uuid"
		req := UpdateScheduleRequest{PractitionerID: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for invalid practitioner_id")
		}
	})
	t.Run("valid service_unit_id", func(t *testing.T) {
		req := UpdateScheduleRequest{ServiceUnitID: &validUnit}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid service_unit_id", func(t *testing.T) {
		bad := "not-uuid"
		req := UpdateScheduleRequest{ServiceUnitID: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for invalid service_unit_id")
		}
	})
	t.Run("valid date", func(t *testing.T) {
		req := UpdateScheduleRequest{ScheduleDate: &date}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid date", func(t *testing.T) {
		bad := "13-13-2026"
		req := UpdateScheduleRequest{ScheduleDate: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for invalid date")
		}
	})
	t.Run("empty date", func(t *testing.T) {
		bad := ""
		req := UpdateScheduleRequest{ScheduleDate: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for empty date")
		}
	})
	t.Run("valid times", func(t *testing.T) {
		req := UpdateScheduleRequest{StartTime: &start, EndTime: &end}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid start_time", func(t *testing.T) {
		bad := "9:00"
		req := UpdateScheduleRequest{StartTime: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for invalid start_time")
		}
	})
	t.Run("invalid end_time", func(t *testing.T) {
		bad := "20-00"
		req := UpdateScheduleRequest{EndTime: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for invalid end_time")
		}
	})
	t.Run("end before start", func(t *testing.T) {
		s := "12:00"
		e := "08:00"
		req := UpdateScheduleRequest{StartTime: &s, EndTime: &e}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for end_time <= start_time")
		}
	})
	t.Run("valid slot_minutes", func(t *testing.T) {
		req := UpdateScheduleRequest{SlotMinutes: &slot}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid slot_minutes", func(t *testing.T) {
		bad := 3
		req := UpdateScheduleRequest{SlotMinutes: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for slot_minutes < 5")
		}
	})
	t.Run("valid capacity_per_slot", func(t *testing.T) {
		req := UpdateScheduleRequest{CapacityPerSlot: &cap}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("invalid capacity_per_slot", func(t *testing.T) {
		bad := 0
		req := UpdateScheduleRequest{CapacityPerSlot: &bad}
		if validateUpdateSchedule(req) == nil {
			t.Error("expected error for capacity_per_slot <= 0")
		}
	})
	t.Run("valid is_active", func(t *testing.T) {
		req := UpdateScheduleRequest{IsActive: &active}
		if err := validateUpdateSchedule(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestExtractScheduleID(t *testing.T) {
	t.Run("valid id", func(t *testing.T) {
		id := "550e8400-e29b-41d4-a716-446655440000"
		got := extractScheduleID("/api/v1/admin/schedules/" + id)
		if got != id {
			t.Errorf("got %s want %s", got, id)
		}
	})
	t.Run("invalid uuid", func(t *testing.T) {
		if extractScheduleID("/api/v1/admin/schedules/not-a-uuid") != "" {
			t.Error("expected empty for invalid uuid")
		}
	})
	t.Run("empty", func(t *testing.T) {
		if extractScheduleID("/api/v1/admin/schedules/") != "" {
			t.Error("expected empty for trailing slash only")
		}
	})
}

func TestListAppointments(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/appointments", nil)
	rec := httptest.NewRecorder()
	NewAdminHandler(nil).ListAppointments(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("ListAppointments with nil pool: got %d want 500", rec.Code)
	}
}

func TestUpdateAppointmentStatus(t *testing.T) {
	h := NewAdminHandler(nil)
	body := map[string]any{"status": "checked_in"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/appointments/550e8400-e29b-41d4-a716-446655440000/status", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.UpdateAppointmentStatus(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("UpdateAppointmentStatus with nil pool: got %d want 500", rec.Code)
	}
}

func TestExtractAppointmentID(t *testing.T) {
	t.Run("valid id with status suffix", func(t *testing.T) {
		id := "550e8400-e29b-41d4-a716-446655440000"
		got := extractAppointmentID("/api/v1/admin/appointments/" + id + "/status")
		if got != id {
			t.Errorf("got %s want %s", got, id)
		}
	})
	t.Run("valid id without suffix", func(t *testing.T) {
		id := "550e8400-e29b-41d4-a716-446655440000"
		got := extractAppointmentID("/api/v1/admin/appointments/" + id)
		if got != id {
			t.Errorf("got %s want %s", got, id)
		}
	})
	t.Run("invalid uuid", func(t *testing.T) {
		if extractAppointmentID("/api/v1/admin/appointments/not-a-uuid/status") != "" {
			t.Error("expected empty for invalid uuid")
		}
	})
	t.Run("empty", func(t *testing.T) {
		if extractAppointmentID("/api/v1/admin/appointments/") != "" {
			t.Error("expected empty for trailing slash only")
		}
	})
}

func TestIsValidAppointmentTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{"scheduled", "checked_in", true},
		{"scheduled", "cancelled", true},
		{"scheduled", "completed", false},
		{"checked_in", "queued", true},
		{"checked_in", "cancelled", true},
		{"checked_in", "scheduled", false},
		{"queued", "completed", true},
		{"queued", "cancelled", true},
		{"queued", "checked_in", false},
		{"completed", "cancelled", false},
		{"cancelled", "scheduled", false},
		{"no_show", "scheduled", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s->%s", tt.from, tt.to), func(t *testing.T) {
			got := isValidAppointmentTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("isValidAppointmentTransition(%q,%q)=%v want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
