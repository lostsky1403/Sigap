package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/migrate"
)

// ---------------------------------------------------------------------------
// Phase 3B5.0 — the seeded local mutation-test actors
// ---------------------------------------------------------------------------
//
// §11 needs a local operator who genuinely holds schedule.manage so the
// ScheduleEditor E2E can drive real authorization, and §12 needs a second
// actor whose facility provenance DIVERGES: readable at one facility, manager
// at another.
//
// The critical property of both is that they are ordinary RBAC data. They are
// app_users rows plus user_roles rows pointing at the EXISTING system roles
// from rbac.sql. There is no special-case permission, no bypass, and no code
// path that treats these subjects differently from any other. That is what
// makes them trustworthy as a test subject: the E2E exercises the same
// resolver production uses.
//
// These tests assert the SEED produces that shape. A seed that silently
// stopped diverging would let a Cartesian-widening regression pass its E2E,
// so the divergence is pinned here, against a real database, using the real
// resolver.
// ---------------------------------------------------------------------------

// deterministic UUIDs and subjects seeded by dev.sql + demo.sql.
const (
	localSeedScheduleManagerID = "00000000-0000-0000-0000-00000000d993"
	localSeedScheduleManager   = "e2e-schedule-manager"

	localSeedScheduleMixedID = "00000000-0000-0000-0000-00000000d994"
	localSeedScheduleMixed   = "e2e-schedule-mixed"

	// Facility B: the canonical demo facility, which the mixed actor manages.
	localSeedFacilityManaged = "00000000-0000-0000-0000-00000000d000"
	// Facility A: readable only, for the mixed actor.
	localSeedFacilityReadOnly = "00000000-0000-0000-0000-00000000e000"
)

// newLocalSeedPool builds a pool on a throwaway schema and applies migrations
// plus the full local seed chain, so the assertions below run against exactly
// the rows the local E2E stack will see.
func newLocalSeedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping local seed provenance test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "test_local_seed, public"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS test_local_seed CASCADE`); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA test_local_seed`); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	dir, err := migrate.DefaultDir()
	if err != nil {
		t.Fatalf("migrations dir: %v", err)
	}
	if _, err := migrate.Run(ctx, pool, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedDir := dir + "/../seed"
	for _, name := range []string{"dev.sql", "rbac.sql", "demo.sql"} {
		seed, err := os.ReadFile(seedDir + "/" + name)
		if err != nil {
			t.Fatalf("read seed %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(seed)); err != nil {
			t.Fatalf("apply seed %s: %v", name, err)
		}
	}
	return pool
}

// TestLocalSeed_ScheduleManagerHoldsManageAtOneFacility proves §11: the
// authorized actor resolves real schedule.manage provenance at exactly one
// facility, resolved through the production resolver.
func TestLocalSeed_ScheduleManagerHoldsManageAtOneFacility(t *testing.T) {
	pool := newLocalSeedPool(t)
	ctx := context.Background()

	resolved, err := NewRBACResolver(pool).(AppUserResolver).ResolveByAppUserID(ctx, localSeedScheduleManagerID)
	if err != nil {
		t.Fatalf("resolve schedule manager: %v", err)
	}
	if resolved.AppUserID != localSeedScheduleManagerID {
		t.Fatalf("AppUserID=%q want %q", resolved.AppUserID, localSeedScheduleManagerID)
	}

	actor := identity.Actor{
		Type:           identity.ActorUser,
		UserID:         localSeedScheduleManager,
		Permissions:    resolved.Permissions,
		FacilityGrants: resolved.Grants,
		AppUserID:      resolved.AppUserID,
	}

	if !actor.HasPermission("schedule.manage") {
		t.Fatalf("the local schedule manager must hold schedule.manage, got %v", actor.Permissions)
	}
	managed := uuid.MustParse(localSeedFacilityManaged)
	if !actor.HasPermissionAtFacility("schedule.manage", managed) {
		t.Error("the local schedule manager must hold schedule.manage AT the demo facility")
	}
	// A facility it is not assigned to must grant nothing, even though the
	// flat key is present. This is the same property S1 relies on.
	other := uuid.MustParse(localSeedFacilityReadOnly)
	if actor.HasPermissionAtFacility("schedule.manage", other) {
		t.Error("the local schedule manager must NOT hold schedule.manage at an unassigned facility")
	}
	// It must be a normal user actor, not dev: the whole value of this actor
	// is that facility-scope enforcement still applies to it.
	if actor.IsDev {
		t.Error("the seeded local actor must not be a dev actor")
	}
}

// TestLocalSeed_MixedActorDivergesByFacility proves §12, and it is the more
// important of the two.
//
// The flat permission union for this actor CONTAINS schedule.manage (it is a
// facility_admin somewhere). An implementation that authorised from the flat
// set would offer both facilities as mutation options. The contract requires
// only the managed one. Asserting the divergence in both directions is what
// makes the actor useful as a regression tripwire.
func TestLocalSeed_MixedActorDivergesByFacility(t *testing.T) {
	pool := newLocalSeedPool(t)
	ctx := context.Background()

	resolved, err := NewRBACResolver(pool).(AppUserResolver).ResolveByAppUserID(ctx, localSeedScheduleMixedID)
	if err != nil {
		t.Fatalf("resolve mixed actor: %v", err)
	}

	actor := identity.Actor{
		Type:           identity.ActorUser,
		UserID:         localSeedScheduleMixed,
		Permissions:    resolved.Permissions,
		FacilityGrants: resolved.Grants,
		AppUserID:      resolved.AppUserID,
	}

	facilityA := uuid.MustParse(localSeedFacilityReadOnly)
	facilityB := uuid.MustParse(localSeedFacilityManaged)

	// Precondition: the flat set DOES contain schedule.manage. Without this,
	// the divergence assertions below would be vacuous — an actor with no
	// manage key anywhere would trivially have none at A.
	if !actor.HasPermission("schedule.manage") {
		t.Fatalf("precondition failed: the mixed actor must hold the flat schedule.manage key "+
			"(it is a facility_admin at facility B); got %v", actor.Permissions)
	}
	// And it must read both facilities, so "not manageable at A" cannot be
	// confused with "not visible at A".
	if !actor.HasPermissionAtFacility("schedule.read", facilityA) {
		t.Error("precondition failed: the mixed actor must hold schedule.read at facility A")
	}
	if !actor.HasPermissionAtFacility("schedule.read", facilityB) {
		t.Error("precondition failed: the mixed actor must hold schedule.read at facility B")
	}

	// The actual divergence.
	if actor.HasPermissionAtFacility("schedule.manage", facilityA) {
		t.Error("facility A must NOT have schedule.manage; the divergence the security " +
			"tests depend on is missing from the seed")
	}
	if !actor.HasPermissionAtFacility("schedule.manage", facilityB) {
		t.Error("facility B must have schedule.manage")
	}

	// And the scope reflects both, so the facility filter narrows on
	// permission rather than on scope.
	scope := FacilityScopeForActor(ctx, actor, NewDBFacilityScope(pool))
	if scope.Err != nil {
		t.Fatalf("resolve facility scope: %v", scope.Err)
	}
	if scope.Unrestricted {
		t.Fatal("the mixed actor must be facility-scoped, not globally unrestricted")
	}
	var hasA, hasB bool
	for _, id := range scope.IDs {
		if id == facilityA {
			hasA = true
		}
		if id == facilityB {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Fatalf("the mixed actor's scope must contain both facilities, got %v", scope.IDs)
	}

	// The decisive assertion: intersecting the scope with permission-at-facility
	// must collapse two facilities down to one. This is exactly what the
	// options endpoint computes, so if this ever yields two, the endpoint would
	// be offering a facility the operator cannot manage.
	manageSet := AuthorizedFacilityIDsForPermission(actor, scope, "schedule.manage")
	if manageSet.Err != nil {
		t.Fatalf("resolve manage set: %v", manageSet.Err)
	}
	if len(manageSet.IDs) != 1 {
		t.Fatalf("manage set = %v, want exactly facility B (%s)", manageSet.IDs, facilityB)
	}
	if manageSet.IDs[0] != facilityB {
		t.Fatalf("manage set = %v, want facility B (%s)", manageSet.IDs, facilityB)
	}
}

// TestLocalSeed_DevIdentityIsReadOnlyNonPHI is the §11 guard that the obvious
// shortcut was NOT taken. The dev identity keeps a read-only, non-PHI
// permission set: schedule.manage is absent (Phase 3B5.0 requires a separate
// DB-backed actor instead), and — since the upstream main security fix
// (vuln-0002) — every write-level permission is absent too.
func TestLocalSeed_DevIdentityIsReadOnlyNonPHI(t *testing.T) {
	t.Setenv("SIGAP_DEV_IDENTITY", "true")
	p := NewDevIdentityProvider()
	if !p.enabled {
		t.Fatal("precondition failed: dev identity provider should be enabled")
	}

	actor := permissionsOfDevActor(t, p)

	// No write-level permission may be granted by a client-supplied header.
	for _, perm := range []string{
		"schedule.manage",
		"facility.manage",
		"appointment.manage",
		"notification.manage",
		"queue.manage",
		"queue.generate",
	} {
		if actor.HasPermission(perm) {
			t.Errorf("dev identity must NOT grant write permission %q (vuln-0002)", perm)
		}
	}
	// No unrestricted facility grant may exist either — that would satisfy the
	// facility-scoped mutation checks the permission list above is meant to fail.
	for _, g := range actor.FacilityGrants {
		if g.Unrestricted {
			t.Errorf("dev identity must not carry an unrestricted facility grant (key %q)", g.Key)
		}
	}
	// Spot-check that the read-only set is intact, so this test would notice if
	// someone "fixed" it by rewriting the list.
	for _, perm := range []string{"queue.read", "facility.read", "schedule.read", "notification.read", "appointment.read", "audit.read"} {
		if !actor.HasPermission(perm) {
			t.Errorf("dev identity unexpectedly lost %q", perm)
		}
	}
}

// permissionsOfDevActor drives the real dev provider through a request and
// returns the actor it produces, so the assertion above reads the shipped
// behaviour rather than a copy of it.
func permissionsOfDevActor(t *testing.T, p *DevIdentityProvider) identity.Actor {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules", nil)
	req.Header.Set("X-Sigap-Dev-User-ID", "local-e2e-dev-user")
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("dev identity authenticate: %v", err)
	}
	if actor.IsZero() {
		t.Fatal("dev identity produced a zero actor")
	}
	return actor
}

// TestLocalSeed_SeededActorsGrantNoHardcodedPermissions cross-checks the seed
// against the resolver for a subject that was never seeded. A newly minted
// subject must resolve to nothing, which proves the local selector selects
// identities rather than manufacturing them.
func TestLocalSeed_SeededActorsGrantNoHardcodedPermissions(t *testing.T) {
	pool := newLocalSeedPool(t)
	ctx := context.Background()

	resolved, err := NewRBACResolver(pool).Resolve(ctx, "a-subject-that-was-never-seeded")
	if err != nil {
		t.Fatalf("resolve unknown subject: %v", err)
	}
	if len(resolved.Permissions) != 0 {
		t.Errorf("an unseeded subject must resolve to zero permissions, got %v", resolved.Permissions)
	}
	if len(resolved.Grants) != 0 {
		t.Errorf("an unseeded subject must resolve to zero grants, got %+v", resolved.Grants)
	}
}
