package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// Phase 3B5.0 — production safety of the local DB-backed test identity selector
// ---------------------------------------------------------------------------
//
// The selector lets a request name which seeded subject it wants to be. That
// is a powerful capability, and it exists so local E2E can drive a real
// facility-scoped schedule.manage without the dev identity gaining a
// permission it deliberately does not have.
//
// The tests here are the proof that it cannot be reached outside local. They
// are deliberately adversarial about the ONE thing that would matter in an
// incident: a production process that somehow has the flag set. Each test
// below sets the flag AND supplies the header, because either alone would be
// a weaker scenario than reality.

const localTestSubjectHeaderName = "X-Sigap-Local-Test-Subject"

// TestLocalRBACTestIdentity_CannotArmOutsideLocal is the core §13 proof.
//
// The important subtlety is the case-sensitivity requirement. An earlier
// reading of "case-insensitive like the other guards" would be a real
// vulnerability here: envguard's other flags are dangerous only when they are
// SET, and EqualFold on "local" would let SIGAP_ENV=Local arm the selector in
// an environment whose operator believed they had not. The provider therefore
// requires an exact "local". These cases pin that.
func TestLocalRBACTestIdentity_CannotArmOutsideLocal(t *testing.T) {
	cases := []struct {
		name string
		env  string
	}{
		{name: "production", env: "production"},
		{name: "staging", env: "staging"},
		{name: "dev", env: "dev"},
		{name: "unset", env: ""},
		// The case variants are the point of this test, not padding.
		{name: "Local capitalised", env: "Local"},
		{name: "LOCAL uppercase", env: "LOCAL"},
		{name: "local with whitespace", env: " local "},
		{name: "local_test", env: "local_test"},
		{name: "localhost", env: "localhost"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SIGAP_ENV", tc.env)
			// Both the env flag AND the header are set: this is the worst
			// case, not a half-configured one.
			t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

			// A resolver that would happily grant everything if consulted.
			// If the provider ever consulted it outside local, this test
			// would see a fully privileged actor and fail.
			permissive := permissiveResolver{}
			p := NewLocalRBACTestIdentityProvider(permissive)

			if p.Armed() {
				t.Fatalf("SIGAP_ENV=%q must not arm the local test identity selector", tc.env)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
			req.Header.Set(localTestSubjectHeaderName, "e2e-schedule-manager@example.test")
			actor, err := p.Authenticate(req)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !actor.IsZero() {
				t.Errorf("a disarmed selector must authenticate nobody, got actor: "+
					"type=%s user=%q perms=%v grants=%d",
					actor.Type, actor.UserID, actor.Permissions, len(actor.FacilityGrants))
			}
		})
	}
}

// TestLocalRBACTestIdentity_ArmsInLocal pins the other direction: the selector
// must actually work locally, or the E2E it exists for cannot run. Without
// this, "never arms" would satisfy every test above.
func TestLocalRBACTestIdentity_ArmsInLocal(t *testing.T) {
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

	p := NewLocalRBACTestIdentityProvider(permissiveResolver{})
	if !p.Armed() {
		t.Fatal("SIGAP_ENV=local must arm the local test identity selector")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
	req.Header.Set(localTestSubjectHeaderName, "e2e-schedule-manager@example.test")
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if actor.IsZero() {
		t.Fatal("an armed selector in local must authenticate the named subject")
	}
	if actor.UserID != "e2e-schedule-manager@example.test" {
		t.Errorf("actor subject = %q, want the header value", actor.UserID)
	}
	// The actor must be a NORMAL user, never dev. IsDev=true would bypass
	// facility scope and permission-at-facility entirely, which would make
	// every E2E built on it meaningless.
	if actor.IsDev {
		t.Error("the local test identity must NOT be a dev actor; dev bypasses the " +
			"facility-scope checks this provider exists to exercise")
	}
	if actor.Type != identity.ActorUser {
		t.Errorf("actor type = %s, want %s", actor.Type, identity.ActorUser)
	}
}

// TestLocalRBACTestIdentity_NeverGrantsItsOwnPermissions is the anti-bypass
// proof. Even armed, in local, WITH a header, the provider must return
// exactly what the resolver said and nothing more. If it could add a
// permission of its own, the E2E would be testing a fiction.
func TestLocalRBACTestIdentity_NeverGrantsItsOwnPermissions(t *testing.T) {
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

	// The resolver says: this subject holds nothing. This is the exact
	// situation the E2E would hit with an unseeded subject, and it is where a
	// provider that quietly added a permission would do the most damage — the
	// test would pass while the E2E exercised a fiction.
	p := NewLocalRBACTestIdentityProvider(emptyResolver{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
	req.Header.Set(localTestSubjectHeaderName, "seeded-operator@example.test")
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(actor.Permissions) != 0 {
		t.Errorf("the provider must not add permissions the resolver did not return, got %v",
			actor.Permissions)
	}
	if len(actor.FacilityGrants) != 0 {
		t.Errorf("the provider must not synthesise facility grants, got %d", len(actor.FacilityGrants))
	}
	// Crucially, schedule.manage must NOT appear from nowhere. The whole
	// point of this provider is that access comes from seeded DB roles.
	for _, perm := range actor.Permissions {
		if perm == "schedule.manage" {
			t.Error("schedule.manage appeared without a seeded grant — the provider has a bypass")
		}
	}
}

// TestLocalRBACTestIdentity_PassesResolverResultVerbatim is the positive
// counterpart: whatever the resolver says must arrive unchanged. Without it,
// the empty case above would be satisfied by a provider that drops everything.
func TestLocalRBACTestIdentity_PassesResolverResultVerbatim(t *testing.T) {
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

	wantPerms := []string{"schedule.read", "schedule.manage", "facility.read"}
	wantGrants := []identity.FacilityGrant{
		{Key: "schedule.read", Unrestricted: true},
		{Key: "schedule.manage", Unrestricted: true},
		{Key: "facility.read", Unrestricted: true},
	}
	res := ResolvedPermissions{
		Permissions: wantPerms,
		Grants:      wantGrants,
		AppUserID:   "11111111-1111-1111-1111-111111111111",
	}

	p := NewLocalRBACTestIdentityProvider(staticResolver{resolved: res})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
	req.Header.Set(localTestSubjectHeaderName, "e2e-schedule-manager@example.test")
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(actor.Permissions) != len(wantPerms) {
		t.Fatalf("permissions = %v, want %v", actor.Permissions, wantPerms)
	}
	for i := range wantPerms {
		if actor.Permissions[i] != wantPerms[i] {
			t.Errorf("permissions[%d] = %q, want %q", i, actor.Permissions[i], wantPerms[i])
		}
	}
	if len(actor.FacilityGrants) != len(wantGrants) {
		t.Fatalf("grants = %+v, want %+v", actor.FacilityGrants, wantGrants)
	}
	for i := range wantGrants {
		if actor.FacilityGrants[i] != wantGrants[i] {
			t.Errorf("grants[%d] = %+v, want %+v", i, actor.FacilityGrants[i], wantGrants[i])
		}
	}
	if actor.AppUserID != res.AppUserID {
		t.Errorf("AppUserID = %q, want %q", actor.AppUserID, res.AppUserID)
	}
}

// emptyResolver is a real, fully-authorised-looking subject that the database
// simply has no roles for: the shape an unseeded subject produces.
type emptyResolver struct{}

func (emptyResolver) Resolve(_ context.Context, _ string) (ResolvedPermissions, error) {
	return ResolvedPermissions{AppUserID: "22222222-2222-2222-2222-222222222222"}, nil
}

// staticResolver returns a fixed resolution, so a test can assert that the
// provider forwards it without alteration.
type staticResolver struct {
	resolved ResolvedPermissions
}

func (s staticResolver) Resolve(_ context.Context, _ string) (ResolvedPermissions, error) {
	return s.resolved, nil
}

// TestLocalRBACTestIdentity_FailsClosedWithoutResolver covers the degenerate
// wiring where the flag is on but no DB pool exists. It must authenticate
// nobody rather than falling back to anything.
func TestLocalRBACTestIdentity_FailsClosedWithoutResolver(t *testing.T) {
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

	p := NewLocalRBACTestIdentityProvider(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
	req.Header.Set(localTestSubjectHeaderName, "anyone@example.test")
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("a selector with no resolver must authenticate nobody, got %+v", actor)
	}
}

// TestLocalRBACTestIdentity_NoHeaderMeansNoActor is the ordinary-path proof: in
// local, an armed selector must still leave normal requests untouched. It is
// a selector, not a blanket grant.
func TestLocalRBACTestIdentity_NoHeaderMeansNoActor(t *testing.T) {
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

	p := NewLocalRBACTestIdentityProvider(permissiveResolver{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("a request without the header must be unauthenticated, got %+v", actor)
	}
}

// TestLocalRBACTestIdentity_ResolverErrorFailsClosed proves there is no
// fallback path: a resolver error must not degrade into a partially
// populated actor.
func TestLocalRBACTestIdentity_ResolverErrorFailsClosed(t *testing.T) {
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

	p := NewLocalRBACTestIdentityProvider(errResolver{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
	req.Header.Set(localTestSubjectHeaderName, "e2e-schedule-manager@example.test")
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error (fail closed silently), got %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("a resolver error must fail closed, got %+v", actor)
	}
}

// TestLocalRBACTestIdentity_DoesNotConsumeAuthorizationHeader proves the
// selector does not weaken or replace Supabase auth. It reads its own header
// only; an Authorization header is irrelevant to it, so a bearer token cannot
// be used to drive the selector and a local test request cannot smuggle
// through a token.
func TestLocalRBACTestIdentity_DoesNotConsumeAuthorizationHeader(t *testing.T) {
	t.Setenv("SIGAP_ENV", "local")
	t.Setenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY", "true")

	p := NewLocalRBACTestIdentityProvider(permissiveResolver{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJSUzI1NiJ9.fake.signature")
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("a bearer token alone must not authenticate against the local selector, got %+v", actor)
	}
}

// permissiveResolver is a stand-in resolver that grants a broad set. It exists
// so the tests above can prove the PROVIDER adds nothing: if any of them
// observe these permissions while disarmed or with an empty resolution, the
// provider itself is the source.
type permissiveResolver struct{}

func (permissiveResolver) Resolve(_ context.Context, subject string) (ResolvedPermissions, error) {
	return ResolvedPermissions{
		Permissions: []string{"schedule.manage", "schedule.read", "facility.read"},
		Grants:      []identity.FacilityGrant{{Key: "schedule.manage", Unrestricted: true}},
		AppUserID:   uuid.NewString(),
	}, nil
}

// errResolver always fails, standing in for an unreachable database.
type errResolver struct{}

func (errResolver) Resolve(_ context.Context, _ string) (ResolvedPermissions, error) {
	return ResolvedPermissions{}, ErrClosed
}
