package config

import (
	"fmt"
	"os"
	"strings"
)

// devOnlyFlags lists environment variables that are ONLY safe when
// SIGAP_ENV=local.  If any of these is set to a "dangerous" value and
// SIGAP_ENV is not "local", the application must refuse to start.
//
// Each entry maps the env-var name to the dangerous value(s) that trigger
// the guard (case-insensitive comparison for string flags, exact "true"
// for boolean flags).
type devFlag struct {
	EnvVar   string
	IsDanger func(value string) bool
	Label    string // human-readable name for error messages
}

var devOnlyFlags = []devFlag{
	{
		EnvVar: "SIGAP_AUTH_MODE",
		IsDanger: func(v string) bool {
			return strings.EqualFold(v, "dev")
		},
		Label: "auth mode dev",
	},
	{
		EnvVar: "SIGAP_DEV_IDENTITY",
		IsDanger: func(v string) bool {
			return strings.EqualFold(v, "true")
		},
		Label: "dev identity",
	},
	{
		EnvVar: "SIGAP_ENGINE_FALLBACK",
		IsDanger: func(v string) bool {
			return strings.EqualFold(v, "dev")
		},
		Label: "engine fallback dev",
	},
}

// GuardDevCapabilities checks that dev-only environment flags are not
// enabled outside an explicit local environment.  It reads SIGAP_ENV and
// compares it case-insensitively to "local".
//
// Returns nil when the environment is safe, or an error listing every
// violation found.  The error message is actionable but never leaks
// secrets, tokens, or connection strings.
//
// Call this early in main() — before any I/O or listener binding — so
// the process exits immediately on misconfiguration.
func GuardDevCapabilities() error {
	env := strings.TrimSpace(os.Getenv("SIGAP_ENV"))
	isLocal := strings.EqualFold(env, "local")

	var violations []string

	for _, f := range devOnlyFlags {
		val := os.Getenv(f.EnvVar)
		if val == "" {
			continue
		}
		if f.IsDanger(val) && !isLocal {
			violations = append(violations,
				fmt.Sprintf("%s is only allowed when SIGAP_ENV=local", f.EnvVar))
		}
	}

	if len(violations) == 0 {
		return nil
	}

	msg := "dev-only capabilities enabled in non-local environment:\n"
	for _, v := range violations {
		msg += "  - " + v + "\n"
	}
	msg += "Set SIGAP_ENV=local to allow dev flags, or remove them for staging/production."
	return fmt.Errorf("%s", msg)
}

// GuardTLS verifies that non-local environments have TLS termination
// confirmed by setting SIGAP_TLS_TERMINATED=true.  This prevents the
// API from accepting plain HTTP traffic in staging or production.
//
// In local mode (SIGAP_ENV=local) or when SIGAP_ENV is unset, the
// check is skipped — local development uses plain HTTP behind a reverse
// proxy or directly.
func GuardTLS() error {
	env := strings.TrimSpace(os.Getenv("SIGAP_ENV"))
	isLocal := strings.EqualFold(env, "local")
	if isLocal || env == "" {
		return nil
	}

	confirmed := strings.EqualFold(os.Getenv("SIGAP_TLS_TERMINATED"), "true")
	if !confirmed {
		return fmt.Errorf(
			"SIGAP_TLS_TERMINATED must be set to true in %s environment; " +
				"the API listens on plain HTTP and requires a TLS-terminating " +
				"reverse proxy in front",
			env)
	}
	return nil
}
