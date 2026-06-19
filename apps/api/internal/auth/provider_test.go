package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sigap/sigap/apps/api/internal/identity"
)

// TestDevIdentityProvider_EnabledWithHeader verifies that when
// SIGAP_DEV_IDENTITY=true and the request carries X-Sigap-Dev-User-ID,
// the provider returns an actor with the full synthetic permission set.
func TestDevIdentityProvider_EnabledWithHeader(t *testing.T) {
	t.Setenv(devIdentityEnv, "true")
	p := NewDevIdentityProvider()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(devIdentityHeader, "dev-user-42")

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.UserID != "dev-user-42" {
		t.Errorf("userID = %q, want %q", actor.UserID, "dev-user-42")
	}
	if actor.Type != identity.ActorDev {
		t.Errorf("type = %v, want %v", actor.Type, identity.ActorDev)
	}
	if !actor.IsDev {
		t.Error("IsDev = false, want true")
	}
	wantPerms := []string{
		"queue.generate", "queue.read", "facility.read", "facility.manage", "audit.read",
	}
	for _, perm := range wantPerms {
		if !actor.HasPermission(perm) {
			t.Errorf("missing permission %q", perm)
		}
	}
}

// TestDevIdentityProvider_Disabled verifies that when SIGAP_DEV_IDENTITY
// is not set to "true", the provider returns a zero actor even if the
// header is present.
func TestDevIdentityProvider_Disabled(t *testing.T) {
	t.Setenv(devIdentityEnv, "false")
	p := NewDevIdentityProvider()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(devIdentityHeader, "dev-user-42")

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor, got %+v", actor)
	}
}

// TestDevIdentityProvider_EnabledNoHeader verifies that when enabled but
// the header is missing, a zero actor is returned.
func TestDevIdentityProvider_EnabledNoHeader(t *testing.T) {
	t.Setenv(devIdentityEnv, "true")
	p := NewDevIdentityProvider()

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor, got %+v", actor)
	}
}

// TestLoadConfigFromEnv_ValidModes exercises config parsing for each
// supported mode and verifies field requirements.
func TestLoadConfigFromEnv_ValidModes(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *AuthConfig
		wantErr bool
	}{
		{
			name: "dev mode",
			env:  map[string]string{"SIGAP_AUTH_MODE": "dev"},
			want: &AuthConfig{Mode: AuthModeDev},
		},
		{
			name: "disabled mode (explicit)",
			env:  map[string]string{"SIGAP_AUTH_MODE": "disabled"},
			want: &AuthConfig{Mode: AuthModeDisabled},
		},
		{
			name: "disabled mode (default)",
			env:  map[string]string{},
			want: &AuthConfig{Mode: AuthModeDisabled},
		},
		{
			name: "jwt mode with all required fields",
			env: map[string]string{
				"SIGAP_AUTH_MODE":     "jwt",
				"SIGAP_AUTH_ISSUER":   "https://accounts.example.com",
				"SIGAP_AUTH_AUDIENCE": "sigap-api",
			},
			want: &AuthConfig{
				Mode:     AuthModeJWT,
				Issuer:   "https://accounts.example.com",
				Audience: "sigap-api",
			},
		},
		{
			name: "jwt mode missing issuer",
			env: map[string]string{
				"SIGAP_AUTH_MODE":     "jwt",
				"SIGAP_AUTH_AUDIENCE": "sigap-api",
			},
			wantErr: true,
		},
		{
			name: "jwt mode missing audience",
			env: map[string]string{
				"SIGAP_AUTH_MODE":   "jwt",
				"SIGAP_AUTH_ISSUER": "https://accounts.example.com",
			},
			wantErr: true,
		},
		{
			name:    "invalid mode value",
			env:     map[string]string{"SIGAP_AUTH_MODE": "oauth2"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg, err := LoadConfigFromEnv()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if cfg.Mode != tt.want.Mode {
				t.Errorf("mode = %q, want %q", cfg.Mode, tt.want.Mode)
			}
			if cfg.Issuer != tt.want.Issuer {
				t.Errorf("issuer = %q, want %q", cfg.Issuer, tt.want.Issuer)
			}
			if cfg.Audience != tt.want.Audience {
				t.Errorf("audience = %q, want %q", cfg.Audience, tt.want.Audience)
			}
		})
	}
}

// TestNewProvider_Selection verifies the factory returns the correct
// concrete provider type for each mode.
func TestNewProvider_Selection(t *testing.T) {
	dev := NewProvider(&AuthConfig{Mode: AuthModeDev})
	if _, ok := dev.(*DevIdentityProvider); !ok {
		t.Errorf("dev mode: got %T, want *DevIdentityProvider", dev)
	}

	jwt := NewProvider(&AuthConfig{Mode: AuthModeJWT, Issuer: "x", Audience: "y"})
	if _, ok := jwt.(*JWTProvider); !ok {
		t.Errorf("jwt mode: got %T, want *JWTProvider", jwt)
	}

	disabled := NewProvider(&AuthConfig{Mode: AuthModeDisabled})
	if disabled != nil {
		t.Errorf("disabled mode: got %T, want nil", disabled)
	}
}

// TestMiddleware_PropagatesActor verifies that a provider returning a
// non-zero actor causes the next handler to see that actor in context.
func TestMiddleware_PropagatesActor(t *testing.T) {
	t.Setenv(devIdentityEnv, "true")
	provider := NewDevIdentityProvider()

	var got identity.Actor
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.ActorFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(devIdentityHeader, "user-1")
	rec := httptest.NewRecorder()

	Middleware(provider)(handler).ServeHTTP(rec, req)

	if got.UserID != "user-1" {
		t.Errorf("actor userID = %q, want %q", got.UserID, "user-1")
	}
	if got.Type != identity.ActorDev {
		t.Errorf("actor type = %v, want %v", got.Type, identity.ActorDev)
	}
}

// TestMiddleware_ZeroActorNoCrash verifies that a provider returning a
// zero actor still allows the request to continue (the authz layer is
// responsible for rejecting it, not the auth middleware).
func TestMiddleware_ZeroActorNoCrash(t *testing.T) {
	// Dev provider with env disabled always returns zero actor.
	t.Setenv(devIdentityEnv, "false")
	provider := NewDevIdentityProvider()

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	Middleware(provider)(handler).ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called; expected pass-through with zero actor")
	}
}

// TestMiddleware_NilProvider verifies that a nil provider results in a
// pass-through middleware that does not panic.
func TestMiddleware_NilProvider(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	Middleware(nil)(handler).ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called; expected nil-provider pass-through")
	}
}

// TestJWTProvider_FailClosed verifies the scaffold returns a zero actor
// so the authorization layer denies access until real JWT handling lands.
func TestJWTProvider_FailClosed(t *testing.T) {
	p := NewJWTProvider(AuthConfig{Issuer: "x", Audience: "y"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor (fail-closed), got %+v", actor)
	}
}
