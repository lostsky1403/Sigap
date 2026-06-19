package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGuardDemoPHI_ClosedByDefault asserts the PHI demo endpoints are NOT served
// when the gate flag is unset (the production-safe default). This is the
// regression guard for the live unauthenticated PHI exposure (R1, B.3#1).
func TestGuardDemoPHI_ClosedByDefault(t *testing.T) {
	t.Setenv("SIGAP_ENABLE_DEMO_PHI", "")

	called := false
	guarded := guardDemoPHI(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	for _, path := range []string{"/api/v1/medical-records", "/api/v1/records/081234567890"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		guarded(rec, req)

		if rec.Code == http.StatusOK {
			t.Errorf("path %s: unauthenticated GET returned 200; PHI must not be exposed by default", path)
		}
		if rec.Code != http.StatusNotFound {
			t.Errorf("path %s: expected 404 when gate disabled, got %d", path, rec.Code)
		}
	}
	if called {
		t.Error("downstream PHI handler was invoked while gate disabled; it must be short-circuited")
	}
}

// TestGuardDemoPHI_EnabledForDev asserts the gate forwards to the handler only
// when explicitly enabled for local development.
func TestGuardDemoPHI_EnabledForDev(t *testing.T) {
	t.Setenv("SIGAP_ENABLE_DEMO_PHI", "true")

	called := false
	guarded := guardDemoPHI(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/medical-records", nil)
	rec := httptest.NewRecorder()
	guarded(rec, req)

	if !called {
		t.Error("handler not invoked while gate enabled")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 when gate enabled, got %d", rec.Code)
	}
}
