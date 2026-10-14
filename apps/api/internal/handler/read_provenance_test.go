package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/notification"
)

// ---------------------------------------------------------------------------
// Phase 3B0.2 read-provenance regression matrix (R1-R9).
//
// Every test in this file proves one property of the canonical rule:
//
//	A facility-scoped permission grant applies ONLY to the facility attached to
//	that grant, therefore permission-at-B MUST NOT authorize read-at-A.
//
// All tests are DB-backed and take BOTH the flat permission set and the
// facility provenance from the real resolver, so a test can never pass because
// of a synthetic actor. They use only permission keys that exist in
// packages/db/seed/rbac.sql.
// ---------------------------------------------------------------------------

// readFixture is a two-facility (A, B) world with one row per resource type at
// each facility, so a test can assert that A rows are hidden while B rows
// remain visible for the same actor.
type readFixture struct {
	pool      *pgxpool.Pool
	facilityA string
	facilityB string

	queueA       string
	queueB       string
	unitA        string
	unitB        string
	scheduleA    string
	scheduleB    string
	notifA       string
	notifB       string
	appointmentA string
	appointmentB string
}

func newReadFixture(t *testing.T) *readFixture {
	t.Helper()
	pool, cleanup := newScopeTestPool(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	f := &readFixture{
		pool:      pool,
		facilityA: uuid.NewString(),
		facilityB: uuid.NewString(),
	}
	if err := seedFacilityByName(ctx, pool, f.facilityA, "Read A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, f.facilityB, "Read B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}

	f.queueA = seedQueueTicketForFacility(t, pool, f.facilityA)
	f.queueB = seedQueueTicketForFacility(t, pool, f.facilityB)
	f.unitA = seedReadServiceUnit(t, pool, f.facilityA, "Read Unit A", "RUA")
	f.unitB = seedReadServiceUnit(t, pool, f.facilityB, "Read Unit B", "RUB")
	f.scheduleA = seedReadSchedule(t, pool, f.facilityA, f.unitA)
	f.scheduleB = seedReadSchedule(t, pool, f.facilityB, f.unitB)
	f.notifA = seedScopeNotification(t, pool, &f.facilityA, "pending", 0)
	f.notifB = seedScopeNotification(t, pool, &f.facilityB, "pending", 0)
	return f
}

// dispatchSchedulesRequest dispatches to the schedules router, mirroring the
// dedicated queue/service-unit/appointment dispatchers. The generic
// dispatchRequest helper tries the facilities router first and therefore
// mis-routes non-facility paths.
func dispatchSchedulesRequest(h *AdminHandler, method, path, body string, actor identity.Actor) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.SchedulesRouter(rec, req)
	return rec
}

// seedReadActor creates an app user whose grants are exactly
// {facility: permissions}. Different permission sets at different facilities are
// schema-valid because user_roles is keyed (user_id, role_id), so this is the
// realistic shape that produces a flat permission union.
func seedReadActor(t *testing.T, pool *pgxpool.Pool, subject string, grants map[string][]string) identity.Actor {
	t.Helper()
	ctx := context.Background()
	appUserID := uuid.NewString()
	if err := seedAppUser(ctx, pool, appUserID, subject); err != nil {
		t.Fatalf("seed app user %s: %v", subject, err)
	}
	for facilityID, perms := range grants {
		if err := seedUserRoles(ctx, pool, appUserID, facilityID, perms); err != nil {
			t.Fatalf("seed role at %s: %v", facilityID, err)
		}
	}
	return dbScopedActor(t, pool, appUserID)
}

func seedReadServiceUnit(t *testing.T, pool *pgxpool.Pool, facilityID, name, code string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
		 VALUES ($1, $2, $3, $4, '', true)`, id, facilityID, name, code); err != nil {
		t.Fatalf("seed service unit: %v", err)
	}
	return id
}

func seedReadSchedule(t *testing.T, pool *pgxpool.Pool, facilityID, unitID string) string {
	t.Helper()
	ctx := context.Background()
	practitionerID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO practitioners (id, facility_id, display_name, role, is_active)
		 VALUES ($1, $2, 'Read Practitioner', 'doctor', true)`, practitionerID, facilityID); err != nil {
		t.Fatalf("seed practitioner: %v", err)
	}
	id := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO practitioner_schedules
		   (id, facility_id, practitioner_id, service_unit_id, schedule_date,
		    start_time, end_time, slot_minutes, capacity_per_slot, is_active)
		 VALUES ($1, $2, $3, $4, '2026-03-02', '08:00', '12:00', 15, 10, true)`,
		id, facilityID, practitionerID, unitID); err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	return id
}

// seedReadAppointment inserts one appointment for a facility. The appointments
// table (migrations/0005_appointments.sql) stores no patient_id: it denormalizes
// patient_display_name, patient_phone, and checkin_code instead.
func seedReadAppointment(t *testing.T, pool *pgxpool.Pool, facilityID, unitID string) string {
	t.Helper()
	ctx := context.Background()
	id := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO appointments
		   (id, facility_id, service_unit_id, appointment_time, status,
		    patient_display_name, patient_phone, checkin_code)
		 VALUES ($1, $2, $3, '2026-03-02 09:00:00+00', 'scheduled',
		         'Read Patient', '085512345678', $4)`,
		id, facilityID, unitID, uuid.NewString()[:8]); err != nil {
		t.Fatalf("seed appointment: %v", err)
	}
	return id
}

// --- assertions -----------------------------------------------------------

func listRowIDs(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got status %d want 200, body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list %q: %v", rec.Body.String(), err)
	}
	ids := make([]string, 0, len(payload.Data))
	for _, row := range payload.Data {
		ids = append(ids, row.ID)
	}
	return ids
}

func listHas(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func notificationFacilityByID(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("notifications: got status %d want 200, body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    []struct {
			ID         string  `json:"id"`
			FacilityID *string `json:"facility_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode notifications %q: %v", rec.Body.String(), err)
	}
	out := make(map[string]string, len(payload.Data))
	for _, row := range payload.Data {
		facility := ""
		if row.FacilityID != nil {
			facility = *row.FacilityID
		}
		out[row.ID] = facility
	}
	return out
}

func summaryCounts(t *testing.T, rec *httptest.ResponseRecorder) map[string]int {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("summary: got status %d want 200, body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]int `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode summary %q: %v", rec.Body.String(), err)
	}
	return payload.Data
}

// assertZeroSummary checks every declared status is present and zero, which is
// the required fail-closed shape for a zero-assignment non-super-admin.
func assertZeroSummary(t *testing.T, counts map[string]int) {
	t.Helper()
	for _, status := range notification.AllStatuses() {
		got, ok := counts[status]
		if !ok {
			t.Errorf("summary missing status %q; a zero summary must list every declared status", status)
			continue
		}
		if got != 0 {
			t.Errorf("summary status %q = %d, want 0", status, got)
		}
	}
}

// assertPreconditionFlatUnion proves the actor's FLAT permission set contains
// the key. Without this, a test could pass simply because the route-level gate
// denied the request, proving nothing about provenance.
func assertPreconditionFlatUnion(t *testing.T, actor identity.Actor, permission string) {
	t.Helper()
	if !actor.HasPermission(permission) {
		t.Fatalf("precondition failed: flat %s missing, so the route gate would deny and the test proves nothing", permission)
	}
}

// assertPreconditionScope proves the actor's SCOPE contains the facility, so a
// hidden row can only be explained by missing permission provenance and never by
// the facility being out of scope.
func assertPreconditionScope(t *testing.T, pool *pgxpool.Pool, actor identity.Actor, facilityID string) {
	t.Helper()
	scope := auth.FacilityScopeForActor(context.Background(), actor, auth.NewDBFacilityScope(pool))
	if !auth.FacilityScopeResultAllows(scope, uuid.MustParse(facilityID)) {
		t.Fatalf("precondition failed: facility %s is not in the actor's scope", facilityID)
	}
}

// assertNoProvenance proves the actor has no facility-scoped grant at that
// facility for the key, which is the exact condition the read gate must catch.
func assertNoProvenance(t *testing.T, actor identity.Actor, key, facilityID string) {
	t.Helper()
	if actor.HasPermissionAtFacility(key, uuid.MustParse(facilityID)) {
		t.Fatalf("precondition failed: unexpected %s provenance at %s", key, facilityID)
	}
}

// ---------------------------------------------------------------------------
// R1 — notification.read provenance at B only must not expose A
// ---------------------------------------------------------------------------

func TestReadProvenance_R1_NotificationListScopedToProvenance(t *testing.T) {
	f := newReadFixture(t)

	// Scoped to BOTH facilities, but notification.read is granted at B ONLY.
	actor := seedReadActor(t, f.pool, "r1-notifications", map[string][]string{
		f.facilityA: {"queue.read"},
		f.facilityB: {"notification.read"},
	})
	assertPreconditionFlatUnion(t, actor, "notification.read")
	assertPreconditionScope(t, f.pool, actor, f.facilityA)
	assertNoProvenance(t, actor, "notification.read", f.facilityA)

	rec := notificationRequest(t, scopedNotificationsHandler(f.pool), http.MethodGet,
		"/api/v1/admin/notifications", actor)
	rows := notificationFacilityByID(t, rec)

	if facility, leaked := rows[f.notifA]; leaked {
		t.Errorf("R1 LEAK: notification %s at facility %s is visible; notification.read provenance exists only at %s",
			f.notifA, facility, f.facilityB)
	}
	if !listHas(mapKeys(rows), f.notifB) {
		t.Errorf("R1 vacuous: authorized notification %s at B is missing from the list", f.notifB)
	}
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---------------------------------------------------------------------------
// R2 — queue.read provenance at B only: B readable, A not
// ---------------------------------------------------------------------------

func TestReadProvenance_R2_QueueListAndDetailScopedToProvenance(t *testing.T) {
	f := newReadFixture(t)

	actor := seedReadActor(t, f.pool, "r2-queue", map[string][]string{
		f.facilityA: {"notification.read"},
		f.facilityB: {"queue.read"},
	})
	assertPreconditionFlatUnion(t, actor, "queue.read")
	assertPreconditionScope(t, f.pool, actor, f.facilityA)
	assertNoProvenance(t, actor, "queue.read", f.facilityA)

	h := scopedHandler(f.pool)

	listRec := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues", "", actor)
	ids := listRowIDs(t, listRec)
	if listHas(ids, f.queueA) {
		t.Errorf("R2 LEAK: queue ticket %s at A appears in the list; queue.read provenance exists only at B", f.queueA)
	}
	if !listHas(ids, f.queueB) {
		t.Errorf("R2 vacuous: authorized queue ticket %s at B is missing", f.queueB)
	}

	detailA := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+f.queueA, "", actor)
	if detailA.Code != http.StatusNotFound {
		t.Errorf("R2 LEAK: GET queue %s at A returned %d, want 404", f.queueA, detailA.Code)
	}
	detailB := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+f.queueB, "", actor)
	if detailB.Code != http.StatusOK {
		t.Errorf("R2 regression: GET queue %s at B returned %d, want 200", f.queueB, detailB.Code)
	}
}

// ---------------------------------------------------------------------------
// R3 — facility.read provenance at A only: A listed/readable, B absent + 404
// ---------------------------------------------------------------------------

func TestReadProvenance_R3_FacilityListAndDetailScopedToProvenance(t *testing.T) {
	f := newReadFixture(t)

	actor := seedReadActor(t, f.pool, "r3-facility", map[string][]string{
		f.facilityA: {"facility.read"},
		f.facilityB: {"queue.read"},
	})
	assertPreconditionFlatUnion(t, actor, "facility.read")
	assertPreconditionScope(t, f.pool, actor, f.facilityB)
	assertNoProvenance(t, actor, "facility.read", f.facilityB)

	h := scopedHandler(f.pool)

	listRec := dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities", "", actor)
	ids := listRowIDs(t, listRec)
	if listHas(ids, f.facilityB) {
		t.Errorf("R3 LEAK: facility %s appears in the list; facility.read provenance exists only at A", f.facilityB)
	}
	if !listHas(ids, f.facilityA) {
		t.Errorf("R3 vacuous: authorized facility %s is missing from the list", f.facilityA)
	}

	detailB := dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities/"+f.facilityB, "", actor)
	if detailB.Code != http.StatusNotFound {
		t.Errorf("R3 LEAK: GET facility %s returned %d, want 404", f.facilityB, detailB.Code)
	}
	detailA := dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities/"+f.facilityA, "", actor)
	if detailA.Code != http.StatusOK {
		t.Errorf("R3 regression: GET facility %s returned %d, want 200", f.facilityA, detailA.Code)
	}
}

// ---------------------------------------------------------------------------
// R4 — schedule.read provenance at B only: A schedule not listed or readable
// ---------------------------------------------------------------------------

func TestReadProvenance_R4_ScheduleAndServiceUnitScopedToProvenance(t *testing.T) {
	f := newReadFixture(t)

	actor := seedReadActor(t, f.pool, "r4-schedule", map[string][]string{
		f.facilityA: {"queue.read"},
		f.facilityB: {"schedule.read"},
	})
	assertPreconditionFlatUnion(t, actor, "schedule.read")
	assertPreconditionScope(t, f.pool, actor, f.facilityA)
	assertNoProvenance(t, actor, "schedule.read", f.facilityA)

	h := scopedHandler(f.pool)

	schedList := dispatchSchedulesRequest(h, http.MethodGet, "/api/v1/admin/schedules", "", actor)
	if listHas(listRowIDs(t, schedList), f.scheduleA) {
		t.Errorf("R4 LEAK: schedule %s at A appears in the list; schedule.read provenance exists only at B", f.scheduleA)
	}
	schedA := dispatchSchedulesRequest(h, http.MethodGet, "/api/v1/admin/schedules/"+f.scheduleA, "", actor)
	if schedA.Code != http.StatusNotFound {
		t.Errorf("R4 LEAK: GET schedule %s at A returned %d, want 404", f.scheduleA, schedA.Code)
	}
	schedB := dispatchSchedulesRequest(h, http.MethodGet, "/api/v1/admin/schedules/"+f.scheduleB, "", actor)
	if schedB.Code != http.StatusOK {
		t.Errorf("R4 regression: GET schedule %s at B returned %d, want 200", f.scheduleB, schedB.Code)
	}

	unitList := dispatchServiceUnitRequest(h, http.MethodGet, "/api/v1/admin/service-units", "", actor)
	if listHas(listRowIDs(t, unitList), f.unitA) {
		t.Errorf("R4 LEAK: service unit %s at A appears in the list; schedule.read provenance exists only at B", f.unitA)
	}
	unitA := dispatchServiceUnitRequest(h, http.MethodGet, "/api/v1/admin/service-units/"+f.unitA, "", actor)
	if unitA.Code != http.StatusNotFound {
		t.Errorf("R4 LEAK: GET service unit %s at A returned %d, want 404", f.unitA, unitA.Code)
	}
	unitB := dispatchServiceUnitRequest(h, http.MethodGet, "/api/v1/admin/service-units/"+f.unitB, "", actor)
	if unitB.Code != http.StatusOK {
		t.Errorf("R4 regression: GET service unit %s at B returned %d, want 200", f.unitB, unitB.Code)
	}
}

// ---------------------------------------------------------------------------
// R5 — global active super_admin keeps global read access
// ---------------------------------------------------------------------------

func TestReadProvenance_R5_GlobalSuperAdminReadsRemainGlobal(t *testing.T) {
	f := newReadFixture(t)
	f.appointmentA = seedReadAppointment(t, f.pool, f.facilityA, f.unitA)

	ctx := context.Background()
	globalID := uuid.NewString()
	if err := seedAppUser(ctx, f.pool, globalID, "r5-global"); err != nil {
		t.Fatalf("seed global user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, f.pool, globalID); err != nil {
		t.Fatalf("seed global super admin: %v", err)
	}
	actor := dbScopedActor(t, f.pool, globalID)

	scope := auth.FacilityScopeForActor(ctx, actor, auth.NewDBFacilityScope(f.pool))
	if !scope.Unrestricted {
		t.Fatalf("precondition failed: global super_admin scope is not unrestricted")
	}

	h := scopedHandler(f.pool)
	notifH := scopedNotificationsHandler(f.pool)

	facilities := listRowIDs(t, dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities", "", actor))
	if !listHas(facilities, f.facilityA) || !listHas(facilities, f.facilityB) {
		t.Errorf("R5 regression: global super_admin facility list = %v, want both A and B", facilities)
	}
	queues := listRowIDs(t, dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues", "", actor))
	if !listHas(queues, f.queueA) || !listHas(queues, f.queueB) {
		t.Errorf("R5 regression: global super_admin queue list = %v, want both A and B", queues)
	}
	appointments := listRowIDs(t, dispatchAppointmentRequest(h, http.MethodGet, "/api/v1/admin/appointments", "", actor))
	if !listHas(appointments, f.appointmentA) {
		t.Errorf("R5 regression: global super_admin appointment list omitted %s", f.appointmentA)
	}
	notifs := notificationFacilityByID(t, notificationRequest(t, notifH, http.MethodGet,
		"/api/v1/admin/notifications", actor))
	if _, ok := notifs[f.notifA]; !ok {
		t.Errorf("R5 regression: global super_admin cannot see notification at A")
	}
	counts := summaryCounts(t, notificationRequest(t, notifH, http.MethodGet,
		"/api/v1/admin/notifications/summary", actor))
	if counts["pending"] == 0 {
		t.Errorf("R5 regression: global super_admin summary reports zero pending, want the seeded rows")
	}
}

// ---------------------------------------------------------------------------
// R6 — zero-assignment non-super_admin: empty lists, 404 details, zero summary
// ---------------------------------------------------------------------------

func TestReadProvenance_R6_ZeroAssignmentNonSuperAdminIsFullyClosed(t *testing.T) {
	f := newReadFixture(t)

	// A non-super_admin role with read keys but NO facility (facility_id IS
	// NULL). It holds the keys flatly yet is scoped to nothing, so it must be
	// denied everywhere.
	actor := seedReadActor(t, f.pool, "r6-zero", map[string][]string{
		"": {"queue.read", "facility.read", "schedule.read", "appointment.read", "notification.read"},
	})
	for _, key := range []string{"queue.read", "facility.read", "schedule.read", "appointment.read", "notification.read"} {
		assertPreconditionFlatUnion(t, actor, key)
	}

	h := scopedHandler(f.pool)
	notifH := scopedNotificationsHandler(f.pool)

	if ids := listRowIDs(t, dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities", "", actor)); len(ids) != 0 {
		t.Errorf("R6 LEAK: zero-assignment facility list returned %v, want empty", ids)
	}
	if ids := listRowIDs(t, dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues", "", actor)); len(ids) != 0 {
		t.Errorf("R6 LEAK: zero-assignment queue list returned %v, want empty", ids)
	}
	if ids := listRowIDs(t, dispatchSchedulesRequest(h, http.MethodGet, "/api/v1/admin/schedules", "", actor)); len(ids) != 0 {
		t.Errorf("R6 LEAK: zero-assignment schedule list returned %v, want empty", ids)
	}
	if ids := listRowIDs(t, dispatchServiceUnitRequest(h, http.MethodGet, "/api/v1/admin/service-units", "", actor)); len(ids) != 0 {
		t.Errorf("R6 LEAK: zero-assignment service unit list returned %v, want empty", ids)
	}
	if ids := listRowIDs(t, dispatchAppointmentRequest(h, http.MethodGet, "/api/v1/admin/appointments", "", actor)); len(ids) != 0 {
		t.Errorf("R6 LEAK: zero-assignment appointment list returned %v, want empty", ids)
	}

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"facility":    dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities/"+f.facilityA, "", actor),
		"queue":       dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+f.queueA, "", actor),
		"schedule":    dispatchSchedulesRequest(h, http.MethodGet, "/api/v1/admin/schedules/"+f.scheduleA, "", actor),
		"serviceUnit": dispatchServiceUnitRequest(h, http.MethodGet, "/api/v1/admin/service-units/"+f.unitA, "", actor),
	} {
		if rec.Code != http.StatusNotFound {
			t.Errorf("R6 LEAK: %s detail returned %d, want 404", name, rec.Code)
		}
	}

	notifs := notificationFacilityByID(t, notificationRequest(t, notifH, http.MethodGet,
		"/api/v1/admin/notifications", actor))
	if len(notifs) != 0 {
		t.Errorf("R6 LEAK: zero-assignment notification list returned %v, want empty", mapKeys(notifs))
	}
	assertZeroSummary(t, summaryCounts(t, notificationRequest(t, notifH, http.MethodGet,
		"/api/v1/admin/notifications/summary", actor)))

	notifDetail := notificationRequest(t, notifH, http.MethodGet, "/api/v1/admin/notifications/"+f.notifA, actor)
	if notifDetail.Code != http.StatusNotFound {
		t.Errorf("R6 LEAK: notification detail returned %d, want 404", notifDetail.Code)
	}
}

// ---------------------------------------------------------------------------
// R7 — inactive / soft-deleted assignment contributes neither scope nor grants
// ---------------------------------------------------------------------------

func TestReadProvenance_R7_InactiveOrDeletedAssignmentGrantsNothing(t *testing.T) {
	f := newReadFixture(t)
	ctx := context.Background()

	actorID := uuid.NewString()
	if err := seedAppUser(ctx, f.pool, actorID, "r7-lifecycle"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, f.pool, actorID, f.facilityB, []string{"queue.read"}); err != nil {
		t.Fatalf("seed role at B: %v", err)
	}

	h := scopedHandler(f.pool)
	notifH := scopedNotificationsHandler(f.pool)

	t.Run("active assignment reads normally", func(t *testing.T) {
		actor := dbScopedActor(t, f.pool, actorID)
		if rec := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+f.queueB, "", actor); rec.Code != http.StatusOK {
			t.Fatalf("precondition failed: active assignment returned %d, want 200", rec.Code)
		}
	})

	// The SAME actor object must lose access once the DB assignment is
	// deactivated, because authorization reads live RBAC state.
	actor := dbScopedActor(t, f.pool, actorID)

	t.Run("inactive assignment", func(t *testing.T) {
		if err := setUserRoleLifecycle(ctx, f.pool, actorID, f.facilityB, "inactive", nil); err != nil {
			t.Fatalf("deactivate role: %v", err)
		}
		t.Cleanup(func() {
			_ = setUserRoleLifecycle(context.Background(), f.pool, actorID, f.facilityB, "active", nil)
		})
		if rec := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+f.queueB, "", actor); rec.Code != http.StatusNotFound {
			t.Errorf("R7 LEAK: inactive assignment still authorized a read (%d, want 404)", rec.Code)
		}
		if ids := listRowIDs(t, dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues", "", actor)); len(ids) != 0 {
			t.Errorf("R7 LEAK: inactive assignment still listed %v", ids)
		}
	})

	t.Run("soft deleted assignment", func(t *testing.T) {
		if err := setUserRoleLifecycle(ctx, f.pool, actorID, f.facilityB, "active", timePtr()); err != nil {
			t.Fatalf("soft delete role: %v", err)
		}
		t.Cleanup(func() {
			_ = setUserRoleLifecycle(context.Background(), f.pool, actorID, f.facilityB, "active", nil)
		})
		if rec := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+f.queueB, "", actor); rec.Code != http.StatusNotFound {
			t.Errorf("R7 LEAK: soft-deleted assignment still authorized a read (%d, want 404)", rec.Code)
		}
	})

	t.Run("deleted global super admin", func(t *testing.T) {
		globalID := uuid.NewString()
		if err := seedAppUser(ctx, f.pool, globalID, "r7-deleted-global"); err != nil {
			t.Fatalf("seed global user: %v", err)
		}
		if err := seedGlobalSuperAdminRole(ctx, f.pool, globalID); err != nil {
			t.Fatalf("seed global role: %v", err)
		}
		globalActor := dbScopedActor(t, f.pool, globalID)
		if rec := notificationRequest(t, notifH, http.MethodGet, "/api/v1/admin/notifications", globalActor); len(notificationFacilityByID(t, rec)) == 0 {
			t.Fatalf("precondition failed: active global super admin sees no notifications")
		}
		if _, err := f.pool.Exec(ctx,
			`UPDATE user_roles SET deleted_at = NOW() WHERE user_id = $1`, globalID); err != nil {
			t.Fatalf("soft delete global role: %v", err)
		}
		if rec := notificationRequest(t, notifH, http.MethodGet, "/api/v1/admin/notifications", globalActor); len(notificationFacilityByID(t, rec)) != 0 {
			t.Errorf("R7 LEAK: soft-deleted global super admin still read notifications")
		}
		assertZeroSummary(t, summaryCounts(t, notificationRequest(t, notifH, http.MethodGet,
			"/api/v1/admin/notifications/summary", globalActor)))
	})
}

func timePtr() *time.Time {
	now := time.Now()
	return &now
}

// ---------------------------------------------------------------------------
// R8 — JWT / client-supplied claims cannot create read provenance
// ---------------------------------------------------------------------------

func TestReadProvenance_R8_ClientClaimsCreateNoProvenance(t *testing.T) {
	f := newReadFixture(t)

	// An actor that carries the read keys ONLY in the flat (claim-derived)
	// permission set, with no DB-resolved facility grants at all. This is the
	// shape a forged or client-asserted claim would produce.
	forged := identity.Actor{
		Type:        identity.ActorUser,
		UserID:      "forged-subject",
		AppUserID:   f.facilityA, // resolves to no app_users row
		Permissions: []string{"queue.read", "facility.read", "schedule.read", "appointment.read", "notification.read"},
	}
	for _, key := range forged.Permissions {
		assertPreconditionFlatUnion(t, forged, key)
	}

	h := scopedHandler(f.pool)
	notifH := scopedNotificationsHandler(f.pool)

	if ids := listRowIDs(t, dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities", "", forged)); len(ids) != 0 {
		t.Errorf("R8 LEAK: claim-only facility list returned %v, want empty", ids)
	}
	if ids := listRowIDs(t, dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues", "", forged)); len(ids) != 0 {
		t.Errorf("R8 LEAK: claim-only queue list returned %v, want empty", ids)
	}
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"facility": dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities/"+f.facilityA, "", forged),
		"queue":    dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+f.queueA, "", forged),
		"schedule": dispatchSchedulesRequest(h, http.MethodGet, "/api/v1/admin/schedules/"+f.scheduleA, "", forged),
	} {
		if rec.Code != http.StatusNotFound {
			t.Errorf("R8 LEAK: claim-only %s detail returned %d, want 404", name, rec.Code)
		}
	}
	if rows := notificationFacilityByID(t, notificationRequest(t, notifH, http.MethodGet,
		"/api/v1/admin/notifications", forged)); len(rows) != 0 {
		t.Errorf("R8 LEAK: claim-only notification list returned %v, want empty", mapKeys(rows))
	}
	assertZeroSummary(t, summaryCounts(t, notificationRequest(t, notifH, http.MethodGet,
		"/api/v1/admin/notifications/summary", forged)))
}

// ---------------------------------------------------------------------------
// R9 — mixed grants must not produce Cartesian-product authorization
// ---------------------------------------------------------------------------

func TestReadProvenance_R9_MixedGrantsProduceNoCrossProduct(t *testing.T) {
	f := newReadFixture(t)

	// permission X at A, permission Y at B. Both facilities are in scope, and
	// the flat set contains both keys — yet neither key may be honored at the
	// facility where it was not granted.
	actor := seedReadActor(t, f.pool, "r9-mixed", map[string][]string{
		f.facilityA: {"queue.read"},
		f.facilityB: {"notification.read"},
	})
	assertPreconditionFlatUnion(t, actor, "queue.read")
	assertPreconditionFlatUnion(t, actor, "notification.read")
	assertPreconditionScope(t, f.pool, actor, f.facilityA)
	assertPreconditionScope(t, f.pool, actor, f.facilityB)
	// Both facilities are in scope and both keys are held flatly, yet neither
	// key may cross over: this is the cross-product condition under test.
	assertNoProvenance(t, actor, "queue.read", f.facilityB)
	assertNoProvenance(t, actor, "notification.read", f.facilityA)

	h := scopedHandler(f.pool)
	notifH := scopedNotificationsHandler(f.pool)

	// queue.read exists at A only.
	queues := listRowIDs(t, dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues", "", actor))
	if !listHas(queues, f.queueA) {
		t.Errorf("R9 regression: queue.read granted at A but %s missing from the list", f.queueA)
	}
	if listHas(queues, f.queueB) {
		t.Errorf("R9 LEAK: queue ticket %s at B visible although queue.read is granted only at A", f.queueB)
	}
	if rec := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+f.queueB, "", actor); rec.Code != http.StatusNotFound {
		t.Errorf("R9 LEAK: queue detail at B returned %d, want 404", rec.Code)
	}

	// notification.read exists at B only.
	notifs := notificationFacilityByID(t, notificationRequest(t, notifH, http.MethodGet,
		"/api/v1/admin/notifications", actor))
	if _, ok := notifs[f.notifB]; !ok {
		t.Errorf("R9 regression: notification.read granted at B but %s missing from the list", f.notifB)
	}
	if _, ok := notifs[f.notifA]; ok {
		t.Errorf("R9 LEAK: notification at A visible although notification.read is granted only at B")
	}
	if rec := notificationRequest(t, notifH, http.MethodGet, "/api/v1/admin/notifications/"+f.notifA, actor); rec.Code != http.StatusNotFound {
		t.Errorf("R9 LEAK: notification detail at A returned %d, want 404", rec.Code)
	}

	// The summary must aggregate only the facility holding notification.read.
	counts := summaryCounts(t, notificationRequest(t, notifH, http.MethodGet,
		"/api/v1/admin/notifications/summary", actor))
	if counts["pending"] != 1 {
		t.Errorf("R9 LEAK: summary pending = %d, want 1 (only the authorized facility's row)", counts["pending"])
	}
}
