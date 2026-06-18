package identity

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDevIdentity_EnabledWithHeader(t *testing.T) {
	t.Setenv(devIdentityEnv, "true")

	var captured Actor
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ActorFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(devIdentityHeader, "dev-user-42")
	rec := httptest.NewRecorder()

	DevIdentity(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if captured.IsZero() {
		t.Fatal("expected actor to be injected")
	}
	if captured.Type != ActorDev {
		t.Errorf("expected Type=ActorDev, got %q", captured.Type)
	}
	if !captured.IsDev {
		t.Error("expected IsDev=true")
	}
	if captured.UserID != "dev-user-42" {
		t.Errorf("expected UserID=dev-user-42, got %q", captured.UserID)
	}
	if !captured.HasPermission("queue.generate") {
		t.Error("expected HasPermission(queue.generate)")
	}
}

func TestDevIdentity_EnabledMissingHeader(t *testing.T) {
	t.Setenv(devIdentityEnv, "true")
	// Ensure any parent env value is cleared
	os.Unsetenv(devIdentityHeader)

	var captured Actor
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ActorFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	DevIdentity(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !captured.IsZero() {
		t.Errorf("expected zero actor when header missing, got %+v", captured)
	}
}

func TestDevIdentity_Disabled(t *testing.T) {
	t.Setenv(devIdentityEnv, "false")

	var captured Actor
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = ActorFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(devIdentityHeader, "dev-user-99")
	rec := httptest.NewRecorder()

	DevIdentity(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !captured.IsZero() {
		t.Errorf("expected zero actor when disabled, got %+v", captured)
	}
}
