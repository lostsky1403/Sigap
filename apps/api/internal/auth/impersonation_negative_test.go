package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sigap/sigap/apps/api/internal/config"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// Impersonation negative tests
// ---------------------------------------------------------------------------
//
// What this file proves, adversarially:
//
//   1. A forged `X-Sigap-Local-Test-Subject` — or any header in the
//      `X-Sigap-Local-Test-*` namespace — names nobody outside SIGAP_ENV=local.
//      The disarmed selector does not merely return a zero actor; it never
//      consults the RBAC resolver at all.
//   2. A forged `X-Sigap-Dev-User-ID` names nobody when SIGAP_DEV_IDENTITY is
//      not exactly "true", and when it IS armed it still cannot be escalated
//      into a write or manage permission from a header.
//   3. No forged header grants a permission: the local selector forwards the
//      resolver's result verbatim (never adding), and the dev identity keeps
//      its fixed read-only set.
//   4. The JWT path ignores the token's `permissions` claim entirely, so a
//      client cannot mint its own authorization by signing (or replaying) a
//      token, and cannot smuggle identity through a dev/test header either.
//   5. The boot guard (config.GuardDevCapabilities) refuses every combination
//      in which a selector could be armed outside local, and permits them only
//      in local. This is the layer that makes cases 1-3 unreachable in a
//      deployment rather than merely inert.
//
// The existing tests in local_test_identity_provider_test.go already pin the
// provider's own fail-closed behaviour. This file adds the COMPOSITION: what a
// request carrying every forged header at once actually produces, at the
// provider level, at the middleware level, and at the boot-guard level.
//
// Every header name used below is the production constant, never a literal,
// so a rename in the provider cannot leave these tests asserting against a
// header nothing reads any more.

// privilegedPermissions is the escalation vocabulary a forged request would
// reach for. None of these may ever appear on an actor that came from a header
// outside its arming condition.
var privilegedPermissions = []string{
	"*",
	"admin",
	"super_admin",
	"facility.manage",
	"appointment.manage",
	"notification.manage",
	"queue.manage",
	"schedule.manage",
}

// forgedImpersonationRequest builds the worst-case request: every identity
// header this codebase knows about, plus the plausible escalation headers that
// do NOT exist, all naming a maximally privileged subject. If any header were
// read as authorization, this request is the one that finds out.
func forgedImpersonationRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules", nil)
	// The two real header families, naming DIFFERENT subjects on purpose: a
	// test can then tell which one the provider actually read instead of
	// observing that "some header" was honoured.
	req.Header.Set(localRBACSubjectHeader, "local-global-super-admin")
	req.Header.Set(devIdentityHeader, "forged-dev-user")
	// The escalation namespace a reader might have added for convenience.
	req.Header.Set("X-Sigap-Local-Test-Role", "super_admin")
	req.Header.Set("X-Sigap-Local-Test-Permissions", "*")
	req.Header.Set("X-Sigap-Local-Test-Facility", "00000000-0000-0000-0000-000000000000")
	req.Header.Set("X-Sigap-Dev-Role", "super_admin")
	req.Header.Set("X-Sigap-Dev-Permissions", "*")
	return req
}

// clearAuthEnv resets every environment variable that can arm a selector, so a
// developer machine that exports one of them cannot change a test's verdict.
func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SIGAP_ENV",
		"SIGAP_AUTH_MODE",
		"SIGAP_DEV_IDENTITY",
		"SIGAP_ENGINE_FALLBACK",
		localRBACIdentityEnv,
		"SIGAP_LOCAL_E2E_ACTOR",
		"SIGAP_API_HOST",
		"SIGAP_TLS_TERMINATED",
	} {
		t.Setenv(k, "")
	}
}

// callCountingResolver records whether it was consulted. A disarmed selector
// that still asked the database would be one configuration mistake away from
// answering with real permissions, so "never called" is the assertion that
// matters, not merely "returned nothing".
type callCountingResolver struct {
	calls   int
	subject string
	perms   ResolvedPermissions
}

func (r *callCountingResolver) Resolve(_ context.Context, subject string) (ResolvedPermissions, error) {
	r.calls++
	r.subject = subject
	return r.perms, nil
}

// assertNoPrivilegedPermissions fails if the actor holds anything from the
// escalation vocabulary.
func assertNoPrivilegedPermissions(t *testing.T, actor identity.Actor, where string) {
	t.Helper()
	for _, perm := range privilegedPermissions {
		if actor.HasPermission(perm) {
			t.Errorf("%s: actor holds privileged permission %q — a forged header escalated", where, perm)
		}
	}
}

func permSet(perms []string) map[string]bool {
	m := make(map[string]bool, len(perms))
	for _, p := range perms {
		m[p] = true
	}
	return m
}

// ---------------------------------------------------------------------------
// 1. Forged X-Sigap-Local-Test-Subject outside SIGAP_ENV=local
// ---------------------------------------------------------------------------

// TestImpersonationForgedLocalTestSubjectIsInertOutsideLocal is the core
// "a header cannot name an actor" proof for the local test selector.
//
// The env flag is set to "true" in every case, because that is the worst
// configuration: an operator has asked for the selector and the environment
// says no. What protects the deployment is the environment, never the flag.
func TestImpersonationForgedLocalTestSubjectIsInertOutsideLocal(t *testing.T) {
	nonLocal := []struct{ name, value string }{
		{name: "unset", value: ""},
		{name: "production", value: "production"},
		{name: "staging", value: "staging"},
		{name: "dev", value: "dev"},
		// The case variants are the point, not padding: the provider compares
		// exactly, so "Local" and "LOCAL" are NOT local and must not arm it.
		{name: "Local capitalised", value: "Local"},
		{name: "LOCAL uppercase", value: "LOCAL"},
		// " local " IS local to config.GuardDevCapabilities (TrimSpace +
		// EqualFold) but NOT to the provider (exact match). The guard is the
		// permissive one and the provider is the strict one, which is the safe
		// direction: the request-time gate fails closed even where the boot
		// gate would allow the process to start.
		{name: "local with surrounding whitespace", value: " local "},
		{name: "local_test", value: "local_test"},
		{name: "localhost", value: "localhost"},
		{name: "whitespace only", value: "   "},
	}

	for _, tc := range nonLocal {
		t.Run(tc.name, func(t *testing.T) {
			clearAuthEnv(t)
			t.Setenv("SIGAP_ENV", tc.value)
			t.Setenv(localRBACIdentityEnv, "true")

			res := &callCountingResolver{perms: ResolvedPermissions{
				Permissions: []string{"schedule.manage", "facility.manage"},
				AppUserID:   "11111111-1111-1111-1111-111111111111",
			}}
			p := NewLocalRBACTestIdentityProvider(res)

			if p.Armed() {
				t.Fatalf("SIGAP_ENV=%q must not arm the local test identity selector", tc.value)
			}

			actor, err := p.Authenticate(forgedImpersonationRequest())
			if err != nil {
				t.Fatalf("expected a silent zero actor, got error %v", err)
			}
			if !actor.IsZero() {
				t.Errorf("a disarmed selector must name nobody, got type=%s user=%q perms=%v",
					actor.Type, actor.UserID, actor.Permissions)
			}
			assertNoPrivilegedPermissions(t, actor, "disarmed selector")
			if res.calls != 0 {
				t.Errorf("the resolver was consulted %d time(s) by a disarmed selector "+
					"(subject %q) — the header reached the authorization path", res.calls, res.subject)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Forged headers when the selector flag is off (in local)
// ---------------------------------------------------------------------------

// providerAsMainWiresIt mirrors the provider-selection branch in
// cmd/server/main.go (the wiring block ending at L317): the configured
// provider is built from the auth mode, and the local test selector REPLACES
// it only when os.Getenv(SIGAP_LOCAL_RBAC_TEST_IDENTITY) == "true" AND the
// provider reports Armed().
//
// It is a copy rather than a call because main() is not extractable without a
// refactor, and refactoring the server entry point is not this audit's job.
// The copy is narrow on purpose — it exists so the tests below can assert what
// the composed server does (flag off ⇒ selector not installed ⇒ forged header
// inert) instead of only what the individual pieces do.
func providerAsMainWiresIt(cfg *AuthConfig, resolver Resolver) Provider {
	var provider Provider
	if cfg.Mode == AuthModeJWT {
		if resolver != nil {
			provider = NewJWTProviderWithResolver(*cfg, resolver)
		} else {
			provider = NewJWTProvider(*cfg)
		}
	} else {
		provider = NewProvider(cfg)
	}

	if os.Getenv(localRBACIdentityEnv) != "true" {
		return provider
	}
	if resolver == nil {
		// main.go leaves the selector disarmed with no resolver; the
		// configured provider stands.
		return provider
	}
	if local := NewLocalRBACTestIdentityProvider(resolver); local.Armed() {
		return local
	}
	return provider
}

// TestImpersonationLocalSelectorIsNotInstalledWhenItsFlagIsOff pins the second
// gate: SIGAP_ENV=local alone must not install the selector. The flag is the
// installer; the environment only decides whether installation is honoured.
func TestImpersonationLocalSelectorIsNotInstalledWhenItsFlagIsOff(t *testing.T) {
	// Values that are not the exact string "true". "TRUE" is included because
	// config.GuardDevCapabilities treats it as dangerous (EqualFold) while the
	// installer does not — refused at boot AND not installed, so the
	// asymmetry can only ever be the safe way round.
	flagValues := []string{"", "false", "False", "FALSE", "TRUE", "1", "yes", "on"}

	for _, flagValue := range flagValues {
		name := "unset"
		if flagValue != "" {
			name = flagValue
		}
		t.Run(name, func(t *testing.T) {
			clearAuthEnv(t)
			t.Setenv("SIGAP_ENV", "local")
			t.Setenv(localRBACIdentityEnv, flagValue)

			res := &callCountingResolver{perms: ResolvedPermissions{
				Permissions: []string{"schedule.manage"},
			}}

			// Disabled mode is the production-shaped default: no provider at
			// all, so the auth middleware is a pass-through.
			provider := providerAsMainWiresIt(&AuthConfig{Mode: AuthModeDisabled}, res)
			if provider != nil {
				t.Fatalf("%s=%q must not install the local selector; got provider %T",
					localRBACIdentityEnv, flagValue, provider)
			}

			var got identity.Actor
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = identity.ActorFromContext(r.Context())
			})
			rec := httptest.NewRecorder()

			Middleware(provider)(handler).ServeHTTP(rec, forgedImpersonationRequest())

			if !got.IsZero() {
				t.Errorf("forged headers produced an actor with the selector flag off: "+
					"type=%s user=%q perms=%v", got.Type, got.UserID, got.Permissions)
			}
			assertNoPrivilegedPermissions(t, got, "selector flag off")
			if res.calls != 0 {
				t.Errorf("the resolver was consulted %d time(s) although the selector was not installed", res.calls)
			}
		})
	}
}

// TestImpersonationLocalSelectorArmsOnEnvironmentAlone pins the documented
// division of labour, so a future change that moves the flag into the provider
// is a visible edit rather than a silent widening of the request-time gate.
//
// The provider arms on SIGAP_ENV=local alone. The flag is enforced by
// main.go's installer (L298) and by config.GuardDevCapabilities at boot; both
// are outside the provider by design, because the provider must be usable by a
// test process that sets SIGAP_ENV=local deliberately.
func TestImpersonationLocalSelectorArmsOnEnvironmentAlone(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("SIGAP_ENV", "local")

	p := NewLocalRBACTestIdentityProvider(&callCountingResolver{})
	if !p.Armed() {
		t.Error("the provider's arming condition is SIGAP_ENV=local (the installer in " +
			"cmd/server/main.go enforces the flag separately): if that changed, this test " +
			"and design/impersonation-production-guard.md must change with it")
	}
}

// ---------------------------------------------------------------------------
// 3. Forged X-Sigap-Dev-User-ID when dev identity is off
// ---------------------------------------------------------------------------

// TestImpersonationForgedDevUserIdIsInertWhenDevIdentityIsOff covers the dev
// identity family. The provider compares the env to "true" exactly, so every
// other truthy-looking spelling leaves it disabled.
func TestImpersonationForgedDevUserIdIsInertWhenDevIdentityIsOff(t *testing.T) {
	// "TRUE", "True", " true " and "1" are all NOT "true" for the provider —
	// the deliberate strict direction. config.GuardDevCapabilities does treat
	// the case variants as dangerous and refuses to boot with them outside
	// local, which is asserted in the boot-guard test below.
	values := []string{"", "false", "False", "FALSE", "TRUE", "True", " true ", "1", "yes", "on", "disabled"}

	for _, v := range values {
		name := v
		if name == "" {
			name = "unset"
		}
		t.Run(name, func(t *testing.T) {
			clearAuthEnv(t)
			t.Setenv(devIdentityEnv, v)

			p := NewDevIdentityProvider()
			actor, err := p.Authenticate(forgedImpersonationRequest())
			if err != nil {
				t.Fatalf("expected a silent zero actor, got error %v", err)
			}
			if !actor.IsZero() {
				t.Errorf("SIGAP_DEV_IDENTITY=%q must leave the dev header inert, got "+
					"type=%s user=%q perms=%v", v, actor.Type, actor.UserID, actor.Permissions)
			}
			assertNoPrivilegedPermissions(t, actor, "dev identity off")
		})
	}
}

// TestImpersonationDevIdentityCannotBeEscalatedByHeader is the armed case: the
// dev header names an actor, and NO header may widen it. The dev identity is
// read-only by construction, so a forged subject, role or permission header
// must not add a write key.
func TestImpersonationDevIdentityCannotBeEscalatedByHeader(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv(devIdentityEnv, "true")

	p := NewDevIdentityProvider()
	actor, err := p.Authenticate(forgedImpersonationRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.IsZero() {
		t.Fatal("precondition failed: the dev identity should be armed by SIGAP_DEV_IDENTITY=true")
	}

	// The identity comes from the dev header, not from the local test subject
	// that also travelled on this request — the two name different subjects,
	// so this assertion can tell them apart.
	if actor.UserID != "forged-dev-user" {
		t.Errorf("userID = %q, want the X-Sigap-Dev-User-ID value", actor.UserID)
	}
	if !actor.IsDev || actor.Type != identity.ActorDev {
		t.Errorf("actor must be a dev actor, got type=%s IsDev=%v", actor.Type, actor.IsDev)
	}
	if actor.AppUserID != "" {
		t.Errorf("the dev identity has no DB provenance; AppUserID must stay empty, got %q", actor.AppUserID)
	}
	if len(actor.FacilityGrants) != 0 {
		t.Errorf("the dev identity must carry no facility grants, got %d", len(actor.FacilityGrants))
	}

	assertNoPrivilegedPermissions(t, actor, "armed dev identity")

	// And the exact read-only set, so neither a forged header nor a future
	// edit can quietly add a member.
	wantReadOnly := map[string]bool{
		"facility.read":     true,
		"notification.read": true,
		"appointment.read":  true,
		"queue.read":        true,
		"schedule.read":     true,
		"audit.read":        true,
	}
	got := permSet(actor.Permissions)
	for perm := range got {
		if !wantReadOnly[perm] {
			t.Errorf("dev identity holds unexpected permission %q; the set must stay read-only", perm)
		}
	}
	for perm := range wantReadOnly {
		if !got[perm] {
			t.Errorf("dev identity is missing expected read-only permission %q", perm)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. The local selector cannot be escalated by header
// ---------------------------------------------------------------------------

// TestImpersonationLocalSelectorNeverEscalatesViaHeaders is the armed-local
// case with a permissive resolver: whatever the resolver says arrives
// unchanged, and the forged `X-Sigap-Local-Test-*` escalation headers add
// nothing. That is the property that makes the selector safe to ship in local:
// it selects an identity, it never grants one.
func TestImpersonationLocalSelectorNeverEscalatesViaHeaders(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv(localRBACIdentityEnv, "true")

	wantPerms := []string{"schedule.manage", "schedule.read", "facility.read"}
	res := &callCountingResolver{perms: ResolvedPermissions{
		Permissions: wantPerms,
		Grants: []identity.FacilityGrant{
			{Key: "schedule.manage", Unrestricted: true},
		},
		AppUserID: "22222222-2222-2222-2222-222222222222",
	}}

	p := NewLocalRBACTestIdentityProvider(res)
	if !p.Armed() {
		t.Fatal("precondition failed: SIGAP_ENV=local must arm the selector")
	}

	actor, err := p.Authenticate(forgedImpersonationRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.UserID != "local-global-super-admin" {
		t.Errorf("userID = %q, want the header's subject", actor.UserID)
	}
	if res.calls != 1 {
		t.Errorf("resolver calls = %d, want exactly 1", res.calls)
	}
	if res.subject != "local-global-super-admin" {
		t.Errorf("resolver subject = %q, want the header's subject", res.subject)
	}

	got := permSet(actor.Permissions)
	if len(actor.Permissions) != len(wantPerms) {
		t.Fatalf("permissions = %v, want exactly %v", actor.Permissions, wantPerms)
	}
	for _, perm := range wantPerms {
		if !got[perm] {
			t.Errorf("missing resolver permission %q", perm)
		}
	}
	// The forged escalation namespace must have contributed nothing.
	for _, forged := range []string{"*", "super_admin", "admin"} {
		if got[forged] {
			t.Errorf("forged header value %q landed in the permission set", forged)
		}
	}
	if actor.IsDev {
		t.Error("the local selector must produce a normal user actor, never a dev actor")
	}
	if actor.AppUserID != res.perms.AppUserID {
		t.Errorf("AppUserID = %q, want the resolver's %q", actor.AppUserID, res.perms.AppUserID)
	}
}

// TestImpersonationLocalSelectorForgeYieldsNothingWithoutSeededRoles is the
// case that matters most for an attacker: a maximally privileged-looking
// subject name plus a forged permission header, against a subject the database
// has no roles for. The answer must be "no permissions at all".
func TestImpersonationLocalSelectorForgeYieldsNothingWithoutSeededRoles(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv(localRBACIdentityEnv, "true")

	// emptyResolver returns an identity with no permissions — the shape an
	// unseeded subject produces.
	p := NewLocalRBACTestIdentityProvider(emptyResolver{})

	actor, err := p.Authenticate(forgedImpersonationRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actor.Permissions) != 0 {
		t.Errorf("an unseeded subject must resolve to zero permissions, got %v", actor.Permissions)
	}
	if len(actor.FacilityGrants) != 0 {
		t.Errorf("an unseeded subject must resolve to zero grants, got %d", len(actor.FacilityGrants))
	}
	assertNoPrivilegedPermissions(t, actor, "unseeded subject with forged headers")
}

// ---------------------------------------------------------------------------
// 5. The JWT path ignores permission claims and forged headers
// ---------------------------------------------------------------------------

// jwtImpersonationFixture builds a JWKS-backed provider plus a signed token
// whose claims attempt the escalation.
func jwtImpersonationFixture(t *testing.T, resolver Resolver, claimPerms []string) (*JWTProvider, string) {
	t.Helper()

	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "impersonation-key")
	t.Cleanup(jwksSrv.Close)

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}

	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    cfg.Issuer,
			Audience:  jwt.ClaimStrings{cfg.Audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
		Permissions: claimPerms,
	}

	var p *JWTProvider
	if resolver != nil {
		p = NewJWTProviderWithResolver(cfg, resolver)
	} else {
		p = NewJWTProvider(cfg)
	}
	return p, signTestToken(t, claims, key, "impersonation-key")
}

// TestImpersonationJWTProviderIgnoresPermissionClaims proves that a valid,
// correctly signed token cannot authorize itself (AUDIT-101). The claims ask
// for everything; the resolver says queue.read; the actor gets queue.read.
func TestImpersonationJWTProviderIgnoresPermissionClaims(t *testing.T) {
	resolver := &callCountingResolver{perms: ResolvedPermissions{
		Permissions: []string{"queue.read"},
		AppUserID:   "33333333-3333-3333-3333-333333333333",
	}}
	p, tokenStr := jwtImpersonationFixture(t, resolver,
		[]string{"*", "super_admin", "schedule.manage", "facility.manage"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.IsZero() {
		t.Fatal("a valid token must authenticate")
	}
	if actor.UserID != "user-42" {
		t.Errorf("userID = %q, want the token subject", actor.UserID)
	}
	if len(actor.Permissions) != 1 || actor.Permissions[0] != "queue.read" {
		t.Errorf("permissions = %v, want exactly the resolver's [queue.read]", actor.Permissions)
	}
	assertNoPrivilegedPermissions(t, actor, "token-claimed permissions")

	// `sub` is identity, never authorization: the resolver must have been
	// asked, and the token's own claim set must not have been used as a
	// fallback.
	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want exactly 1", resolver.calls)
	}
	if resolver.subject != "user-42" {
		t.Errorf("resolver subject = %q, want the token subject", resolver.subject)
	}
}

// TestImpersonationJWTProviderIgnoresForgedHeaders proves the dev/test header
// families carry no meaning on the JWT path. A browser that sets both — or an
// attacker replaying a valid token alongside them — changes nothing.
func TestImpersonationJWTProviderIgnoresForgedHeaders(t *testing.T) {
	resolver := &callCountingResolver{perms: ResolvedPermissions{
		Permissions: []string{"queue.read"},
		AppUserID:   "33333333-3333-3333-3333-333333333333",
	}}
	p, tokenStr := jwtImpersonationFixture(t, resolver, nil)

	// (a) Forged identity headers plus a valid token: the token wins, the
	// headers contribute nothing.
	req := forgedImpersonationRequest()
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.UserID != "user-42" {
		t.Errorf("userID = %q, want the token subject (a forged header named the actor)", actor.UserID)
	}
	if actor.IsDev {
		t.Error("forged dev headers must not mark the actor as dev")
	}
	if resolver.calls != 1 || resolver.subject != "user-42" {
		t.Errorf("resolver consulted with subject %q (%d calls), want the token subject once",
			resolver.subject, resolver.calls)
	}

	// (b) Forged headers with no Authorization at all: nobody.
	actor, err = p.Authenticate(forgedImpersonationRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("JWT mode with no bearer token must name nobody, got type=%s user=%q",
			actor.Type, actor.UserID)
	}
	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d after an unauthenticated request, want 1 (unauthenticated "+
			"requests must not reach the resolver)", resolver.calls)
	}
}

// TestImpersonationJWTProviderFailsClosedWithoutClaimFallback covers the two
// degraded wirings: a resolver error, and no resolver at all. In neither case
// may the token's `permissions` claim become the authorization source.
func TestImpersonationJWTProviderFailsClosedWithoutClaimFallback(t *testing.T) {
	claimPerms := []string{"*", "schedule.manage"}

	t.Run("resolver error yields a zero actor, not the claims", func(t *testing.T) {
		p, tokenStr := jwtImpersonationFixture(t, errResolver{}, claimPerms)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)

		actor, err := p.Authenticate(req)
		if err != nil {
			t.Fatalf("expected a silent fail-closed, got error %v", err)
		}
		if !actor.IsZero() {
			t.Errorf("a resolver error must fail closed, got type=%s user=%q perms=%v",
				actor.Type, actor.UserID, actor.Permissions)
		}
		assertNoPrivilegedPermissions(t, actor, "resolver error")
	})

	t.Run("no resolver yields identity but zero permissions", func(t *testing.T) {
		p, tokenStr := jwtImpersonationFixture(t, nil, claimPerms)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)

		actor, err := p.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if actor.UserID != "user-42" {
			t.Errorf("userID = %q, want the token subject", actor.UserID)
		}
		if len(actor.Permissions) != 0 {
			t.Errorf("without a server-side resolver the actor must hold no permissions, got %v",
				actor.Permissions)
		}
		assertNoPrivilegedPermissions(t, actor, "no resolver")
	})
}

// ---------------------------------------------------------------------------
// 6. The boot guard: the layer that makes all of the above unreachable
// ---------------------------------------------------------------------------

// TestImpersonationBootGuardRefusesArmableCombinations proves the production
// block for the env levers. Every combination in which a selector could be
// armed outside local must refuse to start; the local equivalents must start.
//
// The request-time tests above show the selectors are inert outside local. This
// test shows the stronger property: outside local, a process that asks for one
// does not run at all, so "inert" is not the only thing standing in the way.
func TestImpersonationBootGuardRefusesArmableCombinations(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		flags   map[string]string
		wantErr bool
		// errContains is checked when wantErr is true.
		errContains string
	}{
		{
			name:        "production + local RBAC test identity",
			env:         "production",
			flags:       map[string]string{localRBACIdentityEnv: "true"},
			wantErr:     true,
			errContains: localRBACIdentityEnv + " is only allowed when SIGAP_ENV=local",
		},
		{
			name:        "staging + local RBAC test identity",
			env:         "staging",
			flags:       map[string]string{localRBACIdentityEnv: "true"},
			wantErr:     true,
			errContains: localRBACIdentityEnv + " is only allowed when SIGAP_ENV=local",
		},
		{
			name:        "unset env + local RBAC test identity",
			env:         "",
			flags:       map[string]string{localRBACIdentityEnv: "true"},
			wantErr:     true,
			errContains: localRBACIdentityEnv + " is only allowed when SIGAP_ENV=local",
		},
		{
			name:        "production + dev identity",
			env:         "production",
			flags:       map[string]string{devIdentityEnv: "true"},
			wantErr:     true,
			errContains: devIdentityEnv + " is only allowed when SIGAP_ENV=local",
		},
		{
			// An operator writing TRUE believes they enabled dev identity
			// (the provider does not), and an operator reading the env may
			// believe they disabled it. The guard's EqualFold makes the boot
			// refusal cover the variant the provider ignores.
			name:        "production + dev identity, uppercase variant",
			env:         "production",
			flags:       map[string]string{devIdentityEnv: "TRUE"},
			wantErr:     true,
			errContains: devIdentityEnv + " is only allowed when SIGAP_ENV=local",
		},
		{
			name:        "production + auth mode dev",
			env:         "production",
			flags:       map[string]string{"SIGAP_AUTH_MODE": "dev"},
			wantErr:     true,
			errContains: "SIGAP_AUTH_MODE is only allowed when SIGAP_ENV=local",
		},
		{
			name: "production + every dev flag",
			env:  "production",
			flags: map[string]string{
				localRBACIdentityEnv: "true",
				devIdentityEnv:       "true",
				"SIGAP_AUTH_MODE":    "dev",
			},
			wantErr:     true,
			errContains: localRBACIdentityEnv + " is only allowed when SIGAP_ENV=local",
		},
		{
			// SIGAP_LOCAL_E2E_ACTOR is consumed by the WEB tier
			// (apps/web/src/lib/server/auth.ts) and by the ops preflight
			// (scripts/ops/preflight-production-env.sh). The Go process never
			// reads it, so the Go boot guard has no opinion on it — the block
			// for that lever is the web gate plus the ops preflight, and the
			// header it emits is inert outside local (proved above). Pinned
			// here so the division of responsibility is a stated fact rather
			// than an assumption.
			name:    "production + web-side E2E actor variable only",
			env:     "production",
			flags:   map[string]string{"SIGAP_LOCAL_E2E_ACTOR": "local-global-super-admin"},
			wantErr: false,
		},
		{
			name: "local + local RBAC test identity",
			env:  "local",
			flags: map[string]string{
				localRBACIdentityEnv: "true",
			},
			wantErr: false,
		},
		{
			name: "local + dev identity",
			env:  "local",
			flags: map[string]string{
				devIdentityEnv: "true",
			},
			wantErr: false,
		},
		{
			name: "local + every dev flag",
			env:  "local",
			flags: map[string]string{
				localRBACIdentityEnv: "true",
				devIdentityEnv:       "true",
				"SIGAP_AUTH_MODE":    "dev",
			},
			wantErr: false,
		},
		{
			name: "LOCAL is local to the boot guard",
			env:  "LOCAL",
			flags: map[string]string{
				localRBACIdentityEnv: "true",
				devIdentityEnv:       "true",
			},
			wantErr: false,
		},
		{
			name: "production + flags present but disarmed values",
			env:  "production",
			flags: map[string]string{
				localRBACIdentityEnv: "false",
				devIdentityEnv:       "false",
				"SIGAP_AUTH_MODE":    "jwt",
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAuthEnv(t)
			t.Setenv("SIGAP_ENV", tc.env)
			for k, v := range tc.flags {
				t.Setenv(k, v)
			}

			err := config.GuardDevCapabilities()
			if (err != nil) != tc.wantErr {
				t.Fatalf("GuardDevCapabilities() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr && tc.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error = %v, want it to contain %q", err, tc.errContains)
				}
			}
		})
	}
}

// TestImpersonationLoopbackGuardRequiresLoopbackForDevCapabilities records the
// second boot gate: in local, an armed dev capability may not be exposed on a
// non-loopback address. This is what keeps the dev header off the network
// during local development on a shared machine or a LAN-bound container.
//
// FINDING (recorded here, not fixed here): `config.GuardDevNetworkBinding`
// tests SIGAP_DEV_IDENTITY, SIGAP_AUTH_MODE and SIGAP_ENGINE_FALLBACK, and does
// NOT test SIGAP_LOCAL_RBAC_TEST_IDENTITY. The third case below pins that
// asymmetry so it is a stated fact rather than an assumption. The consequence
// is narrow — SIGAP_ENV=local AND the selector flag must both be set — but a
// compose stack that runs `SIGAP_ENV=local` (as many local stacks do) and
// publishes 0.0.0.0 would expose header-driven, DB-backed identity selection to
// anything that can reach the port. See
// design/impersonation-production-guard.md §6.
func TestImpersonationLoopbackGuardRequiresLoopbackForDevCapabilities(t *testing.T) {
	cases := []struct {
		name    string
		flags   map[string]string
		host    string
		wantErr bool
	}{
		{
			name:    "local + dev identity + loopback",
			flags:   map[string]string{devIdentityEnv: "true"},
			host:    "127.0.0.1",
			wantErr: false,
		},
		{
			name:    "local + dev identity + all interfaces",
			flags:   map[string]string{devIdentityEnv: "true"},
			host:    "0.0.0.0",
			wantErr: true,
		},
		{
			// F1 FIXED: the local RBAC test selector is now part of the loopback
			// guard, so publishing it on all interfaces is refused at boot.
			name:    "local + local RBAC test identity + all interfaces (REJECTED)",
			flags:   map[string]string{localRBACIdentityEnv: "true"},
			host:    "0.0.0.0",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAuthEnv(t)
			t.Setenv("SIGAP_ENV", "local")
			t.Setenv("SIGAP_API_HOST", tc.host)
			for k, v := range tc.flags {
				t.Setenv(k, v)
			}

			err := config.GuardDevNetworkBinding()
			if (err != nil) != tc.wantErr {
				t.Fatalf("GuardDevNetworkBinding() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
