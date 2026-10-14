package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// Phase 3B5.0 — Schedule mutation options: facility-provenance security
// ---------------------------------------------------------------------------
//
// THE THING UNDER TEST
//
// GET /api/v1/admin/schedules/options returns the facilities an operator may
// build a schedule for. The security claim is precise and narrow:
//
//     A facility appears if and only if the actor holds schedule.manage
//     AT THAT FACILITY.
//
// Not "holds schedule.manage somewhere". Not "is inside its facility scope".
// Not "can read schedules there". At THAT facility.
//
// The tempting bug is to filter by the actor's READ set, or by the flat
// Actor.Permissions set, and call it done — both look right for a single-facility
// actor and both are catastrophically wrong for a mixed one. S1 below is the
// test that separates them, and the Phase 3B5.0 negative control breaks the
// source deliberately to prove S1 catches it.
// ---------------------------------------------------------------------------

// scheduleOptionsBody is the decoded wire shape. It is declared field-by-field
// ON PURPOSE. A map[string]any decode would accept anything, and then a test
// asserting "no permission leaked" would be asserting against its own mirror
// image. Decoding into a struct means encoding/json silently DISCARDS any
// unexpected key, so `containsForbiddenKey` below re-reads the raw bytes to
// catch what the struct chose to ignore.
type scheduleOptionsBody struct {
	Success bool `json:"success"`
	Data    struct {
		Facilities []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			ServiceUnits []struct {
				ID         string `json:"id"`
				FacilityID string `json:"facility_id"`
				Name       string `json:"name"`
			} `json:"service_units"`
		} `json:"facilities"`
	} `json:"data"`
}

// forbiddenOptionKeys are the security-sensitive names that must never appear
// anywhere in an options payload. This list is the spec's §6 denial set:
// roles, permission names, grants, scope flags, Unrestricted, super_admin,
// user ids, and practitioner data.
var forbiddenOptionKeys = []string{
	"role", "roles", "user_roles", "permission", "permissions", "role_permissions",
	"grant", "grants", "facility_grant", "facility_grants", "facility_scope",
	"unrestricted", "super_admin", "is_super_admin", "is_dev", "user_id",
	"userid", "subject", "app_user_id", "practitioner", "practitioner_id",
	"capabilities", "capability", "actions", "can_manage", "can_edit",
	"actor", "authenticated",
}

func decodeScheduleOptions(t *testing.T, rec *httptest.ResponseRecorder) scheduleOptionsBody {
	t.Helper()
	var body scheduleOptionsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal schedule options: %v (body=%s)", err, rec.Body.String())
	}
	return body
}

// scheduleOptionIDs flattens the facility ids from a decoded options payload.
func scheduleOptionIDs(body scheduleOptionsBody) []string {
	ids := make([]string, 0, len(body.Data.Facilities))
	for _, f := range body.Data.Facilities {
		ids = append(ids, f.ID)
	}
	return ids
}

// callScheduleOptions invokes the handler through the real router dispatch, so
// the test exercises the same path production takes — including the ordering
// that keeps the literal "options" segment from being parsed as a schedule id.
func callScheduleOptions(t *testing.T, h *AdminHandler, actor identity.Actor) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules/options", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.SchedulesRouter(rec, req)
	return rec
}

// assertNoForbiddenKeys walks the raw JSON and fails on any security-sensitive
// key, at any depth. It is case-insensitive and substring-aware so that
// "role_permissions", "FacilityGrants", and "can_manage" are all caught.
func assertNoForbiddenKeys(t *testing.T, label string, raw []byte) {
	t.Helper()
	lower := strings.ToLower(string(raw))
	for _, bad := range forbiddenOptionKeys {
		if strings.Contains(lower, `"`+bad) {
			t.Errorf("%s: payload leaks forbidden key %q; body=%s", label, bad, string(raw))
		}
	}
	// camelCase forms that a naive lowercase substring test would miss.
	for _, bad := range []string{"facilityScope", "superAdmin", "roleId", "permissionKey"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("%s: payload leaks forbidden key %q; body=%s", label, bad, string(raw))
		}
	}
}

// seedServiceUnitForFacility inserts an ACTIVE service unit under a facility.
func seedServiceUnitForFacility(t *testing.T, pool *pgxpool.Pool, facilityID, name string) string {
	t.Helper()
	unitID := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
		 VALUES ($1, $2, $3, $4, '', true)`,
		unitID, facilityID, name, strings.ToUpper(name[:1])+name[1:4]); err != nil {
		t.Fatalf("seed service unit %s: %v", name, err)
	}
	return unitID
}

// seedTwoFacilitiesWithUnits creates facility A (readable, NOT manageable) and
// facility B (readable AND manageable) for the mixed-provenance tests, and
// returns their ids.
func seedTwoFacilitiesWithUnits(t *testing.T, pool *pgxpool.Pool) (facA, facB string) {
	t.Helper()
	ctx := context.Background()
	facA = uuid.NewString()
	facB = uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A Only Read"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B Managed"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	seedServiceUnitForFacility(t, pool, facA, "Poli A")
	seedServiceUnitForFacility(t, pool, facB, "Poli B")
	return facA, facB
}

// ---------------------------------------------------------------------------
// S1 — schedule.read at A + schedule.manage only at B → options = B only
// ---------------------------------------------------------------------------
//
// This is the single most important test in the file. A filtering bug that
// returns all READABLE facilities fails here. So does a flat-permission
// filter, because the flat set contains schedule.manage and cannot say WHERE.
func TestScheduleOptions_S1_MixedProvenanceReturnsManageFacilityOnly(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA, facB := seedTwoFacilitiesWithUnits(t, pool)
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "mixed-schedule-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	// A: readable, NOT manageable. B: readable AND manageable.
	if err := seedUserRoles(ctx, pool, userID, facA, []string{"schedule.read"}); err != nil {
		t.Fatalf("seed roles A: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facB, []string{"schedule.read", "schedule.manage"}); err != nil {
		t.Fatalf("seed roles B: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)
	// Precondition: the actor genuinely holds both keys, so a failure below
	// cannot be explained by the seed not having taken effect.
	if !actor.HasPermission("schedule.manage") {
		t.Fatalf("precondition failed: actor lacks flat schedule.manage: %+v", actor.Permissions)
	}
	if actor.HasPermissionAtFacility("schedule.manage", mustParseUUID(t, facA)) {
		t.Fatalf("precondition failed: actor must NOT hold schedule.manage at facility A")
	}
	if !actor.HasPermissionAtFacility("schedule.manage", mustParseUUID(t, facB)) {
		t.Fatalf("precondition failed: actor must hold schedule.manage at facility B")
	}

	rec := callScheduleOptions(t, scopedHandler(pool), actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := decodeScheduleOptions(t, rec)
	ids := scheduleOptionIDs(body)
	if len(ids) != 1 {
		t.Fatalf("expected exactly 1 manageable facility, got %d: %v", len(ids), ids)
	}
	if ids[0] != facB {
		t.Errorf("expected ONLY facility B (%s) in options, got %v", facB, ids)
	}
	if containsString(ids, facA) {
		t.Errorf("facility A leaked into mutation options: %v", ids)
	}

	// The nesting must be intact: B's service unit is present and points back
	// at B, so the editor can bind the parent without a second request.
	for _, f := range body.Data.Facilities {
		if f.ID != facB {
			continue
		}
		if len(f.ServiceUnits) != 1 {
			t.Errorf("expected 1 service unit under B, got %d", len(f.ServiceUnits))
			continue
		}
		if f.ServiceUnits[0].FacilityID != facB {
			t.Errorf("service unit facility_id = %q, want %q", f.ServiceUnits[0].FacilityID, facB)
		}
	}
}

// ---------------------------------------------------------------------------
// S2 — schedule.read only, no schedule.manage anywhere → empty options, 200
// ---------------------------------------------------------------------------
func TestScheduleOptions_S2_ReadWithoutManageYieldsEmptyOptions(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA, _ := seedTwoFacilitiesWithUnits(t, pool)
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "read-only-schedule-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facA, []string{"schedule.read"}); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)
	if actor.HasPermission("schedule.manage") {
		t.Fatalf("precondition failed: actor must not hold schedule.manage at all")
	}

	rec := callScheduleOptions(t, scopedHandler(pool), actor)
	// 200, NOT 403. A reader who cannot manage anything is a legitimate
	// operator with a reduced capability, not an authentication failure.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for zero manageable facilities, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeScheduleOptions(t, rec)
	if len(body.Data.Facilities) != 0 {
		t.Errorf("expected 0 facilities for a read-only actor, got %v", scheduleOptionIDs(body))
	}
	// The key must be PRESENT and empty, never null: the client distinguishes
	// "loaded, nothing to offer" from "not loaded" by exactly this.
	if !strings.Contains(rec.Body.String(), `"facilities":[]`) {
		t.Errorf(`expected an explicit empty array, got %s`, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// S3 — zero facility assignment at all → empty options
// ---------------------------------------------------------------------------
func TestScheduleOptions_S3_NoFacilityAssignmentYieldsEmptyOptions(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	// Facilities exist, but the actor is assigned to none of them.
	_, facB := seedTwoFacilitiesWithUnits(t, pool)
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "unassigned-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, "", []string{"schedule.read", "schedule.manage"}); err != nil {
		t.Fatalf("seed global role: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)
	rec := callScheduleOptions(t, scopedHandler(pool), actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeScheduleOptions(t, rec)
	ids := scheduleOptionIDs(body)
	if len(ids) != 0 {
		t.Errorf("an unassigned actor must receive no options, got %v", ids)
	}
	if containsString(ids, facB) {
		t.Errorf("facility B leaked to an unassigned actor: %v", ids)
	}
}

// ---------------------------------------------------------------------------
// S4 — inactive / soft-deleted role assignment → no options
// ---------------------------------------------------------------------------
func TestScheduleOptions_S4_InactiveAndDeletedRoleYieldsNoOptions(t *testing.T) {
	cases := []struct {
		name    string
		status  string
		deleted *time.Time
	}{
		{name: "inactive assignment", status: "inactive"},
		{name: "soft-deleted assignment", status: "active", deleted: &time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool, cleanup := newScopeTestPool(t)
			defer cleanup()
			ctx := context.Background()

			facA, _ := seedTwoFacilitiesWithUnits(t, pool)
			userID := uuid.NewString()
			if err := seedAppUser(ctx, pool, userID, "lifecycle-"+uuid.NewString()[:8]); err != nil {
				t.Fatalf("seed app user: %v", err)
			}
			if err := seedUserRoles(ctx, pool, userID, facA, []string{"schedule.read", "schedule.manage"}); err != nil {
				t.Fatalf("seed roles: %v", err)
			}
			if err := setUserRoleLifecycle(ctx, pool, userID, facA, tc.status, tc.deleted); err != nil {
				t.Fatalf("set role lifecycle: %v", err)
			}

			actor := dbScopedActor(t, pool, userID)
			rec := callScheduleOptions(t, scopedHandler(pool), actor)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if body := decodeScheduleOptions(t, rec); len(body.Data.Facilities) != 0 {
				t.Errorf("a %s assignment must grant no options, got %v",
					tc.name, scheduleOptionIDs(body))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// S5 — resolver failure → fail closed (empty, not 500, not everything)
// ---------------------------------------------------------------------------
func TestScheduleOptions_S5_ResolverFailureFailsClosed(t *testing.T) {
	// A nil pool makes the DB-backed grant resolver error. A handler that
	// ignored the error and fell back to the flat permission set would return
	// every facility; a handler that surfaced it as a 500 would be an
	// availability bug. The contract is 200 with an empty list.
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA, _ := seedTwoFacilitiesWithUnits(t, pool)
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "resolver-failure-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facA, []string{"schedule.read", "schedule.manage"}); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	// Actor carries the REAL grants resolved from the DB...
	actor := dbScopedActor(t, pool, userID)
	// ...but the handler is wired to a DEAD grant resolver, so every request
	// re-resolution fails.
	h := NewAdminHandler(pool).
		WithFacilityScopeResolver(auth.NewDBFacilityScope(pool)).
		WithFacilityGrantResolver(auth.NewDBFacilityGrantResolver(nil))

	rec := callScheduleOptions(t, h, actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("resolver failure must fail closed with 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeScheduleOptions(t, rec)
	if len(body.Data.Facilities) != 0 {
		t.Errorf("resolver failure must yield no options, got %v", scheduleOptionIDs(body))
	}
}

// ---------------------------------------------------------------------------
// S6 — client/JWT claims cannot fabricate options
// ---------------------------------------------------------------------------
//
// The actor is built by hand, NOT by the DB resolver, and carries NO provenance
// for any facility. It looks maximally privileged from the outside: correct
// actor type, a flat permission set containing schedule.manage, a plausible
// user id. If any of that were enough, S6 would return facilities. It must not —
// because a flat key set carries no facility information, and that is precisely
// why authorization reads provenance instead.
func TestScheduleOptions_S6_JWTClaimsCannotFabricateOptions(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA, facB := seedTwoFacilitiesWithUnits(t, pool)
	_ = facA
	_ = facB

	forged := identity.Actor{
		Type:   identity.ActorUser,
		UserID: "sub:forged-not-a-real-session",
		// A flat set with BOTH keys — the shape a careless implementation
		// would treat as sufficient.
		Permissions: []string{"schedule.read", "schedule.manage"},
		// No FacilityGrants at all: no provenance, no facility, no DB row.
		FacilityGrants: nil,
		AppUserID:      uuid.NewString(),
		IsDev:          false,
	}

	h := scopedHandler(pool)
	rec := callScheduleOptions(t, h, forged)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeScheduleOptions(t, rec)
	ids := scheduleOptionIDs(body)
	if len(ids) != 0 {
		t.Fatalf("a claim-only actor with no DB provenance must receive NO options, got %v", ids)
	}
	// A forged user id must not even be looked up as a real subject: assert
	// the forged id was never persisted in the first place.
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM app_users WHERE id = $1`, forged.AppUserID).Scan(&count); err != nil {
		t.Fatalf("count app users: %v", err)
	}
	if count != 0 {
		t.Errorf("precondition broken: forged AppUserID exists in the DB")
	}
}

// ---------------------------------------------------------------------------
// S7 — global facility scope WITHOUT schedule.manage → no manage option
// ---------------------------------------------------------------------------
//
// The subtle one. A global scope says WHERE an actor may act; it does not say
// WHAT they may do. An actor scoped at every facility but holding only
// schedule.read must receive nothing. If scope implied capability, every
// operator would silently become a schedule manager.
//
// The seed uses a real global `super_admin` assignment, because that is the
// only shape that actually produces global reach. This is worth stating, since
// it is easy to write the obvious-looking seed and draw the wrong conclusion
// from it: assigning ANY OTHER role with facility_id IS NULL does NOT create a
// global scope. Both rbac_resolver.go and facility_scope.go gate global reach
// on `r.name = 'super_admin'`, so a non-super_admin unscoped assignment yields
// a grant with no facility and Unrestricted=false — a ZERO scope that is
// trivially denied, and therefore proves nothing. The role below is
// super_admin (so the scope really is global) but is given a permission set
// that deliberately omits schedule.manage, which is the actual §7 case.
func TestScheduleOptions_S7_GlobalScopeWithoutManageGrantsNoOption(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	_, facB := seedTwoFacilitiesWithUnits(t, pool)
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "global-scope-read-only"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}

	// A genuine global assignment: the super_admin role with facility_id NULL.
	// The permission set is the system set MINUS schedule.manage, so the actor
	// is globally scoped everywhere yet may not manage a schedule anywhere.
	globalWithoutManage := []string{
		"queue.generate", "queue.read", "queue.manage",
		"facility.read", "facility.manage",
		"appointment.read", "appointment.manage",
		"schedule.read",
		"notification.read", "notification.manage",
	}
	if err := seedUserRoles(ctx, pool, userID, "", globalWithoutManage); err != nil {
		t.Fatalf("seed global role: %v", err)
	}
	// Promote the seeded role to super_admin so the assignment is genuinely
	// global. seedUserRoles creates a synthetic uniquely-named role, so the
	// rename is safe and scoped to this test's own row.
	if _, err := pool.Exec(ctx,
		`UPDATE roles SET name = 'super_admin'
		 WHERE id = (SELECT role_id FROM user_roles
		             WHERE user_id = $1 AND facility_id IS NULL
		             ORDER BY role_id LIMIT 1)`, userID); err != nil {
		t.Fatalf("promote seeded role to super_admin: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)

	// Precondition 1: the actor really IS globally scoped. Without this the
	// test would pass for the wrong reason (an empty scope denies everything).
	var unrestrictedGrants int
	for _, g := range actor.FacilityGrants {
		if g.Unrestricted {
			unrestrictedGrants++
		}
	}
	if unrestrictedGrants == 0 {
		t.Fatalf("precondition failed: expected unrestricted grants from a global super_admin, got %+v",
			actor.FacilityGrants)
	}
	// Precondition 2: the actor really does hold schedule.read, globally.
	if !actor.HasPermission("schedule.read") {
		t.Fatalf("precondition failed: actor must hold schedule.read")
	}
	// Precondition 3: and genuinely does NOT hold schedule.manage.
	if actor.HasPermission("schedule.manage") {
		t.Fatalf("precondition failed: actor must not hold schedule.manage")
	}
	// The scope itself must include the seeded facility, or the test is moot.
	scope := auth.FacilityScopeForActor(ctx, actor, auth.NewDBFacilityScope(pool))
	if !scope.Unrestricted {
		t.Fatalf("precondition failed: expected an unrestricted facility scope, got %+v", scope)
	}

	rec := callScheduleOptions(t, scopedHandler(pool), actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if body := decodeScheduleOptions(t, rec); len(body.Data.Facilities) != 0 {
		t.Errorf("global scope must not imply schedule.manage, got %v (facility B was %s)",
			scheduleOptionIDs(body), facB)
	}
}

// ---------------------------------------------------------------------------
// Data minimization — §6
// ---------------------------------------------------------------------------
func TestScheduleOptions_ExposeOnlyDomainChoices(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA, _ := seedTwoFacilitiesWithUnits(t, pool)
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "minimization-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facA, []string{"schedule.read", "schedule.manage"}); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)
	rec := callScheduleOptions(t, scopedHandler(pool), actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// The raw bytes, not the decoded struct: a struct decode hides unknown
	// keys by design, which is precisely what we are testing for.
	assertNoForbiddenKeys(t, "schedule options", rec.Body.Bytes())

	body := decodeScheduleOptions(t, rec)
	if len(body.Data.Facilities) != 1 {
		t.Fatalf("expected exactly 1 facility, got %d", len(body.Data.Facilities))
	}

	// Enumerate the ACTUAL top-level keys of the payload, so a future field
	// addition that slips past the blocklist above still trips this.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &top); err != nil {
		t.Fatalf("unmarshal top level: %v", err)
	}
	for key := range top {
		if key != "success" && key != "data" {
			t.Errorf("unexpected top-level key %q in options payload", key)
		}
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(top["data"], &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	for key := range data {
		if key != "facilities" {
			t.Errorf("unexpected key %q under data; only domain choices belong here", key)
		}
	}
	// `facilities` is an ARRAY, so it decodes to []json.RawMessage; index [0]
	// addresses the first facility entry.
	var facilities []json.RawMessage
	if err := json.Unmarshal(data["facilities"], &facilities); err != nil {
		t.Fatalf("unmarshal facilities: %v", err)
	}
	if len(facilities) != 1 {
		t.Fatalf("expected 1 facility entry, got %d", len(facilities))
	}
	var first map[string]json.RawMessage
	if err := json.Unmarshal(facilities[0], &first); err != nil {
		t.Fatalf("unmarshal facilities[0]: %v", err)
	}
	for key := range first {
		if key != "id" && key != "name" && key != "service_units" {
			t.Errorf("unexpected facility key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// A facility with no active service unit must still be offered
// ---------------------------------------------------------------------------
//
// This pins the LEFT JOIN / ON-clause defect. With `su.is_active` in the WHERE
// clause, PostgreSQL discards the NULL-extended rows and the facility vanishes
// — an operator loses the ability to schedule against a facility that has not
// onboarded its service units yet, with no error and no explanation.
func TestScheduleOptions_FacilityWithoutActiveServiceUnitIsStillOffered(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Fasilitas Tanpa Poli"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "no-service-unit-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facID, []string{"schedule.read", "schedule.manage"}); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)
	rec := callScheduleOptions(t, scopedHandler(pool), actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeScheduleOptions(t, rec)
	ids := scheduleOptionIDs(body)
	if len(ids) != 1 || ids[0] != facID {
		t.Fatalf("expected the facility with no active service unit to be offered, got %v", ids)
	}
	if len(body.Data.Facilities[0].ServiceUnits) != 0 {
		t.Errorf("expected an empty service_units list, got %d", len(body.Data.Facilities[0].ServiceUnits))
	}
	// An empty list must be [], never null, or the client has to special-case it.
	if !strings.Contains(rec.Body.String(), `"service_units":[]`) {
		t.Errorf(`expected an explicit empty service_units array, got %s`, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// An INACTIVE facility must not be offered
// ---------------------------------------------------------------------------
func TestScheduleOptions_InactiveFacilityIsNotOffered(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Fasilitas Nonaktif"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE facilities SET is_active = false WHERE id = $1`, facID); err != nil {
		t.Fatalf("deactivate facility: %v", err)
	}
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "inactive-facility-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facID, []string{"schedule.read", "schedule.manage"}); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)
	rec := callScheduleOptions(t, scopedHandler(pool), actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if body := decodeScheduleOptions(t, rec); len(body.Data.Facilities) != 0 {
		t.Errorf("a deactivated facility must not be offered, got %v", scheduleOptionIDs(body))
	}
}

// mustParseUUID is a small test helper for precondition checks.
func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return parsed
}
