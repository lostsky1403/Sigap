package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnableCORS_LoopbackException(t *testing.T) {
	tests := []struct {
		name           string
		webOrigin      string
		requestOrigin  string
		wantACAO       string // expected Access-Control-Allow-Origin header
	}{
		{
			name:          "local default allows localhost origin",
			webOrigin:     "",
			requestOrigin: "http://localhost:3005",
			wantACAO:      "http://localhost:3005",
		},
		{
			name:          "local default allows 127.0.0.1 origin",
			webOrigin:     "",
			requestOrigin: "http://127.0.0.1:3005",
			wantACAO:      "http://127.0.0.1:3005",
		},
		{
			name:          "configured localhost allows 127.0.0.1",
			webOrigin:     "http://localhost:5173",
			requestOrigin: "http://127.0.0.1:5173",
			wantACAO:      "http://127.0.0.1:5173",
		},
		{
			name:          "configured 127.0.0.1 allows localhost",
			webOrigin:     "http://127.0.0.1:5173",
			requestOrigin: "http://localhost:5173",
			wantACAO:      "http://localhost:5173",
		},
		{
			name:          "production origin allows exact match",
			webOrigin:     "https://app.example.com",
			requestOrigin: "https://app.example.com",
			wantACAO:      "https://app.example.com",
		},
		{
			name:          "production origin rejects 127.0.0.1",
			webOrigin:     "https://app.example.com",
			requestOrigin: "http://127.0.0.1:3005",
			wantACAO:      "", // no ACAO header set
		},
		{
			name:          "production origin rejects localhost",
			webOrigin:     "https://app.example.com",
			requestOrigin: "http://localhost:3005",
			wantACAO:      "",
		},
		{
			name:          "unknown origin rejected",
			webOrigin:     "https://app.example.com",
			requestOrigin: "https://evil.com",
			wantACAO:      "",
		},
		{
			name:          "no origin header uses configured default",
			webOrigin:     "https://app.example.com",
			requestOrigin: "",
			wantACAO:      "https://app.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.webOrigin != "" {
				t.Setenv("SIGAP_WEB_ORIGIN", tt.webOrigin)
			} else {
				t.Setenv("SIGAP_WEB_ORIGIN", "")
			}

			called := false
			handler := enableCORS(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}
			rec := httptest.NewRecorder()
			handler(rec, req)

			if !called {
				t.Fatal("downstream handler was not called")
			}

			gotACAO := rec.Header().Get("Access-Control-Allow-Origin")
			if gotACAO != tt.wantACAO {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", gotACAO, tt.wantACAO)
			}
		})
	}
}
