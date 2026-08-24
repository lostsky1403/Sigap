package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRegistry_PHIRoutesHavePolicy is the seed of the Phase-1 CI gate: every
// route that serves PHI MUST declare a non-empty RequiredPolicy.
func TestRegistry_PHIRoutesHavePolicy(t *testing.T) {
	if len(Registry) == 0 {
		t.Fatal("route registry is empty")
	}
	for _, rt := range Registry {
		if rt.PHI && rt.RequiredPolicy == "" {
			t.Errorf("PHI route %s %s has empty RequiredPolicy", rt.Method, rt.Path)
		}
	}
}

// TestRegistry_NoEmptyFields guards against malformed declarations.
func TestRegistry_NoEmptyFields(t *testing.T) {
	for i, rt := range Registry {
		if rt.Method == "" || rt.Path == "" {
			t.Errorf("registry[%d] has empty Method or Path: %+v", i, rt)
		}
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{http.MethodPost, "/api/v1/queues/generate", true},
		{http.MethodGet, "/api/v1/queues/generate", false}, // wrong method
		{http.MethodGet, "/api/v1/records/081234567890", false}, // PHI route removed (AUDIT-803)
		{http.MethodGet, "/api/v1/facilities/nearby", true},
		{http.MethodGet, "/api/v1/public/facilities", true},
		{http.MethodGet, "/api/v1/public/service-units", true},
		{http.MethodGet, "/api/v1/admin/facilities", true},
		{http.MethodGet, "/api/v1/admin/facilities/550e8400-e29b-41d4-a716-446655440000", true},
		{http.MethodPost, "/api/v1/admin/facilities", true},
		{http.MethodPatch, "/api/v1/admin/facilities/550e8400-e29b-41d4-a716-446655440000", true},
		{http.MethodGet, "/api/v1/admin/queues", true},
		{http.MethodGet, "/api/v1/admin/queues/550e8400-e29b-41d4-a716-446655440000", true},
		{http.MethodPatch, "/api/v1/admin/queues/550e8400-e29b-41d4-a716-446655440000/status", true},
		{http.MethodGet, "/api/v1/admin/service-units", true},
		{http.MethodGet, "/api/v1/admin/service-units/550e8400-e29b-41d4-a716-446655440000", true},
		{http.MethodPost, "/api/v1/admin/service-units", true},
		{http.MethodPatch, "/api/v1/admin/service-units/550e8400-e29b-41d4-a716-446655440000", true},
		{http.MethodGet, "/api/v1/admin/schedules", true},
		{http.MethodGet, "/api/v1/admin/schedules/550e8400-e29b-41d4-a716-446655440000", true},
		{http.MethodPost, "/api/v1/admin/schedules", true},
		{http.MethodPatch, "/api/v1/admin/schedules/550e8400-e29b-41d4-a716-446655440000", true},
		{http.MethodGet, "/api/v1/admin/appointments", true},
		{http.MethodPatch, "/api/v1/admin/appointments/550e8400-e29b-41d4-a716-446655440000/status", true},
		{http.MethodGet, "/api/v1/unknown", false},
	}
	for _, c := range cases {
		if _, ok := Match(c.method, c.path); ok != c.want {
			t.Errorf("Match(%s,%s)=%v want %v", c.method, c.path, ok, c.want)
		}
	}
}

func TestDenyByDefault(t *testing.T) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	h := DenyByDefault(next)

	t.Run("undeclared route returns 401", func(t *testing.T) {
		reached = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("got %d want 401", rec.Code)
		}
		if reached {
			t.Error("undeclared route reached downstream handler")
		}
	})

	t.Run("declared route passes through", func(t *testing.T) {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/api/v1/queues/generate", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !reached {
			t.Error("declared route did not reach downstream handler")
		}
	})

	t.Run("allow-listed health passes", func(t *testing.T) {
		reached = false
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !reached {
			t.Error("/health was blocked")
		}
	})

	t.Run("OPTIONS preflight passes", func(t *testing.T) {
		reached = false
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/queues/generate", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !reached {
			t.Error("OPTIONS preflight was blocked")
		}
	})
}
