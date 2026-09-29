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
	"github.com/sigap/sigap/apps/api/internal/notification"
)

// ---------------------------------------------------------------------------
// Phase 3B5.0 — Notification actionability: per-row affordance projection
// ---------------------------------------------------------------------------
//
// THE TWO AXES
//
// Each row's can_retry / can_cancel is the AND of two independent questions:
//
//	STATUS:    is the row in a state where the mutation would even be accepted?
//	PROVENANCE: does the actor hold notification.manage AT THIS ROW'S OWN
//	            STORED FACILITY?
//
// Both must be true. The failure mode this file exists to prevent is the one
// where only the first is checked — where an actor who manages facility B is
// shown live action buttons on facility-A rows that the server will refuse with
// 404. The UI would then tell the operator to perform an action that cannot
// succeed.
//
// Note what is NOT an input: the actor's FLAT permission set. An actor holding
// `notification.manage` in their JWT-adjacent union is managing *somewhere*;
// that "somewhere" is exactly the information a flat set throws away.
// ---------------------------------------------------------------------------

// notificationListRow is the decoded wire shape for one listed row.
//
// The affordance booleans are declared HERE, at the top level, which is itself
// part of the contract: they must be siblings of `status` and `facility_id`,
// not nested under an "actionability" object. encoding/json only produces that
// flat shape for an ANONYMOUSLY embedded struct, so this decode is what pins
// the embedding decision in notificationListItem.
type notificationListRow struct {
	ID         string  `json:"id"`
	FacilityID *string `json:"facility_id"`
	Status     string  `json:"status"`
	CanRetry   bool    `json:"can_retry"`
	CanCancel  bool    `json:"can_cancel"`
}

func decodeNotificationList(t *testing.T, rec *httptest.ResponseRecorder) []notificationListRow {
	t.Helper()
	var response struct {
		Data []notificationListRow `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode notification list: %v (body=%s)", err, rec.Body.String())
	}
	return response.Data
}

func findNotificationRow(t *testing.T, rows []notificationListRow, id string) notificationListRow {
	t.Helper()
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("notification %s not present in list (got %d rows)", id, len(rows))
	return notificationListRow{}
}

func callNotificationList(t *testing.T, h *NotificationsHandler, actor identity.Actor) *httptest.ResponseRecorder {
	t.Helper()
	return notificationRequest(t, h, http.MethodGet, "/api/v1/admin/notifications", actor)
}

// assertNoAffordanceMetadataLeak proves the projection added two booleans and
// nothing else. It inspects the raw keys of a single row, which is the only
// place an accidental permission leak would surface.
func assertNoAffordanceMetadataLeak(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var response struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode rows as maps: %v", err)
	}
	if len(response.Data) == 0 {
		t.Fatal("expected at least one row to inspect")
	}
	allowed := map[string]bool{
		"id": true, "facility_id": true, "channel": true, "template_key": true,
		"subject": true, "body_template": true, "recipient_type": true,
		"recipient_contact_masked": true, "status": true, "attempt_count": true,
		"next_attempt_at": true, "last_error_code": true,
		"created_at": true, "updated_at": true,
		"can_retry": true, "can_cancel": true,
	}
	for i, row := range response.Data {
		for key := range row {
			if !allowed[key] {
				t.Errorf("row %d exposes unexpected key %q; actionability must add only can_retry/can_cancel", i, key)
			}
		}
	}
	// Belt and braces: a substring sweep for the spec's §2 denial set.
	lower := strings.ToLower(rec.Body.String())
	for _, bad := range []string{"role", "permission", "grant", "unrestricted", "super_admin", "facility_scope", "user_id"} {
		if strings.Contains(lower, `"`+bad) {
			t.Errorf("notification list leaks forbidden key %q", bad)
		}
	}
}

// seedGlobalReaderActor creates an app user holding a GENUINELY global grant of
// the given permission keys — but never notification.manage.
//
// The "super_admin" role name is load-bearing and not incidental. Global reach
// in this codebase is gated on the role name in two independent places
// (rbac_resolver.go and facility_scope.go both test `r.name = 'super_admin'`),
// so a synthetic role assigned with facility_id IS NULL has NO scope at all
// and can read nothing. Writing the obvious-looking seed and then concluding
// "global scope grants no actionability" would be a false pass: the empty
// result would be caused by an empty SCOPE, not by missing provenance. This
// helper therefore produces an actor that can genuinely see the rows, so the
// assertions that follow are about actionability rather than visibility.
func seedGlobalReaderActor(t *testing.T, pool *pgxpool.Pool, subject string, perms []string) identity.Actor {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, subject); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, "", perms); err != nil {
		t.Fatalf("seed global role: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE roles SET name = 'super_admin'
		 WHERE id = (SELECT role_id FROM user_roles
		             WHERE user_id = $1 AND facility_id IS NULL
		             ORDER BY role_id LIMIT 1)`, userID); err != nil {
		t.Fatalf("promote seeded role to super_admin: %v", err)
	}
	actor := dbScopedActor(t, pool, userID)
	// Assert the premise the callers depend on: real global reach, and no
	// manage key. Without both, a downstream "not actionable" assertion would
	// be unfalsifiable.
	if !actor.HasPermission(perms[0]) {
		t.Fatalf("precondition failed: global reader lacks %s: %+v", perms[0], actor.Permissions)
	}
	var unrestricted bool
	for _, g := range actor.FacilityGrants {
		if g.Unrestricted {
			unrestricted = true
		}
	}
	if !unrestricted {
		t.Fatalf("precondition failed: global reader has no unrestricted grant: %+v", actor.FacilityGrants)
	}
	if actor.HasPermission("notification.manage") {
		t.Fatalf("precondition failed: global reader must not hold notification.manage")
	}
	return actor
}

// ---------------------------------------------------------------------------
// N1 — notification.read at A + notification.manage only at B
//       A rows: can_retry=false, can_cancel=false
//       eligible B rows: true
// ---------------------------------------------------------------------------
func TestNotificationActionability_N1_MixedGrantsUseStoredRowFacility(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facA, "Notif Action A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Notif Action B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}

	// Both rows are in a status that WOULD allow retry and cancel. If the
	// projection checked status alone, every row would be true and this test
	// would catch it.
	rowA := seedScopeNotification(t, pool, &facA, notification.StatusFailed, 2)
	rowB := seedScopeNotification(t, pool, &facB, notification.StatusFailed, 1)
	rowBPending := seedScopeNotification(t, pool, &facB, notification.StatusPending, 1)

	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "notif-mixed-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	// A: read only. B: read AND manage.
	if err := seedUserRoles(ctx, pool, userID, facA, []string{"notification.read"}); err != nil {
		t.Fatalf("seed roles A: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facB, []string{"notification.read", "notification.manage"}); err != nil {
		t.Fatalf("seed roles B: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)
	if !actor.HasPermission("notification.manage") {
		t.Fatalf("precondition failed: actor must hold the flat manage key")
	}
	if actor.HasPermissionAtFacility("notification.manage", mustParseUUID(t, facA)) {
		t.Fatalf("precondition failed: actor must NOT manage facility A")
	}

	rec := callNotificationList(t, scopedNotificationsHandler(pool), actor)
	assertNotificationStatus(t, rec, http.StatusOK)
	rows := decodeNotificationList(t, rec)

	// A: status permits it, provenance does not. Both booleans must be false.
	gotA := findNotificationRow(t, rows, rowA)
	if gotA.CanRetry || gotA.CanCancel {
		t.Errorf("facility-A row (status=%s) must not be actionable; got can_retry=%v can_cancel=%v",
			gotA.Status, gotA.CanRetry, gotA.CanCancel)
	}

	// B failed: both actions permitted by status and provenance.
	gotB := findNotificationRow(t, rows, rowB)
	if !gotB.CanRetry || !gotB.CanCancel {
		t.Errorf("eligible facility-B failed row must be actionable; got can_retry=%v can_cancel=%v",
			gotB.CanRetry, gotB.CanCancel)
	}

	// B pending: retry and cancel both permitted.
	gotBP := findNotificationRow(t, rows, rowBPending)
	if !gotBP.CanRetry || !gotBP.CanCancel {
		t.Errorf("eligible facility-B pending row must be actionable; got can_retry=%v can_cancel=%v",
			gotBP.CanRetry, gotBP.CanCancel)
	}

	assertNoAffordanceMetadataLeak(t, rec)
}

// ---------------------------------------------------------------------------
// N2 — status-ineligible row at a MANAGED facility → false despite permission
// ---------------------------------------------------------------------------
func TestNotificationActionability_N2_StatusRulesGateTheAffordance(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Notif Status Rules"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "notif-status-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facID, []string{"notification.read", "notification.manage"}); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	// One row per status, all at a facility the actor fully manages.
	rows := map[string]string{}
	for _, status := range notification.AllStatuses() {
		rows[status] = seedScopeNotification(t, pool, &facID, notification.Status(status), 1)
	}

	actor := dbScopedActor(t, pool, userID)
	if !actor.HasPermissionAtFacility("notification.manage", mustParseUUID(t, facID)) {
		t.Fatalf("precondition failed: actor must manage the facility")
	}

	rec := callNotificationList(t, scopedNotificationsHandler(pool), actor)
	assertNotificationStatus(t, rec, http.StatusOK)
	listed := decodeNotificationList(t, rec)

	// Expected from the CURRENT service contract:
	//   retry  allowed on failed, pending
	//   cancel allowed on pending, failed, cancelled (idempotent)
	wantRetry := map[string]bool{
		string(notification.StatusPending):   true,
		string(notification.StatusFailed):    true,
		string(notification.StatusProcessing): false,
		string(notification.StatusDelivered):  false,
		string(notification.StatusCancelled):  false,
	}
	wantCancel := map[string]bool{
		string(notification.StatusPending):   true,
		string(notification.StatusFailed):    true,
		string(notification.StatusCancelled): true,
		string(notification.StatusProcessing): false,
		string(notification.StatusDelivered):  false,
	}

	for status, id := range rows {
		row := findNotificationRow(t, listed, id)
		if row.CanRetry != wantRetry[status] {
			t.Errorf("status=%s can_retry=%v want %v", status, row.CanRetry, wantRetry[status])
		}
		if row.CanCancel != wantCancel[status] {
			t.Errorf("status=%s can_cancel=%v want %v", status, row.CanCancel, wantCancel[status])
		}
	}
}

// ---------------------------------------------------------------------------
// N2b — the affordance and the MUTATION must agree, status for status
// ---------------------------------------------------------------------------
//
// The point of the projection is that it never promises an action the server
// will reject. So for every status, `can_retry` must be true if and only if a
// real retry call does NOT return 409. This is the anti-drift test: if someone
// later adds a status to the Go predicate but forgets the SQL (or vice versa),
// this fails.
func TestNotificationActionability_N2b_AffordanceMatchesRealMutationOutcome(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Notif Anti Drift"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "notif-drift-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facID, []string{"notification.read", "notification.manage"}); err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	ids := map[string]string{}
	for _, status := range notification.AllStatuses() {
		ids[status] = seedScopeNotification(t, pool, &facID, notification.Status(status), 1)
	}

	actor := dbScopedActor(t, pool, userID)
	h := scopedNotificationsHandler(pool)

	// Capture the affordances BEFORE any mutation perturbs the statuses.
	rec := callNotificationList(t, h, actor)
	assertNotificationStatus(t, rec, http.StatusOK)
	affordance := decodeNotificationList(t, rec)

	// Now actually attempt the mutations on a FRESH set of rows (the ones above
	// are still untouched — the list call does not mutate).
	for status, id := range ids {
		row := findNotificationRow(t, affordance, id)

		retryRec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+id+"/retry", actor)
		retryAccepted := retryRec.Code == http.StatusOK
		if retryAccepted != row.CanRetry {
			t.Errorf("status=%s: can_retry=%v but a real retry returned %d (accepted=%v) — the affordance has drifted from the mutation",
				status, row.CanRetry, retryRec.Code, retryAccepted)
		}

		cancelRec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+id+"/cancel", actor)
		cancelAccepted := cancelRec.Code == http.StatusOK
		if cancelAccepted != row.CanCancel {
			t.Errorf("status=%s: can_cancel=%v but a real cancel returned %d (accepted=%v) — the affordance has drifted from the mutation",
				status, row.CanCancel, cancelRec.Code, cancelAccepted)
		}
	}
}

// ---------------------------------------------------------------------------
// N3 — zero facility assignment → no actionable row
// ---------------------------------------------------------------------------
func TestNotificationActionability_N3_ZeroAssignmentHasNoActionableRow(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Notif Zero"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	rowID := seedScopeNotification(t, pool, &facID, notification.StatusFailed, 1)

	// A zero assignment: flat keys present, NO provenance. Exactly the shape
	// that a flat-permission implementation would wave through.
	zeroID := uuid.NewString()
	if err := seedAppUser(ctx, pool, zeroID, "notif-zero-actor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	zeroActor := makeScopedActor(zeroID, "notification.read", "notification.manage")
	if !zeroActor.HasPermission("notification.manage") {
		t.Fatalf("precondition failed: the flat key must be present")
	}

	rec := callNotificationList(t, scopedNotificationsHandler(pool), zeroActor)
	assertNotificationStatus(t, rec, http.StatusOK)

	// A zero-assignment actor cannot even READ the row, so the list is empty.
	// The assertion that matters: nothing anywhere is actionable.
	for _, row := range decodeNotificationList(t, rec) {
		if row.CanRetry || row.CanCancel {
			t.Errorf("a zero-assignment actor must see no actionable row, got %+v", row)
		}
	}
	_ = rowID

	// Additionally: a globally-scoped READER (no manage) genuinely sees the
	// row, because the row is readable, but must see it as inert.
	reader := seedGlobalReaderActor(t, pool, "notif-global-reader", []string{"notification.read"})
	rec = callNotificationList(t, scopedNotificationsHandler(pool), reader)
	assertNotificationStatus(t, rec, http.StatusOK)
	rows := decodeNotificationList(t, rec)
	if len(rows) == 0 {
		t.Fatalf("precondition failed: a global reader must be able to read the row")
	}
	var sawRow bool
	for _, row := range rows {
		if row.ID == rowID {
			sawRow = true
		}
		if row.CanRetry || row.CanCancel {
			t.Errorf("a read-only actor must see no actionable row, got id=%s can_retry=%v can_cancel=%v",
				row.ID, row.CanRetry, row.CanCancel)
		}
	}
	if !sawRow {
		t.Fatalf("precondition failed: the seeded row %s was not listed for the global reader", rowID)
	}
}

// ---------------------------------------------------------------------------
// N4 — inactive / soft-deleted grant → no actionability
// ---------------------------------------------------------------------------
func TestNotificationActionability_N4_InactiveAndDeletedGrantsAreNotActionable(t *testing.T) {
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

			facID := uuid.NewString()
			if err := seedFacilityByName(ctx, pool, facID, "Notif Lifecycle"); err != nil {
				t.Fatalf("seed facility: %v", err)
			}
			rowID := seedScopeNotification(t, pool, &facID, notification.StatusFailed, 1)

			userID := uuid.NewString()
			if err := seedAppUser(ctx, pool, userID, "notif-lifecycle-"+uuid.NewString()[:8]); err != nil {
				t.Fatalf("seed app user: %v", err)
			}
			if err := seedUserRoles(ctx, pool, userID, facID, []string{"notification.read", "notification.manage"}); err != nil {
				t.Fatalf("seed roles: %v", err)
			}
			if err := setUserRoleLifecycle(ctx, pool, userID, facID, tc.status, tc.deleted); err != nil {
				t.Fatalf("set role lifecycle: %v", err)
			}

			actor := dbScopedActor(t, pool, userID)
			rec := callNotificationList(t, scopedNotificationsHandler(pool), actor)
			assertNotificationStatus(t, rec, http.StatusOK)

			// Whether the row is listed at all or not, it must never be
			// actionable: a revoked grant takes effect on the next request.
			for _, row := range decodeNotificationList(t, rec) {
				if row.ID == rowID && (row.CanRetry || row.CanCancel) {
					t.Errorf("a %s grant must not be actionable, got can_retry=%v can_cancel=%v",
						tc.name, row.CanRetry, row.CanCancel)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// N5 — JWT/client claims cannot fabricate actionability
// ---------------------------------------------------------------------------
func TestNotificationActionability_N5_ClaimsCannotFabricateActionability(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Notif Claims"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	rowID := seedScopeNotification(t, pool, &facID, notification.StatusFailed, 2)

	forged := identity.Actor{
		Type:           identity.ActorUser,
		UserID:         "sub:forged-not-a-real-notification-session",
		Permissions:    []string{"notification.read", "notification.manage"},
		FacilityGrants: nil,
		AppUserID:      uuid.NewString(),
		IsDev:          false,
	}
	if !forged.HasPermission("notification.manage") {
		t.Fatalf("precondition failed: the forged flat key must be present")
	}

	rec := callNotificationList(t, scopedNotificationsHandler(pool), forged)
	assertNotificationStatus(t, rec, http.StatusOK)
	for _, row := range decodeNotificationList(t, rec) {
		if row.ID == rowID && (row.CanRetry || row.CanCancel) {
			t.Errorf("claim-only actor must not receive actionable affordances")
		}
	}

	// And the decisive proof: the forged identity cannot actually mutate,
	// so a "true" affordance would have been a lie.
	retryRec := notificationRequest(t, scopedNotificationsHandler(pool), http.MethodPost,
		"/api/v1/admin/notifications/"+rowID+"/retry", forged)
	if retryRec.Code == http.StatusOK {
		t.Errorf("precondition broken: a claim-only actor must not be able to retry")
	}
}

// ---------------------------------------------------------------------------
// Global super_admin: still requires the ACTUAL notification.manage permission
// ---------------------------------------------------------------------------
func TestNotificationActionability_GlobalScopeAloneGrantsNoActionability(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Notif Global Reader"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	rowID := seedScopeNotification(t, pool, &facID, notification.StatusFailed, 1)

	// A GLOBAL assignment carrying read but NOT manage. Scope says where; the
	// permission says what. This actor sees the row and can do nothing with it.
	actor := seedGlobalReaderActor(t, pool, "notif-global-read-only", []string{"notification.read"})

	rec := callNotificationList(t, scopedNotificationsHandler(pool), actor)
	assertNotificationStatus(t, rec, http.StatusOK)
	rows := decodeNotificationList(t, rec)
	if len(rows) == 0 {
		t.Fatalf("precondition failed: a global reader must be able to read the row")
	}
	var sawRow bool
	for _, row := range rows {
		if row.ID == rowID {
			sawRow = true
			if row.CanRetry || row.CanCancel {
				t.Errorf("global scope must not imply notification.manage (status=%s, can_retry=%v, can_cancel=%v)",
					row.Status, row.CanRetry, row.CanCancel)
			}
		}
	}
	if !sawRow {
		t.Fatalf("precondition failed: the seeded row %s was not listed; without it this test would pass vacuously", rowID)
	}
}

// A real global super_admin (which the seed gives the full permission set) IS
// actionable, proving the projection is not simply broken for globals.
func TestNotificationActionability_GlobalSuperAdminWithManageIsActionable(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Notif Global Admin"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	rowFailed := seedScopeNotification(t, pool, &facID, notification.StatusFailed, 1)
	rowDelivered := seedScopeNotification(t, pool, &facID, notification.StatusDelivered, 1)

	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "notif-global-admin"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, userID); err != nil {
		t.Fatalf("seed global super admin: %v", err)
	}

	actor := dbScopedActor(t, pool, userID)
	rec := callNotificationList(t, scopedNotificationsHandler(pool), actor)
	assertNotificationStatus(t, rec, http.StatusOK)
	rows := decodeNotificationList(t, rec)

	failed := findNotificationRow(t, rows, rowFailed)
	if !failed.CanRetry || !failed.CanCancel {
		t.Errorf("a global super_admin with the manage permission must see actionable rows; got can_retry=%v can_cancel=%v",
			failed.CanRetry, failed.CanCancel)
	}
	delivered := findNotificationRow(t, rows, rowDelivered)
	if delivered.CanRetry || delivered.CanCancel {
		t.Errorf("a delivered row is never actionable regardless of permission; got can_retry=%v can_cancel=%v",
			delivered.CanRetry, delivered.CanCancel)
	}
}

// ---------------------------------------------------------------------------
// Resolver failure → no actionability (fail closed)
// ---------------------------------------------------------------------------
func TestNotificationActionability_ResolverFailureGrantsNoActionability(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Notif Resolver Fail"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	rowID := seedScopeNotification(t, pool, &facID, notification.StatusFailed, 1)

	userID := uuid.NewString()
	if err := seedAppUser(ctx, pool, userID, "notif-resolver-fail"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userID, facID, []string{"notification.read", "notification.manage"}); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	actor := dbScopedActor(t, pool, userID)

	// Dead grant resolver: every re-resolution errors.
	h := NewNotificationsHandler(notification.NewService(pool)).
		WithFacilityScopeResolver(auth.NewDBFacilityScope(pool)).
		WithFacilityGrantResolver(auth.NewDBFacilityGrantResolver(nil))

	rec := callNotificationList(t, h, actor)
	assertNotificationStatus(t, rec, http.StatusOK)
	for _, row := range decodeNotificationList(t, rec) {
		if row.ID == rowID && (row.CanRetry || row.CanCancel) {
			t.Errorf("a resolver failure must fail closed to no actionability")
		}
	}
}

// ---------------------------------------------------------------------------
// A nil stored facility is actionable only under an UNRESTRICTED manage grant
// ---------------------------------------------------------------------------
func TestNotificationActionability_NilFacilityRequiresUnrestrictedManage(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	// A notification that belongs to no facility.
	facID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facID, "Notif Nil Facility"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	globalRow := seedScopeNotification(t, pool, nil, notification.StatusFailed, 1)
	scopedRow := seedScopeNotification(t, pool, &facID, notification.StatusFailed, 1)

	// (a) An actor with an UNRESTRICTED manage grant may act on the global row.
	globalID := uuid.NewString()
	if err := seedAppUser(ctx, pool, globalID, "notif-nil-global"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, globalID); err != nil {
		t.Fatalf("seed global role: %v", err)
	}
	globalActor := dbScopedActor(t, pool, globalID)

	rec := callNotificationList(t, scopedNotificationsHandler(pool), globalActor)
	assertNotificationStatus(t, rec, http.StatusOK)
	row := findNotificationRow(t, decodeNotificationList(t, rec), globalRow)
	if !row.CanRetry || !row.CanCancel {
		t.Errorf("an unrestricted manage grant must cover a facility-less row; got can_retry=%v can_cancel=%v",
			row.CanRetry, row.CanCancel)
	}

	// (b) An actor with a FACILITY-SCOPED manage grant may NOT act on it.
	scopedID := uuid.NewString()
	if err := seedAppUser(ctx, pool, scopedID, "notif-nil-scoped"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, scopedID, facID, []string{"notification.read", "notification.manage"}); err != nil {
		t.Fatalf("seed scoped role: %v", err)
	}
	scopedActor := dbScopedActor(t, pool, scopedID)

	rec = callNotificationList(t, scopedNotificationsHandler(pool), scopedActor)
	assertNotificationStatus(t, rec, http.StatusOK)
	for _, r := range decodeNotificationList(t, rec) {
		if r.ID == globalRow && (r.CanRetry || r.CanCancel) {
			t.Errorf("a facility-scoped manage grant must not cover a facility-less row")
		}
		if r.ID == scopedRow && (!r.CanRetry || !r.CanCancel) {
			t.Errorf("the facility-scoped row itself must remain actionable")
		}
	}
}

// ---------------------------------------------------------------------------
// The shared predicate is PURE and total over every known status
// ---------------------------------------------------------------------------
//
// If a new status constant is ever added, this fails until the affordance
// rules are updated to say what it means. That is the whole point of routing
// both the affordance and (via N2b) the mutation through one declaration.
func TestNotificationActionability_PredicateIsTotalOverEveryStatus(t *testing.T) {
	for _, status := range notification.AllStatuses() {
		got := notification.ActionabilityForStatus(string(status))
		if got.CanRetry != notification.CanRetryStatus(string(status)) {
			t.Errorf("status=%s inconsistent retry affordance", status)
		}
		if got.CanCancel != notification.CanCancelStatus(string(status)) {
			t.Errorf("status=%s inconsistent cancel affordance", status)
		}
		// delivered is terminal: neither action may ever be offered.
		if status == string(notification.StatusDelivered) && (got.CanRetry || got.CanCancel) {
			t.Errorf("delivered must be inert, got %+v", got)
		}
	}
	// An unknown status must be inert, never default-true.
	unknown := notification.ActionabilityForStatus("some_future_status")
	if unknown.CanRetry || unknown.CanCancel {
		t.Errorf("an unknown status must fail closed, got %+v", unknown)
	}
}
