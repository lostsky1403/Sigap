package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxy_LocalStripsHeaders(t *testing.T) {
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv("SIGAP_TRUSTED_PROXIES", "")

	var gotIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = ClientIPFromContext(r)
		w.WriteHeader(http.StatusOK)
	})

	handler := TrustedProxy(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")
	req.Header.Set("X-Real-IP", "5.6.7.8")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotIP != "192.168.1.100" {
		t.Errorf("local mode: got IP %q, want RemoteAddr %q", gotIP, "192.168.1.100")
	}
}

func TestTrustedProxy_UnsetStripsHeaders(t *testing.T) {
	t.Setenv("SIGAP_ENV", "")
	t.Setenv("SIGAP_TRUSTED_PROXIES", "")

	var gotIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = ClientIPFromContext(r)
	})

	handler := TrustedProxy(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotIP != "10.0.0.1" {
		t.Errorf("unset env: got IP %q, want RemoteAddr %q", gotIP, "10.0.0.1")
	}
}

func TestTrustedProxy_ProductionOneHop(t *testing.T) {
	t.Setenv("SIGAP_ENV", "production")
	t.Setenv("SIGAP_TRUSTED_PROXIES", "")

	var gotIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = ClientIPFromContext(r)
	})

	handler := TrustedProxy(next)

	// X-Forwarded-For: client, proxy
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotIP != "203.0.113.50" {
		t.Errorf("1-hop production: got IP %q, want %q", gotIP, "203.0.113.50")
	}
}

func TestTrustedProxy_ProductionXRealIP(t *testing.T) {
	t.Setenv("SIGAP_ENV", "production")
	t.Setenv("SIGAP_TRUSTED_PROXIES", "")

	var gotIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = ClientIPFromContext(r)
	})

	handler := TrustedProxy(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	req.Header.Set("X-Real-IP", "203.0.113.50")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotIP != "203.0.113.50" {
		t.Errorf("X-Real-IP 1-hop: got IP %q, want %q", gotIP, "203.0.113.50")
	}
}

func TestTrustedProxy_ProductionTwoHops(t *testing.T) {
	t.Setenv("SIGAP_ENV", "production")
	t.Setenv("SIGAP_TRUSTED_PROXIES", "2")

	var gotIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = ClientIPFromContext(r)
	})

	handler := TrustedProxy(next)

	// X-Forwarded-For: client, proxy1, proxy2
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1, 10.0.0.2")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotIP != "203.0.113.50" {
		t.Errorf("2-hop production: got IP %q, want %q", gotIP, "203.0.113.50")
	}
}

func TestTrustedProxy_ProductionZeroHops(t *testing.T) {
	t.Setenv("SIGAP_ENV", "production")
	t.Setenv("SIGAP_TRUSTED_PROXIES", "0")

	var gotIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = ClientIPFromContext(r)
	})

	handler := TrustedProxy(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotIP != "10.0.0.1" {
		t.Errorf("0-hop: got IP %q, want RemoteAddr %q", gotIP, "10.0.0.1")
	}
}

func TestTrustedProxy_NoProxyHeaders(t *testing.T) {
	t.Setenv("SIGAP_ENV", "production")
	t.Setenv("SIGAP_TRUSTED_PROXIES", "")

	var gotIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = ClientIPFromContext(r)
	})

	handler := TrustedProxy(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "203.0.113.50:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotIP != "203.0.113.50" {
		t.Errorf("no proxy headers: got IP %q, want %q", gotIP, "203.0.113.50")
	}
}

func TestClientIPFromContext_Fallback(t *testing.T) {
	// When middleware hasn't run, ClientIPFromContext should use RemoteAddr.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:54321"

	got := ClientIPFromContext(req)
	if got != "10.0.0.1" {
		t.Errorf("fallback: got %q, want %q", got, "10.0.0.1")
	}
}
