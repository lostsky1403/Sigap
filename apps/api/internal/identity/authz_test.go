package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequirePermission_AllowListed(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	RequirePermission(next).ServeHTTP(rec, req)
	if !called {
		t.Error("allow-listed route was blocked")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequirePermission_Unregistered(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	rec := httptest.NewRecorder()

	RequirePermission(next).ServeHTTP(rec, req)
	if !called {
		t.Error("unregistered route was blocked")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequirePermission_EmptyPolicy_Allowed(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queues/generate", nil)
	rec := httptest.NewRecorder()

	RequirePermission(next).ServeHTTP(rec, req)
	if !called {
		t.Error("empty RequiredPolicy route was blocked")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequirePermission_MissingActor(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	rec := httptest.NewRecorder()

	RequirePermission(next).ServeHTTP(rec, req)
	if called {
		t.Error("missing actor should not reach next")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "autentikasi diperlukan") {
		t.Errorf("expected Indonesian auth error, got %s", rec.Body.String())
	}
}

func TestRequirePermission_MissingPermission(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	actor := Actor{UserID: "u1", Type: ActorUser, Permissions: []string{"queue.generate"}}
	req = req.WithContext(ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	RequirePermission(next).ServeHTTP(rec, req)
	if called {
		t.Error("missing permission should not reach next")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "izin tidak mencukupi") {
		t.Errorf("expected Indonesian permission error, got %s", rec.Body.String())
	}
}

func TestRequirePermission_CorrectPermission(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	actor := Actor{UserID: "u1", Type: ActorUser, Permissions: []string{"facility.read"}}
	req = req.WithContext(ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	RequirePermission(next).ServeHTTP(rec, req)
	if !called {
		t.Error("correct permission route was blocked")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequirePermission_PrefixRoute(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities/550e8400-e29b-41d4-a716-446655440000", nil)
	actor := Actor{UserID: "u1", Type: ActorUser, Permissions: []string{"facility.read"}}
	req = req.WithContext(ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	RequirePermission(next).ServeHTTP(rec, req)
	if !called {
		t.Error("prefix route with correct permission was blocked")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
