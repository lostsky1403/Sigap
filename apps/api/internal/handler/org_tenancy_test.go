package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// Regional tenancy: organization-level cross-tenant isolation. Reuses the
// disposable test_scope harness from facility_scope_test.go, which now also
// applies 0013_organizations.sql via migrate.DefaultDir().
// ---------------------------------------------------------------------------

// orgAdminPermissions mirrors packages/db/seed/regional_tenancy.sql for the
// organization_admin role (org-scoped operational manage + org directory).
func orgAdminPermissions() []string {
	return []string{
		"organization.read", "organization.manage",
		"facility.read", "facility.manage",
		"queue.read", "queue.manage", "queue.generate",
		"appointment.read", "appointment.manage",
		"schedule.read", "schedule.manage",
		"notification.read",
	}
}

// regionalAdminPermissions mirrors regional_tenancy.sql for regional_admin:
// operational stats READ ONLY (including the org directory). No .manage keys
// at all — organization.manage included — no audit.read, no medical_record.read
// (which must not exist). This is the documented no-.manage invariant.
func regionalAdminPermissions() []string {
	return []string{
		"organization.read",
		"facility.read",
		"queue.read",
		"appointment.read",
		"schedule.read",
		"notification.read",
	}
}

func seedOrganization(ctx context.Context, pool *pgxpool.Pool, orgID, name, region string) {
	if _, err := pool.Exec(ctx,
		`INSERT INTO organizations (id, name, region, is_active) VALUES ($1, $2, $3, true)
		 ON CONFLICT (id) DO NOTHING`, orgID, name, region); err != nil {
		panic(err)
	}
}

func seedOrgFacility(ctx context.Context, pool *pgxpool.Pool, facilityID, name, orgID string) {
	if _, err := pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, short_code, organization_id)
		 VALUES ($1, $2, 'puskesmas', 'Jl. Org', 'Kec. Org', 'Kab. Org', 'Prov. Org', '021-1111111', 10, 5, $3, $4)
		 ON CONFLICT (id) DO NOTHING`, facilityID, name, "ORG"+facilityID[len(facilityID)-6:], orgID); err != nil {
		panic(err)
	}
}

func seedOrgServiceUnit(ctx context.Context, pool *pgxpool.Pool, id, facilityID, name, code string) {
	if _, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
		 VALUES ($1, $2, $3, $4, '', true)`, id, facilityID, name, code); err != nil {
		panic(err)
	}
}

// seedOrgScopedRole grants appUserID a named role scoped to orgID (facility_id
// NULL, organization_id NOT NULL) with the given permission keys. This is the
// org-axis analogue of seedNamedRoleAtFacility.
func seedOrgScopedRole(ctx context.Context, pool *pgxpool.Pool, appUserID, roleName, orgID string, perms []string) {
	if _, err := pool.Exec(ctx,
		`INSERT INTO roles (name, description, is_system) VALUES ($1, $2, true)
		 ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description`, roleName, "test "+roleName); err != nil {
		panic(err)
	}
	if err := attachPermissionsToRole(ctx, pool, roleName, perms); err != nil {
		panic(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id, facility_id, organization_id, status, deleted_at)
		 SELECT $1, id, NULL, $3, 'active', NULL FROM roles WHERE name = $2`,
		appUserID, roleName, orgID); err != nil {
		panic(err)
	}
}

// seedUnscopedRole grants appUserID a named role with facility_id NULL and
// organization_id NULL (global reach for super_admin/regional_admin).
func seedUnscopedRole(ctx context.Context, pool *pgxpool.Pool, appUserID, roleName string, perms []string) {
	if _, err := pool.Exec(ctx,
		`INSERT INTO roles (name, description, is_system) VALUES ($1, $2, true)
		 ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description`, roleName, "test "+roleName); err != nil {
		panic(err)
	}
	if err := attachPermissionsToRole(ctx, pool, roleName, perms); err != nil {
		panic(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id, facility_id, organization_id, status, deleted_at)
		 SELECT $1, id, NULL, NULL, 'active', NULL FROM roles WHERE name = $2`,
		appUserID, roleName); err != nil {
		panic(err)
	}
}

func dispatchOrgRequest(h *AdminHandler, method, path string, actor identity.Actor) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	ctx := identity.ContextWithActor(req.Context(), actor)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.OrganizationsRouter(rec, req)
	return rec
}

func decodeOrganizationList(t *testing.T, body []byte) []string {
	t.Helper()
	var resp struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode organization list: %v: %s", err, body)
	}
	ids := make([]string, 0, len(resp.Data))
	for _, o := range resp.Data {
		ids = append(ids, o.ID)
	}
	return ids
}

func orgTenancyFixture(t *testing.T) (*pgxpool.Pool, func(), string, string, string, string, string) {
	t.Helper()
	pool, cleanup := newScopeTestPool(t)
	ctx := context.Background()

	orgA := uuid.NewString()
	orgB := uuid.NewString()
	facA1 := uuid.NewString()
	facA2 := uuid.NewString()
	facB1 := uuid.NewString()

	seedOrganization(ctx, pool, orgA, "Org A (synthetic)", "Jawa Barat")
	seedOrganization(ctx, pool, orgB, "Org B (synthetic)", "DKI Jakarta")
	seedOrgFacility(ctx, pool, facA1, "Org A Facility 1", orgA)
	seedOrgFacility(ctx, pool, facA2, "Org A Facility 2", orgA)
	seedOrgFacility(ctx, pool, facB1, "Org B Facility 1", orgB)
	seedOrgServiceUnit(ctx, pool, uuid.NewString(), facA1, "Poli A1", "PA1")
	seedOrgServiceUnit(ctx, pool, uuid.NewString(), facB1, "Poli B1", "PB1")

	return pool, cleanup, orgA, orgB, facA1, facA2, facB1
}

// TestOrgTenancy_OrgAdminFacilityScopeExpansion proves an org-scoped
// organization_admin sees every facility of its org and none of a sibling org.
func TestOrgTenancy_OrgAdminFacilityScopeExpansion(t *testing.T) {
	pool, cleanup, orgA, _, facA1, facA2, facB1 := orgTenancyFixture(t)
	defer cleanup()
	ctx := context.Background()

	orgAdminA := uuid.NewString()
	if err := seedAppUser(ctx, pool, orgAdminA, "org-admin-a"); err != nil {
		t.Fatal(err)
	}
	seedOrgScopedRole(ctx, pool, orgAdminA, "organization_admin", orgA, orgAdminPermissions())

	actor := dbScopedActor(t, pool, orgAdminA)
	// Sanity: the resolver must have expanded the org grant into per-facility
	// provenance for both org A facilities.
	if !actor.HasPermissionAtFacility("facility.read", uuid.MustParse(facA1)) ||
		!actor.HasPermissionAtFacility("facility.read", uuid.MustParse(facA2)) {
		t.Fatalf("org admin must hold facility.read at both org A facilities, grants=%+v", actor.FacilityGrants)
	}
	if actor.HasPermissionAtFacility("facility.read", uuid.MustParse(facB1)) {
		t.Fatalf("org admin must NOT hold facility.read at org B, grants=%+v", actor.FacilityGrants)
	}

	h := scopedHandler(pool)
	rec := dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities", "", actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("list facilities status=%d want 200: %s", rec.Code, rec.Body.String())
	}
	got := extractFacilityIDsFromListBody(t, rec.Body.Bytes())
	if !containsString(got, facA1) || !containsString(got, facA2) {
		t.Fatalf("org admin must see both org A facilities, got %v", got)
	}
	if containsString(got, facB1) {
		t.Fatalf("org admin must NOT see org B facility, got %v", got)
	}
}

// TestOrgTenancy_OrgAdminListOrganizationsOnlyOwn proves ListOrganizations is
// scoped: an org admin sees only its own org; a regional admin sees both.
func TestOrgTenancy_OrgAdminListOrganizationsOnlyOwn(t *testing.T) {
	pool, cleanup, orgA, orgB, _, _, _ := orgTenancyFixture(t)
	defer cleanup()
	ctx := context.Background()

	orgAdminA := uuid.NewString()
	regional := uuid.NewString()
	if err := seedAppUser(ctx, pool, orgAdminA, "org-admin-a-list"); err != nil {
		t.Fatal(err)
	}
	if err := seedAppUser(ctx, pool, regional, "regional-list"); err != nil {
		t.Fatal(err)
	}
	seedOrgScopedRole(ctx, pool, orgAdminA, "organization_admin", orgA, orgAdminPermissions())
	seedUnscopedRole(ctx, pool, regional, "regional_admin", regionalAdminPermissions())

	h := scopedHandler(pool)

	recA := dispatchOrgRequest(h, http.MethodGet, "/api/v1/admin/organizations", dbScopedActor(t, pool, orgAdminA))
	if recA.Code != http.StatusOK {
		t.Fatalf("org admin list status=%d: %s", recA.Code, recA.Body.String())
	}
	orgsA := decodeOrganizationList(t, recA.Body.Bytes())
	if !containsString(orgsA, orgA) || containsString(orgsA, orgB) {
		t.Fatalf("org admin must see only org A, got %v", orgsA)
	}

	recR := dispatchOrgRequest(h, http.MethodGet, "/api/v1/admin/organizations", dbScopedActor(t, pool, regional))
	if recR.Code != http.StatusOK {
		t.Fatalf("regional list status=%d: %s", recR.Code, recR.Body.String())
	}
	orgsR := decodeOrganizationList(t, recR.Body.Bytes())
	if !containsString(orgsR, orgA) || !containsString(orgsR, orgB) {
		t.Fatalf("regional admin must see both orgs, got %v", orgsR)
	}
}

// TestOrgTenancy_ForgedOrganizationIDDenied proves a scoped org admin cannot
// read a sibling organization's overview (forged organization_id -> 404, no
// existence leak).
func TestOrgTenancy_ForgedOrganizationIDDenied(t *testing.T) {
	pool, cleanup, orgA, orgB, _, _, _ := orgTenancyFixture(t)
	defer cleanup()
	ctx := context.Background()

	orgAdminA := uuid.NewString()
	if err := seedAppUser(ctx, pool, orgAdminA, "org-admin-a-forge"); err != nil {
		t.Fatal(err)
	}
	seedOrgScopedRole(ctx, pool, orgAdminA, "organization_admin", orgA, orgAdminPermissions())

	h := scopedHandler(pool)
	actor := dbScopedActor(t, pool, orgAdminA)

	// Own org works.
	rec := dispatchOrgRequest(h, http.MethodGet, "/api/v1/admin/organizations/"+orgA, actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("own org overview status=%d want 200: %s", rec.Code, rec.Body.String())
	}

	// Forged sibling org id -> 404.
	recB := dispatchOrgRequest(h, http.MethodGet, "/api/v1/admin/organizations/"+orgB, actor)
	if recB.Code != http.StatusNotFound {
		t.Fatalf("forged org overview status=%d want 404: %s", recB.Code, recB.Body.String())
	}
}

// TestOrgTenancy_ForgedFacilityIDDenied proves an org-scoped admin cannot read
// a sibling facility (forged facility_id -> 404 on detail, absent on lists).
func TestOrgTenancy_ForgedFacilityIDDenied(t *testing.T) {
	pool, cleanup, orgA, _, _, _, facB1 := orgTenancyFixture(t)
	defer cleanup()
	ctx := context.Background()

	orgAdminA := uuid.NewString()
	if err := seedAppUser(ctx, pool, orgAdminA, "org-admin-a-fac-forge"); err != nil {
		t.Fatal(err)
	}
	seedOrgScopedRole(ctx, pool, orgAdminA, "organization_admin", orgA, orgAdminPermissions())

	h := scopedHandler(pool)
	actor := dbScopedActor(t, pool, orgAdminA)

	// Read of a sibling facility -> 404 (GetFacility does not leak existence).
	rec := dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities/"+facB1, "", actor)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sibling facility read status=%d want 404: %s", rec.Code, rec.Body.String())
	}

	// Mutation of a sibling facility -> 404 (UpdateFacility denies out-of-scope).
	mut := dispatchRequest(h, http.MethodPatch, "/api/v1/admin/facilities/"+facB1, `{"name":"hijack"}`, actor)
	if mut.Code != http.StatusNotFound {
		t.Fatalf("sibling facility mutation status=%d want 404: %s", mut.Code, mut.Body.String())
	}
}

// TestOrgTenancy_OperatorCrossOrgQueueIsolation proves a facility-scoped
// operator cannot enumerate a sibling org's queue, even with a forged
// facility_id query parameter.
func TestOrgTenancy_OperatorCrossOrgQueueIsolation(t *testing.T) {
	pool, cleanup, _, _, facA1, _, facB1 := orgTenancyFixture(t)
	defer cleanup()
	ctx := context.Background()

	opA := uuid.NewString()
	opB := uuid.NewString()
	if err := seedAppUser(ctx, pool, opA, "operator-a1"); err != nil {
		t.Fatal(err)
	}
	if err := seedAppUser(ctx, pool, opB, "operator-b1"); err != nil {
		t.Fatal(err)
	}
	seedNamedRoleAtFacility(t, pool, opA, "operator", facA1)
	seedNamedRoleAtFacility(t, pool, opB, "operator", facB1)

	seedQueueTicketForFacility(t, pool, facA1)
	seedQueueTicketForFacility(t, pool, facB1)

	h := scopedHandler(pool)

	actorA := dbScopedActor(t, pool, opA)
	recA := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues", "", actorA)
	if recA.Code != http.StatusOK {
		t.Fatalf("operator A queue list status=%d: %s", recA.Code, recA.Body.String())
	}
	if strings.Contains(recA.Body.String(), facB1) {
		t.Fatalf("operator A must not observe org B queue: %s", recA.Body.String())
	}

	actorB := dbScopedActor(t, pool, opB)
	recB := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues?facility_id="+facA1, "", actorB)
	if recB.Code != http.StatusOK {
		t.Fatalf("operator B forged queue list status=%d: %s", recB.Code, recB.Body.String())
	}
	if strings.Contains(recB.Body.String(), facA1) {
		// A forged facility_id for a facility the operator is not scoped to is a
		// no-op: ListQueueTickets filters solely by the resolved scope (readSet.IDs),
		// so opB must see their OWN facility's rows and never the forged org-A id.
		t.Fatalf("operator B leaked org A facility via forged facility_id: %s", recB.Body.String())
	}
}

// TestOrgTenancy_RegionalAdminReadOnly proves a regional admin has unrestricted
// operational READ scope but no manage/clinical capability: it can list every
// org but a mutation is denied because no .manage permission is granted.
func TestOrgTenancy_RegionalAdminReadOnly(t *testing.T) {
	pool, cleanup, _, _, facA1, facA2, facB1 := orgTenancyFixture(t)
	defer cleanup()
	ctx := context.Background()

	regional := uuid.NewString()
	if err := seedAppUser(ctx, pool, regional, "regional-ro"); err != nil {
		t.Fatal(err)
	}
	seedUnscopedRole(ctx, pool, regional, "regional_admin", regionalAdminPermissions())

	actor := dbScopedActor(t, pool, regional)
	if actor.HasPermission("queue.manage") || actor.HasPermission("facility.manage") {
		t.Fatalf("regional_admin must not hold manage permissions, got %v", actor.Permissions)
	}
	if actor.HasPermission("medical_record.read") {
		t.Fatalf("regional_admin must not hold medical_record.read")
	}
	// The no-.manage invariant includes organization.manage. Although no route
	// gates on that key today, the regional admin's unscoped grants are expanded
	// into per-facility grants for EVERY key — so if a future org-write handler
	// used it, this seed would authorize the change at every facility. Pin both
	// the flat and the per-facility form so the seed/test cannot drift back.
	if actor.HasPermission("organization.manage") {
		t.Fatalf("regional_admin must not hold organization.manage, got %v", actor.Permissions)
	}
	for _, f := range []string{facA1, facA2, facB1} {
		if actor.HasPermissionAtFacility("organization.manage", uuid.MustParse(f)) {
			t.Fatalf("regional_admin must not hold organization.manage at facility %s", f)
		}
	}

	h := scopedHandler(pool)
	// Mutation denied (no facility.manage provenance despite unrestricted scope).
	rec := dispatchRequest(h, http.MethodPatch, "/api/v1/admin/facilities/"+facA1, `{"name":"ro-hijack"}`, actor)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("regional admin mutation status=%d want 404: %s", rec.Code, rec.Body.String())
	}
}

// TestOrgTenancy_AuditCarriesActorAndScope proves organization access audits
// record the correct actor and scope (resource_id = organization_id, actor =
// the org admin app user) without carrying PHI.
func TestOrgTenancy_AuditCarriesActorAndScope(t *testing.T) {
	pool, cleanup, orgA, _, _, _, _ := orgTenancyFixture(t)
	defer cleanup()
	ctx := context.Background()

	orgAdminA := uuid.NewString()
	if err := seedAppUser(ctx, pool, orgAdminA, "org-admin-a-audit"); err != nil {
		t.Fatal(err)
	}
	seedOrgScopedRole(ctx, pool, orgAdminA, "organization_admin", orgA, orgAdminPermissions())

	h := NewAdminHandler(pool).
		WithAudit(audit.NewService(pool)).
		WithFacilityScopeResolver(auth.NewDBFacilityScope(pool))

	actor := dbScopedActor(t, pool, orgAdminA)
	rec := dispatchOrgRequest(h, http.MethodGet, "/api/v1/admin/organizations/"+orgA, actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("org overview status=%d: %s", rec.Code, rec.Body.String())
	}

	var (
		actorUserID string
		action      string
		resourceID  string
		metadata    map[string]any
	)
	if err := pool.QueryRow(ctx,
		`SELECT actor_user_id::text, action, resource_id, metadata
		 FROM audit_events
		 WHERE action = 'organization.get'
		 ORDER BY occurred_at DESC
		 LIMIT 1`).Scan(&actorUserID, &action, &resourceID, &metadata); err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	if actorUserID != orgAdminA {
		t.Fatalf("audit actor_user_id=%q want %q", actorUserID, orgAdminA)
	}
	if resourceID != orgA {
		t.Fatalf("audit resource_id=%q want org id %q", resourceID, orgA)
	}
	if got, ok := metadata["organization_id"].(string); !ok || got != orgA {
		t.Fatalf("audit metadata organization_id=%v want %v (full metadata=%v)", metadata["organization_id"], orgA, metadata)
	}
	if _, hasPHI := metadata["patient"]; hasPHI {
		t.Fatalf("audit metadata must not carry PHI: %v", metadata)
	}
}