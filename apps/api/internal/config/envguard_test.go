package config

import (
	"strings"
	"testing"
)

// TestGuardDevCapabilities is a table-driven test covering every guard branch
// required by the production readiness audit.
func TestGuardDevCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string // env vars to set (empty string = unset)
		wantErr bool
		errMsg  string // substring expected in error message (empty = any error)
	}{
		// --- allowed cases ---
		{
			name: "local + auth mode dev",
			env: map[string]string{
				"SIGAP_ENV":        "local",
				"SIGAP_AUTH_MODE":  "dev",
			},
			wantErr: false,
		},
		{
			name: "local + dev identity",
			env: map[string]string{
				"SIGAP_ENV":           "local",
				"SIGAP_DEV_IDENTITY":  "true",
			},
			wantErr: false,
		},
		{
			name: "local + demo PHI",
			env: map[string]string{
				"SIGAP_ENV":              "local",
				"SIGAP_ENABLE_DEMO_PHI":  "true",
			},
			wantErr: false,
		},
		{
			name: "local + engine fallback dev",
			env: map[string]string{
				"SIGAP_ENV":                "local",
				"SIGAP_ENGINE_FALLBACK":    "dev",
			},
			wantErr: false,
		},
		{
			name: "local + all dev flags",
			env: map[string]string{
				"SIGAP_ENV":                "local",
				"SIGAP_AUTH_MODE":          "dev",
				"SIGAP_DEV_IDENTITY":       "true",
				"SIGAP_ENABLE_DEMO_PHI":    "true",
				"SIGAP_ENGINE_FALLBACK":    "dev",
			},
			wantErr: false,
		},
		{
			name: "production safe config",
			env: map[string]string{
				"SIGAP_ENV":        "production",
				"SIGAP_AUTH_MODE":  "jwt",
			},
			wantErr: false,
		},
		{
			name: "SIGAP_ENV unset + no dev flags",
			env:  map[string]string{},
			wantErr: false,
		},
		{
			name: "staging + safe config",
			env: map[string]string{
				"SIGAP_ENV":       "staging",
				"SIGAP_AUTH_MODE": "jwt",
			},
			wantErr: false,
		},

		// --- rejected cases ---
		{
			name: "production + auth mode dev",
			env: map[string]string{
				"SIGAP_ENV":       "production",
				"SIGAP_AUTH_MODE": "dev",
			},
			wantErr: true,
			errMsg:  "SIGAP_AUTH_MODE is only allowed when SIGAP_ENV=local",
		},
		{
			name: "staging + auth mode dev",
			env: map[string]string{
				"SIGAP_ENV":       "staging",
				"SIGAP_AUTH_MODE": "dev",
			},
			wantErr: true,
			errMsg:  "SIGAP_AUTH_MODE is only allowed when SIGAP_ENV=local",
		},
		{
			name: "SIGAP_ENV unset + auth mode dev",
			env: map[string]string{
				"SIGAP_AUTH_MODE": "dev",
			},
			wantErr: true,
			errMsg:  "SIGAP_AUTH_MODE is only allowed when SIGAP_ENV=local",
		},
		{
			name: "production + dev identity",
			env: map[string]string{
				"SIGAP_ENV":          "production",
				"SIGAP_DEV_IDENTITY": "true",
			},
			wantErr: true,
			errMsg:  "SIGAP_DEV_IDENTITY is only allowed when SIGAP_ENV=local",
		},
		{
			name: "staging + dev identity",
			env: map[string]string{
				"SIGAP_ENV":          "staging",
				"SIGAP_DEV_IDENTITY": "true",
			},
			wantErr: true,
			errMsg:  "SIGAP_DEV_IDENTITY is only allowed when SIGAP_ENV=local",
		},
		{
			name: "production + demo PHI",
			env: map[string]string{
				"SIGAP_ENV":             "production",
				"SIGAP_ENABLE_DEMO_PHI": "true",
			},
			wantErr: true,
			errMsg:  "SIGAP_ENABLE_DEMO_PHI is only allowed when SIGAP_ENV=local",
		},
		{
			name: "production + engine fallback dev",
			env: map[string]string{
				"SIGAP_ENV":              "production",
				"SIGAP_ENGINE_FALLBACK":  "dev",
			},
			wantErr: true,
			errMsg:  "SIGAP_ENGINE_FALLBACK is only allowed when SIGAP_ENV=local",
		},
		{
			name: "SIGAP_ENV unset + dev identity",
			env: map[string]string{
				"SIGAP_DEV_IDENTITY": "true",
			},
			wantErr: true,
			errMsg:  "SIGAP_DEV_IDENTITY is only allowed when SIGAP_ENV=local",
		},
		{
			name: "SIGAP_ENV unset + engine fallback dev",
			env: map[string]string{
				"SIGAP_ENGINE_FALLBACK": "dev",
			},
			wantErr: true,
			errMsg:  "SIGAP_ENGINE_FALLBACK is only allowed when SIGAP_ENV=local",
		},
		{
			name: "production + all dev flags",
			env: map[string]string{
				"SIGAP_ENV":              "production",
				"SIGAP_AUTH_MODE":        "dev",
				"SIGAP_DEV_IDENTITY":     "true",
				"SIGAP_ENABLE_DEMO_PHI":  "true",
				"SIGAP_ENGINE_FALLBACK":  "dev",
			},
			wantErr: true,
			// All four should appear in the error
			errMsg: "SIGAP_AUTH_MODE is only allowed when SIGAP_ENV=local",
		},
		{
			name: "local env is case-insensitive",
			env: map[string]string{
				"SIGAP_ENV":         "LOCAL",
				"SIGAP_AUTH_MODE":   "dev",
				"SIGAP_DEV_IDENTITY": "true",
			},
			wantErr: false,
		},
		{
			name: "non-local env is case-insensitive",
			env: map[string]string{
				"SIGAP_ENV":          "Local",
				"SIGAP_DEV_IDENTITY": "false", // not dangerous
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant env vars first
			envVars := []string{
				"SIGAP_ENV", "SIGAP_AUTH_MODE", "SIGAP_DEV_IDENTITY",
				"SIGAP_ENABLE_DEMO_PHI", "SIGAP_ENGINE_FALLBACK",
			}
			for _, k := range envVars {
				t.Setenv(k, "")
			}
			// Set test-specific values
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			err := GuardDevCapabilities()

			if (err != nil) != tt.wantErr {
				t.Fatalf("GuardDevCapabilities() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			}
			// Verify error never leaks secrets
			if err != nil {
				lower := strings.ToLower(err.Error())
				for _, secret := range []string{"password", "token", "secret", "database_url", "jdbc"} {
					if strings.Contains(lower, secret) {
						t.Errorf("error message must not contain secret-like term %q: %s", secret, err.Error())
					}
				}
			}
		})
	}
}

// TestGuardDevCapabilities_MultipleViolations verifies that when multiple
// dev-only flags are enabled in a non-local environment, ALL violations
// are reported in a single error.
func TestGuardDevCapabilities_MultipleViolations(t *testing.T) {
	envVars := []string{
		"SIGAP_ENV", "SIGAP_AUTH_MODE", "SIGAP_DEV_IDENTITY",
		"SIGAP_ENABLE_DEMO_PHI", "SIGAP_ENGINE_FALLBACK",
	}
	for _, k := range envVars {
		t.Setenv(k, "")
	}
	t.Setenv("SIGAP_ENV", "production")
	t.Setenv("SIGAP_AUTH_MODE", "dev")
	t.Setenv("SIGAP_DEV_IDENTITY", "true")
	t.Setenv("SIGAP_ENABLE_DEMO_PHI", "true")
	t.Setenv("SIGAP_ENGINE_FALLBACK", "dev")

	err := GuardDevCapabilities()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	for _, want := range []string{
		"SIGAP_AUTH_MODE is only allowed when SIGAP_ENV=local",
		"SIGAP_DEV_IDENTITY is only allowed when SIGAP_ENV=local",
		"SIGAP_ENABLE_DEMO_PHI is only allowed when SIGAP_ENV=local",
		"SIGAP_ENGINE_FALLBACK is only allowed when SIGAP_ENV=local",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing expected violation %q", want)
		}
	}
}
