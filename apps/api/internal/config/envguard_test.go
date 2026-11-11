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
				"SIGAP_ENV":       "local",
				"SIGAP_AUTH_MODE": "dev",
			},
			wantErr: false,
		},
		{
			name: "local + dev identity",
			env: map[string]string{
				"SIGAP_ENV":          "local",
				"SIGAP_DEV_IDENTITY": "true",
			},
			wantErr: false,
		},
		{
			name: "local + engine fallback dev",
			env: map[string]string{
				"SIGAP_ENV":              "local",
				"SIGAP_ENGINE_FALLBACK":  "dev",
			},
			wantErr: false,
		},
		{
			name: "local + all dev flags",
			env: map[string]string{
				"SIGAP_ENV":             "local",
				"SIGAP_AUTH_MODE":       "dev",
				"SIGAP_DEV_IDENTITY":    "true",
				"SIGAP_ENGINE_FALLBACK": "dev",
			},
			wantErr: false,
		},
		{
			name: "production safe config",
			env: map[string]string{
				"SIGAP_ENV":       "production",
				"SIGAP_AUTH_MODE": "jwt",
			},
			wantErr: false,
		},
		{
			name:    "SIGAP_ENV unset + no dev flags",
			env:     map[string]string{},
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
			name: "production + engine fallback dev",
			env: map[string]string{
				"SIGAP_ENV":             "production",
				"SIGAP_ENGINE_FALLBACK": "dev",
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
				"SIGAP_ENV":             "production",
				"SIGAP_AUTH_MODE":       "dev",
				"SIGAP_DEV_IDENTITY":    "true",
				"SIGAP_ENGINE_FALLBACK": "dev",
			},
			wantErr: true,
			errMsg:  "SIGAP_AUTH_MODE is only allowed when SIGAP_ENV=local",
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
		// Phase 3B5.0: the local DB-backed test identity selector must be
		// refused at BOOT outside local, not merely inert at request time.
		// Inert-at-request is not sufficient on its own: a process that
		// believes it is running with a test selector and is silently running
		// without one is a confusing failure, and one that assumes the flag
		// is inert is a worse one. Failing fast is the honest behaviour.
		{
			name: "production + local RBAC test identity",
			env: map[string]string{
				"SIGAP_ENV":                    "production",
				"SIGAP_LOCAL_RBAC_TEST_IDENTITY": "true",
			},
			wantErr: true,
			errMsg:  "SIGAP_LOCAL_RBAC_TEST_IDENTITY is only allowed when SIGAP_ENV=local",
		},
		{
			name: "staging + local RBAC test identity",
			env: map[string]string{
				"SIGAP_ENV":                    "staging",
				"SIGAP_LOCAL_RBAC_TEST_IDENTITY": "true",
			},
			wantErr: true,
			errMsg:  "SIGAP_LOCAL_RBAC_TEST_IDENTITY is only allowed when SIGAP_ENV=local",
		},
		{
			// GuardDevCapabilities treats SIGAP_ENV case-insensitively, so
			// "Local" satisfies it while the provider's own exact-match check
			// keeps the selector disarmed. Both behaviours are intentional and
			// safe: the guard is the permissive one and the provider is the
			// strict one.
			name: "SIGAP_ENV=Local + local RBAC test identity passes the boot guard",
			env: map[string]string{
				"SIGAP_ENV":                    "Local",
				"SIGAP_LOCAL_RBAC_TEST_IDENTITY": "true",
			},
			wantErr: false,
		},
		{
			name: "local + local RBAC test identity is allowed",
			env: map[string]string{
				"SIGAP_ENV":                    "local",
				"SIGAP_LOCAL_RBAC_TEST_IDENTITY": "true",
			},
			wantErr: false,
		},
		{
			name: "production + local RBAC test identity disabled is fine",
			env: map[string]string{
				"SIGAP_ENV":                    "production",
				"SIGAP_LOCAL_RBAC_TEST_IDENTITY": "false",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant env vars first
			envVars := []string{
				"SIGAP_ENV", "SIGAP_AUTH_MODE", "SIGAP_DEV_IDENTITY",
				"SIGAP_ENGINE_FALLBACK", "SIGAP_LOCAL_RBAC_TEST_IDENTITY",
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
// are reported in a single error — the operator sees the whole list at once
// rather than fixing one flag per restart.
func TestGuardDevCapabilities_MultipleViolations(t *testing.T) {
	envVars := []string{
		"SIGAP_ENV", "SIGAP_AUTH_MODE", "SIGAP_DEV_IDENTITY",
		"SIGAP_ENGINE_FALLBACK", "SIGAP_LOCAL_RBAC_TEST_IDENTITY",
	}
	for _, k := range envVars {
		t.Setenv(k, "")
	}
	t.Setenv("SIGAP_ENV", "production")
	t.Setenv("SIGAP_AUTH_MODE", "dev")
	t.Setenv("SIGAP_DEV_IDENTITY", "true")
	t.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

	err := GuardDevCapabilities()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	for _, want := range []string{
		"SIGAP_AUTH_MODE is only allowed when SIGAP_ENV=local",
		"SIGAP_DEV_IDENTITY is only allowed when SIGAP_ENV=local",
		"SIGAP_ENGINE_FALLBACK is only allowed when SIGAP_ENV=local",
		"SIGAP_LOCAL_RBAC_TEST_IDENTITY is only allowed when SIGAP_ENV=local",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing expected violation %q", want)
		}
	}
}

// TestGuardDevNetworkBinding verifies that dev capabilities are blocked
// from non-loopback addresses.
func TestGuardDevNetworkBinding(t *testing.T) {
	envVars := []string{
		"SIGAP_ENV", "SIGAP_API_HOST", "SIGAP_DEV_IDENTITY",
		"SIGAP_AUTH_MODE", "SIGAP_ENGINE_FALLBACK",
	}
	clearEnv := func() {
		for _, k := range envVars {
			t.Setenv(k, "")
		}
	}

	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{
			name: "local + dev identity + loopback host",
			env: map[string]string{
				"SIGAP_ENV":          "local",
				"SIGAP_DEV_IDENTITY": "true",
				"SIGAP_API_HOST":     "127.0.0.1",
			},
			wantErr: false,
		},
		{
			name: "local + dev identity + localhost",
			env: map[string]string{
				"SIGAP_ENV":          "local",
				"SIGAP_DEV_IDENTITY": "true",
				"SIGAP_API_HOST":     "localhost",
			},
			wantErr: false,
		},
		{
			name: "local + dev identity + empty host (defaults to loopback)",
			env: map[string]string{
				"SIGAP_ENV":          "local",
				"SIGAP_DEV_IDENTITY": "true",
			},
			wantErr: false,
		},
		{
			name: "local + dev identity + non-loopback host (REJECTED)",
			env: map[string]string{
				"SIGAP_ENV":          "local",
				"SIGAP_DEV_IDENTITY": "true",
				"SIGAP_API_HOST":     "0.0.0.0",
			},
			wantErr: true,
		},
		{
			name: "local + dev identity + external IP (REJECTED)",
			env: map[string]string{
				"SIGAP_ENV":          "local",
				"SIGAP_DEV_IDENTITY": "true",
				"SIGAP_API_HOST":     "192.168.1.100",
			},
			wantErr: true,
		},
		{
			name: "local + auth mode dev + non-loopback (REJECTED)",
			env: map[string]string{
				"SIGAP_ENV":       "local",
				"SIGAP_AUTH_MODE": "dev",
				"SIGAP_API_HOST":  "0.0.0.0",
			},
			wantErr: true,
		},
		{
			name: "local + no dev flags + any host (allowed)",
			env: map[string]string{
				"SIGAP_ENV":       "local",
				"SIGAP_API_HOST":  "0.0.0.0",
			},
			wantErr: false,
		},
		{
			name: "production + dev identity (GuardDevCapabilities handles; NetworkBinding returns nil)",
			env: map[string]string{
				"SIGAP_ENV":          "production",
				"SIGAP_DEV_IDENTITY": "true",
				"SIGAP_API_HOST":     "0.0.0.0",
			},
			wantErr: false,
		},
		{
			name: "localhost with ::1 (IPv6 loopback)",
			env: map[string]string{
				"SIGAP_ENV":          "local",
				"SIGAP_DEV_IDENTITY": "true",
				"SIGAP_API_HOST":     "::1",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv()
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			err := GuardDevNetworkBinding()
			if (err != nil) != tt.wantErr {
				t.Fatalf("GuardDevNetworkBinding() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGuardTLS verifies the TLS termination guard for non-local environments.
func TestGuardTLS(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "local env skips TLS check",
			env:     map[string]string{"SIGAP_ENV": "local"},
			wantErr: false,
		},
		{
			name:    "unset env skips TLS check",
			env:     map[string]string{},
			wantErr: false,
		},
		{
			name:    "production + TLS confirmed",
			env:     map[string]string{"SIGAP_ENV": "production", "SIGAP_TLS_TERMINATED": "true"},
			wantErr: false,
		},
		{
			name:    "staging + TLS confirmed",
			env:     map[string]string{"SIGAP_ENV": "staging", "SIGAP_TLS_TERMINATED": "true"},
			wantErr: false,
		},
		{
			name:    "production + TLS not set",
			env:     map[string]string{"SIGAP_ENV": "production"},
			wantErr: true,
			errMsg:  "SIGAP_TLS_TERMINATED must be set to true in production",
		},
		{
			name:    "staging + TLS not set",
			env:     map[string]string{"SIGAP_ENV": "staging"},
			wantErr: true,
			errMsg:  "SIGAP_TLS_TERMINATED must be set to true in staging",
		},
		{
			name:    "production + TLS false",
			env:     map[string]string{"SIGAP_ENV": "production", "SIGAP_TLS_TERMINATED": "false"},
			wantErr: true,
			errMsg:  "SIGAP_TLS_TERMINATED must be set to true in production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"SIGAP_ENV", "SIGAP_TLS_TERMINATED"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			err := GuardTLS()
			if (err != nil) != tt.wantErr {
				t.Fatalf("GuardTLS() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Per-flag refusal matrix
//
// The table above enumerates cases by hand. The tests below enumerate the
// REGISTRY instead, so the audit question "does the guard refuse each dev flag
// outside SIGAP_ENV=local, and allow it only in local?" is answered for every
// flag rather than for the ones a hand-written table happened to include.
// ---------------------------------------------------------------------------

// auditedDevFlag pairs a registered dev flag with the value that triggers the
// guard and values that must not. TestGuardDevCapabilities_RegistryIsAudited
// checks this list against devOnlyFlags in both directions, so it cannot drift:
// a newly registered flag fails until it is audited here, and a removed one
// fails until it is deleted here.
type auditedDevFlag struct {
	envVar    string
	dangerous string
	safe      []string
}

var auditedDevFlags = []auditedDevFlag{
	// SIGAP_AUTH_MODE=dev selects DevIdentityProvider at boot: the header-driven
	// synthetic actor.
	{envVar: "SIGAP_AUTH_MODE", dangerous: "dev", safe: []string{"jwt", "disabled", "prod"}},
	// SIGAP_DEV_IDENTITY=true arms the X-Sigap-Dev-User-ID actor.
	{envVar: "SIGAP_DEV_IDENTITY", dangerous: "true", safe: []string{"false", "0", "no"}},
	// Not an identity selector at all, but a dev capability: it swaps the Rust
	// engine for an in-process fake.
	{envVar: "SIGAP_ENGINE_FALLBACK", dangerous: "dev", safe: []string{"grpc", "false", "prod"}},
	// SIGAP_LOCAL_RBAC_TEST_IDENTITY=true installs the header-driven,
	// DB-backed subject selector.
	{envVar: "SIGAP_LOCAL_RBAC_TEST_IDENTITY", dangerous: "true", safe: []string{"false", "0", "no"}},
}

// localEnvSpellings are the SIGAP_ENV values the guard treats as local
// (strings.TrimSpace + strings.EqualFold). A dev flag must be ALLOWED for
// every one of them.
var localEnvSpellings = []struct{ name, value string }{
	{"exact local", "local"},
	{"LOCAL uppercase", "LOCAL"},
	{"Local capitalised", "Local"},
	{"surrounding spaces", "  local  "},
	{"tab and newline padding", "\tlocal\n"},
}

// nonLocalEnvSpellings are the values that must NOT be treated as local. The
// near-misses are the point: "local_test" and "localhost" read as local to a
// human skimming a config file, and "Local" / "LOCAL" read as local to anyone
// who assumes case-insensitive comparison everywhere.
var nonLocalEnvSpellings = []struct{ name, value string }{
	{"unset", ""},
	{"whitespace only", "   "},
	{"tab only", "\t"},
	{"production", "production"},
	{"staging", "staging"},
	{"dev", "dev"},
	{"local_test", "local_test"},
	{"localhost", "localhost"},
	{"local-1", "local-1"},
	{"LOCAL_", "LOCAL_"},
}

// guardedEnvVars is every environment variable these tests manipulate. Clearing
// all of them means a developer machine that exports one cannot change a
// verdict.
var guardedEnvVars = []string{
	"SIGAP_ENV",
	"SIGAP_AUTH_MODE",
	"SIGAP_DEV_IDENTITY",
	"SIGAP_ENGINE_FALLBACK",
	"SIGAP_LOCAL_RBAC_TEST_IDENTITY",
	// Web-side only, and deliberately NOT registered as a Go dev flag; see
	// TestGuardDevCapabilities_WebSideE2eActorIsNotAGoGate.
	"SIGAP_LOCAL_E2E_ACTOR",
	"SIGAP_API_HOST",
	"SIGAP_TLS_TERMINATED",
}

func clearGuardedEnv(t *testing.T) {
	t.Helper()
	for _, k := range guardedEnvVars {
		t.Setenv(k, "")
	}
}

// capitalizeFirstASCII upper-cases the first byte of an ASCII flag value, so
// the "Dev"/"True" spellings are covered without the deprecated
// strings.Title. Flag values are ASCII by construction ("dev", "true").
func capitalizeFirstASCII(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func devFlagByEnvVar(t *testing.T, envVar string) devFlag {
	t.Helper()
	for _, f := range devOnlyFlags {
		if f.EnvVar == envVar {
			return f
		}
	}
	t.Fatalf("%s is not registered in devOnlyFlags", envVar)
	return devFlag{}
}

// TestGuardDevCapabilities_RegistryIsAudited proves the matrix above covers the
// registry exactly, and that every registered entry is well formed.
func TestGuardDevCapabilities_RegistryIsAudited(t *testing.T) {
	registered := make(map[string]bool, len(devOnlyFlags))
	for _, f := range devOnlyFlags {
		if strings.TrimSpace(f.EnvVar) == "" {
			t.Error("devOnlyFlags contains an entry with an empty EnvVar")
			continue
		}
		if f.IsDanger == nil {
			t.Errorf("%s has no IsDanger predicate: it can never be refused", f.EnvVar)
		}
		if strings.TrimSpace(f.Label) == "" {
			t.Errorf("%s has no Label: its violation message would be unactionable", f.EnvVar)
		}
		if registered[f.EnvVar] {
			t.Errorf("%s is registered twice in devOnlyFlags", f.EnvVar)
		}
		registered[f.EnvVar] = true
	}

	audited := make(map[string]bool, len(auditedDevFlags))
	for _, spec := range auditedDevFlags {
		if audited[spec.envVar] {
			t.Errorf("%s is audited twice in auditedDevFlags", spec.envVar)
		}
		audited[spec.envVar] = true
	}

	for envVar := range registered {
		if !audited[envVar] {
			t.Errorf("devOnlyFlags registers %s but no audited value exists for it; add it to "+
				"auditedDevFlags so its refusal and allowance are both tested", envVar)
		}
	}
	for envVar := range audited {
		if !registered[envVar] {
			t.Errorf("auditedDevFlags covers %s but devOnlyFlags does not register it; either "+
				"the flag was removed (delete it here) or the guard lost a lever", envVar)
		}
	}

	// The web-side local E2E actor variable is deliberately NOT a Go dev flag.
	// The Go API never reads it: it is consumed by apps/web/src/lib/server/auth.ts
	// (gated by localE2eActorHeader) and by scripts/ops/preflight-production-env.sh.
	// The header it produces is inert outside SIGAP_ENV=local, which the auth
	// package's impersonation tests prove. Registering it here would imply the
	// Go process polices it, which it cannot.
	if registered["SIGAP_LOCAL_E2E_ACTOR"] {
		t.Error("SIGAP_LOCAL_E2E_ACTOR must not be registered as a Go dev flag: the Go " +
			"process never reads it, so the guard could not enforce it and the entry " +
			"would misstate where the block lives")
	}
}

// TestGuardDevCapabilities_FlagValueClassification pins what each registered
// predicate considers dangerous, including the case-insensitivity that makes
// the guard cover spellings the request-time providers ignore.
func TestGuardDevCapabilities_FlagValueClassification(t *testing.T) {
	for _, spec := range auditedDevFlags {
		t.Run(spec.envVar, func(t *testing.T) {
			f := devFlagByEnvVar(t, spec.envVar)

			if !f.IsDanger(spec.dangerous) {
				t.Errorf("IsDanger(%q) = false, want true", spec.dangerous)
			}
			// EqualFold semantics: "DEV"/"TRUE" trigger the guard. The
			// providers compare exactly, so the guard is the stricter of the
			// two — the safe direction. A case variant therefore refuses the
			// boot rather than silently arming nothing.
			if !f.IsDanger(strings.ToUpper(spec.dangerous)) {
				t.Errorf("IsDanger(%q) = false, want true (the guard compares "+
					"case-insensitively)", strings.ToUpper(spec.dangerous))
			}
			// Padded values are NOT dangerous here, and that is consistent:
			// the request-time providers compare exactly too (os.Getenv(...) ==
			// "true"), so " true " arms nothing anywhere. A value that the
			// guard ignored but a provider honoured would be the dangerous
			// asymmetry; this asserts it does not exist.
			if f.IsDanger(" " + spec.dangerous + " ") {
				t.Errorf("IsDanger(%q) = true; neither the guard nor the providers trim "+
					"this value, and they must agree", " "+spec.dangerous+" ")
			}
			for _, safe := range spec.safe {
				if f.IsDanger(safe) {
					t.Errorf("IsDanger(%q) = true, want false: a non-dev value must never "+
						"refuse the boot", safe)
				}
				if f.IsDanger(strings.ToUpper(safe)) {
					t.Errorf("IsDanger(%q) = true, want false", strings.ToUpper(safe))
				}
			}
			// An empty value means "unset" to the guard, which skips it before
			// consulting the predicate — but pin the predicate's own view too,
			// so a predicate that treated "" as dangerous could not make an
			// unset variable refuse the boot.
			if f.IsDanger("") {
				t.Errorf("IsDanger(\"\") = true: an unset variable must never refuse the boot")
			}
		})
	}
}

// TestGuardDevCapabilities_EachFlagRefusedOutsideLocal is the refusal half of
// the audit question, for every registered flag and every non-local spelling.
func TestGuardDevCapabilities_EachFlagRefusedOutsideLocal(t *testing.T) {
	for _, spec := range auditedDevFlags {
		for _, env := range nonLocalEnvSpellings {
			t.Run(spec.envVar+"/"+env.name, func(t *testing.T) {
				clearGuardedEnv(t)
				t.Setenv("SIGAP_ENV", env.value)
				t.Setenv(spec.envVar, spec.dangerous)

				err := GuardDevCapabilities()
				if err == nil {
					t.Fatalf("GuardDevCapabilities() = nil with SIGAP_ENV=%q and %s=%q; the "+
						"process would start with a dev-only capability armed outside local",
						env.value, spec.envVar, spec.dangerous)
				}
				want := spec.envVar + " is only allowed when SIGAP_ENV=local"
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), want)
				}
			})
		}
	}
}

// TestGuardDevCapabilities_EachFlagAllowedOnlyInLocal is the allowance half.
// Without it, "never start" would satisfy every refusal case above while
// breaking local development entirely.
func TestGuardDevCapabilities_EachFlagAllowedOnlyInLocal(t *testing.T) {
	for _, spec := range auditedDevFlags {
		for _, env := range localEnvSpellings {
			t.Run(spec.envVar+"/"+env.name, func(t *testing.T) {
				clearGuardedEnv(t)
				t.Setenv("SIGAP_ENV", env.value)
				t.Setenv(spec.envVar, spec.dangerous)

				if err := GuardDevCapabilities(); err != nil {
					t.Errorf("GuardDevCapabilities() = %v with SIGAP_ENV=%q and %s=%q; local "+
						"development must be able to use its own flags",
						err, env.value, spec.envVar, spec.dangerous)
				}
			})
		}
	}
}

// TestGuardDevCapabilities_EachFlagSafeValueNeverRefuses pins the other side of
// the predicate: a flag that is present but not set to its dangerous value must
// not refuse the boot, in any environment, because that is the ordinary
// configuration of a staging deploy.
func TestGuardDevCapabilities_EachFlagSafeValueNeverRefuses(t *testing.T) {
	for _, spec := range auditedDevFlags {
		for _, env := range nonLocalEnvSpellings {
			for _, safe := range spec.safe {
				t.Run(spec.envVar+"/"+safe+"/"+env.name, func(t *testing.T) {
					clearGuardedEnv(t)
					t.Setenv("SIGAP_ENV", env.value)
					t.Setenv(spec.envVar, safe)

					if err := GuardDevCapabilities(); err != nil {
						t.Errorf("GuardDevCapabilities() = %v with SIGAP_ENV=%q and %s=%q; a "+
							"present-but-safe value must not refuse the boot",
							err, env.value, spec.envVar, safe)
					}
				})
			}
		}
	}
}

// TestGuardDevCapabilities_CaseVariantDangerousValueRefusedOutsideLocal covers
// the escalation attempt the case-insensitive predicate exists for: an operator
// writing TRUE (or Dev) may believe they armed nothing, while every request-time
// provider compares exactly and would agree with them. The guard refuses the
// boot anyway, so the disagreement can never be resolved in the dangerous
// direction.
func TestGuardDevCapabilities_CaseVariantDangerousValueRefusedOutsideLocal(t *testing.T) {
	for _, spec := range auditedDevFlags {
		for _, variant := range []string{strings.ToUpper(spec.dangerous), capitalizeFirstASCII(spec.dangerous)} {
			t.Run(spec.envVar+"/"+variant, func(t *testing.T) {
				clearGuardedEnv(t)
				t.Setenv("SIGAP_ENV", "production")
				t.Setenv(spec.envVar, variant)

				err := GuardDevCapabilities()
				if err == nil {
					t.Fatalf("GuardDevCapabilities() = nil with SIGAP_ENV=production and %s=%q",
						spec.envVar, variant)
				}
				if !strings.Contains(err.Error(), spec.envVar+" is only allowed when SIGAP_ENV=local") {
					t.Errorf("error = %q, want it to name %s", err.Error(), spec.envVar)
				}
			})
		}
	}
}

// TestGuardDevCapabilities_WebSideE2eActorIsNotAGoGate records where the sixth
// lever is blocked, because the Go guard is not it.
//
// SIGAP_LOCAL_E2E_ACTOR is read by the web tier only
// (apps/web/src/lib/server/auth.ts: localE2eActorHeader), which emits
// X-Sigap-Local-Test-Subject and nothing else, and only when SIGAP_ENV is
// local. The Go API never reads the variable. The block for it therefore lives
// in three places this test does NOT replace:
//
//  1. apps/web/src/lib/server/auth.ts — no header unless SIGAP_ENV=local.
//  2. scripts/ops/preflight-production-env.sh — refuses a production deploy
//     with the variable set.
//  3. The Go side refuses the HEADER it produces outside local
//     (auth.LocalRBACTestIdentityProvider is disarmed, and this test's own
//     registry assertions plus the auth impersonation tests prove it).
//
// What this test pins is that the Go boot guard's verdict is a function of
// SIGAP_ENV and the REGISTERED flags only. If it ever started refusing this
// variable, that would be a new — and welcome — defence layer, and this test
// would say so instead of the change landing silently.
func TestGuardDevCapabilities_WebSideE2eActorIsNotAGoGate(t *testing.T) {
	for _, env := range []string{"production", "staging", "local"} {
		t.Run(env, func(t *testing.T) {
			clearGuardedEnv(t)
			t.Setenv("SIGAP_ENV", env)
			t.Setenv("SIGAP_LOCAL_E2E_ACTOR", "local-global-super-admin")

			if err := GuardDevCapabilities(); err != nil {
				t.Errorf("GuardDevCapabilities() = %v with SIGAP_ENV=%q and "+
					"SIGAP_LOCAL_E2E_ACTOR set; the Go process does not read that variable, "+
					"so if it now refuses it, update this test and the audit doc "+
					"(design/impersonation-production-guard.md)", err, env)
			}
		})
	}
}
