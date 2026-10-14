package auth

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// LocalRBACTestIdentityProvider — a LOCAL/TEST-ONLY identity selector that
// exercises the REAL DB-backed RBAC resolver.
// ---------------------------------------------------------------------------
//
// WHY THIS EXISTS
//
// Phase 3B5 needs a ScheduleEditor, and the E2E that will exercise it needs
// an operator who genuinely holds schedule.manage. The existing dev identity
// deliberately does not: it carries a fixed synthetic permission set that
// omits schedule.manage, and that is correct — the dev actor is for the
// read-only demo, and quietly granting it schedule.manage would make the
// demo's "you cannot manage schedules here" state unrepresentable.
//
// So this is a SEPARATE actor, not an extension of the dev one. The design
// constraint that makes it safe is that it contains no permissions at all.
//
//   - It does not ship a permission list. There is no hardcoded bypass
//     granting everything, and no permission that exists only in Go.
//   - Every permission and every piece of facility provenance comes from the
//     REAL RBAC resolver reading user_roles / role_permissions in the
//     database, exactly as the production JWT provider does.
//   - A subject with no seeded roles resolves to zero permissions, so the
//     selector cannot manufacture access — it can only SELECT an identity
//     that the database has already authorized.
//
// That last point is the whole safety argument. The header names a subject;
// the database decides what that subject may do. Removing the seeded role
// removes the access, with no code change.
//
// WHY IT CANNOT REACH PRODUCTION
//
// Activation requires SIGAP_LOCAL_RBAC_TEST_IDENTITY=true, and the provider
// refuses to construct unless SIGAP_ENV is exactly "local". Three independent
// things have to line up before it does anything:
//
//  1. The constructor checks SIGAP_ENV=local and returns a disabled provider
//     otherwise, so the provider is inert in staging/production.
//  2. config.GuardDevCapabilities refuses to let the process START if the
//     flag is set outside SIGAP_ENV=local, so a misconfigured deploy fails
//     loudly at boot rather than running with a live selector.
//  3. The header is stripped or ignored outside local; see proxy.go, which
//     already drops X-Sigap-* headers when SIGAP_ENV != local.
//
// The provider is NOT reachable from the JWT path and does not weaken Supabase
// auth: it is selected explicitly by the factory, never as a fallback, and it
// ignores Authorization entirely.
// ---------------------------------------------------------------------------

const (
	// localRBACSubjectHeader names the app_users.subject whose seeded
	// RBAC grants should be used for this request.
	localRBACSubjectHeader = "X-Sigap-Local-Test-Subject"

	// localRBACIdentityEnv must be "true" for the selector to be armed.
	localRBACIdentityEnv = "SIGAP_LOCAL_RBAC_TEST_IDENTITY"
)

// LocalRBACTestIdentityProvider authenticates a local test request by
// resolving the named subject's permissions from the database.
//
// It is a real Provider, not a bypass: it satisfies the same interface as the
// JWT provider and produces the same shape of actor, because it runs the same
// resolver.
type LocalRBACTestIdentityProvider struct {
	// armed reports whether the provider was constructed in a local
	// environment. When false, Authenticate is a no-op regardless of headers.
	armed bool
	// resolver is the REAL DB RBAC resolver. There is no fallback: a nil
	// resolver fails closed.
	resolver Resolver
}

// localTestIdentityEnabled reports whether SIGAP_ENV names a local
// environment. The comparison is exact and case-sensitive, which is
// deliberate: a permissive EqualFold here would let "Local" or "LOCAL"
// arm a test-only identity selector in an environment that did not intend
// to. Failing to arm is the safe direction.
func localTestIdentityEnabled() bool {
	return os.Getenv("SIGAP_ENV") == "local"
}

// NewLocalRBACTestIdentityProvider builds the provider, arming it only when
// SIGAP_ENV=local.
//
// The resolver is required. Passing nil yields a provider that
// authenticates nobody, because a selector that could not consult the
// database would have nothing trustworthy to report.
func NewLocalRBACTestIdentityProvider(resolver Resolver) *LocalRBACTestIdentityProvider {
	p := &LocalRBACTestIdentityProvider{armed: localTestIdentityEnabled(), resolver: resolver}
	if !p.armed {
		// Loud on purpose. Reaching this in a non-local environment means a
		// deployment is misconfigured, and the log line is the fastest way to
		// find out why the local test identity is not working.
		slog.Warn("local RBAC test identity is NOT armed because SIGAP_ENV is not \"local\"",
			"sigap_env", os.Getenv("SIGAP_ENV"))
	}
	return p
}

// Armed reports whether the provider is active. Exposed for the
// production-safety test and for startup diagnostics.
func (p *LocalRBACTestIdentityProvider) Armed() bool {
	return p != nil && p.armed
}

// Authenticate resolves the named subject through the real RBAC resolver.
//
// Every failure path returns a ZERO actor rather than an error, so the
// request falls through to the normal 401/403 handling instead of surfacing
// an authentication error that could be confused with an authorization one.
// A missing header, a disabled provider, a missing resolver, and an
// unresolvable subject are all indistinguishable to the caller — all of them
// mean "not authenticated".
func (p *LocalRBACTestIdentityProvider) Authenticate(r *http.Request) (identity.Actor, error) {
	// Fail closed first. The order matters: the armed check precedes the
	// header read, so a production request carrying the header still gets
	// nothing, and does not even reach the code that would look it up.
	if !p.Armed() {
		return identity.Actor{}, nil
	}
	if p.resolver == nil {
		slog.Warn("local RBAC test identity has no resolver; failing closed")
		return identity.Actor{}, nil
	}

	subject := strings.TrimSpace(r.Header.Get(localRBACSubjectHeader))
	if subject == "" {
		return identity.Actor{}, nil
	}

	resolved, err := p.resolver.Resolve(r.Context(), subject)
	if err != nil {
		// Resolver failure is fail-closed, never a fallback to any
		// hardcoded set. Logging the subject is safe: it is a local test
		// seed value, not a credential.
		slog.Warn("local RBAC test identity resolution failed; failing closed",
			"subject", subject, "err", err)
		return identity.Actor{}, nil
	}

	// An unknown subject resolves to empty permissions with no error. That is
	// correct fail-closed behaviour, but it is also almost always a typo in a
	// seed, so it is worth saying so.
	if len(resolved.Permissions) == 0 {
		slog.Warn("local RBAC test identity resolved to zero permissions; "+
			"is the subject seeded with an active role?",
			"subject", subject)
	}

	// The actor is a NORMAL user actor, never ActorDev. IsDev=false is the
	// load-bearing field: it keeps facility scope and permission-at-facility
	// enforcement in force, so this actor is subject to exactly the same
	// checks as a production operator. Marking it dev would bypass them and
	// would prove nothing.
	//
	// Permissions and FacilityGrants come verbatim from the resolver. This
	// provider contributes identity only.
	slog.Warn("local RBAC test identity is active — never use outside local",
		"subject", subject,
		"app_user_id", resolved.AppUserID,
		"request_method", r.Method,
		"request_path", r.URL.Path,
	)
	return identity.Actor{
		Type:           identity.ActorUser,
		UserID:         subject,
		Permissions:    resolved.Permissions,
		FacilityGrants: resolved.Grants,
		AppUserID:      resolved.AppUserID,
		IsDev:          false,
	}, nil
}

// NewLocalRBACTestIdentityProviderFromEnv constructs the provider with the
// standard DB-backed resolver. The caller supplies the pool so this package
// does not depend on pgxpool directly beyond what facility_scope.go already
// imports.
func NewLocalRBACTestIdentityProviderFromEnv(poolResolver Resolver) *LocalRBACTestIdentityProvider {
	return NewLocalRBACTestIdentityProvider(poolResolver)
}
