package handler

import (
	"context"
	"encoding/json"
	"errors"
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

type adminScopeMutationFixture struct {
	facilityA       string
	facilityB       string
	serviceUnitA    string
	serviceUnitB    string
	queueA          string
	queueB          string
	practitionerA   string
	practitionerB   string
	scheduleA       string
	scheduleB       string
	appointmentA    string
	appointmentB    string
	actorA          identity.Actor
	zeroActor       identity.Actor
	inactiveActor   identity.Actor
	deletedActor    identity.Actor
	claimsOnlyActor identity.Actor
}

func seedAdminScopeMutationFixture(t *testing.T, pool *pgxpool.Pool) adminScopeMutationFixture {
	t.Helper()
	ctx := context.Background()
	fixture := adminScopeMutationFixture{
		facilityA:     uuid.NewString(),
		facilityB:     uuid.NewString(),
		serviceUnitA:  uuid.NewString(),
		serviceUnitB:  uuid.NewString(),
		queueA:        uuid.NewString(),
		queueB:        uuid.NewString(),
		practitionerA: uuid.NewString(),
		practitionerB: uuid.NewString(),
		scheduleA:     uuid.NewString(),
		scheduleB:     uuid.NewString(),
		appointmentA:  uuid.NewString(),
		appointmentB:  uuid.NewString(),
	}
	for _, facility := range []struct{ id, name string }{{fixture.facilityA, "Scope Matrix A"}, {fixture.facilityB, "Scope Matrix B"}} {
		if err := seedFacilityByName(ctx, pool, facility.id, facility.name); err != nil {
			t.Fatalf("seed facility: %v", err)
		}
	}
	for _, serviceUnit := range []struct{ id, facility, name, code string }{
		{fixture.serviceUnitA, fixture.facilityA, "Service A", "SCA"},
		{fixture.serviceUnitB, fixture.facilityB, "Service B", "SCB"},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO service_units (id, facility_id, name, code, description, is_active)
VALUES ($1, $2, $3, $4, '', true)`, serviceUnit.id, serviceUnit.facility, serviceUnit.name, serviceUnit.code); err != nil {
			t.Fatalf("seed service unit: %v", err)
		}
	}
	for _, patient := range []struct{ id, phone string }{
		{uuid.NewString(), "08555008991"},
		{uuid.NewString(), "08555008992"},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO patients (id, full_name, phone, gender, date_of_birth)
VALUES ($1, 'Scope Patient', $2, 'L', '1990-01-01')`, patient.id, patient.phone); err != nil {
			t.Fatalf("seed patient: %v", err)
		}
	}
	patients, err := pool.Query(ctx, `SELECT id FROM patients WHERE phone IN ('08555008991', '08555008992') ORDER BY phone`)
	if err != nil {
		t.Fatalf("load patients: %v", err)
	}
	var patientIDs []string
	for patients.Next() {
		var id string
		if err := patients.Scan(&id); err != nil {
			patients.Close()
			t.Fatalf("scan patient: %v", err)
		}
		patientIDs = append(patientIDs, id)
	}
	patients.Close()
	if err := patients.Err(); err != nil {
		t.Fatalf("iterate patients: %v", err)
	}
	if len(patientIDs) != 2 {
		t.Fatalf("seeded patient count=%d want 2", len(patientIDs))
	}
	for _, ticket := range []struct {
		id, facility, patient, number string
	}{
		{fixture.queueA, fixture.facilityA, patientIDs[0], "SCA-0001"},
		{fixture.queueB, fixture.facilityB, patientIDs[1], "SCB-0001"},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status)
VALUES ($1, $2, $3, 1, $4, 'waiting')`, ticket.id, ticket.facility, ticket.patient, ticket.number); err != nil {
			t.Fatalf("seed queue ticket: %v", err)
		}
	}
	for _, practitioner := range []struct{ id, facility, name string }{
		{fixture.practitionerA, fixture.facilityA, "Practitioner A"},
		{fixture.practitionerB, fixture.facilityB, "Practitioner B"},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO practitioners (id, facility_id, display_name, role, is_active)
VALUES ($1, $2, $3, 'doctor', true)`, practitioner.id, practitioner.facility, practitioner.name); err != nil {
			t.Fatalf("seed practitioner: %v", err)
		}
	}
	for _, schedule := range []struct {
		id, facility, practitioner, unit, date string
	}{
		{fixture.scheduleA, fixture.facilityA, fixture.practitionerA, fixture.serviceUnitA, "2026-12-01"},
		{fixture.scheduleB, fixture.facilityB, fixture.practitionerB, fixture.serviceUnitB, "2026-12-02"},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO practitioner_schedules (id, facility_id, practitioner_id, service_unit_id,
    schedule_date, start_time, end_time, slot_minutes, capacity_per_slot)
VALUES ($1, $2, $3, $4, $5, '08:00', '16:00', 30, 5)`, schedule.id, schedule.facility, schedule.practitioner, schedule.unit, schedule.date); err != nil {
			t.Fatalf("seed schedule: %v", err)
		}
	}
	for _, appointment := range []struct {
		id, facility, unit, schedule, phone, code string
	}{
		{fixture.appointmentA, fixture.facilityA, fixture.serviceUnitA, fixture.scheduleA, "08555008991", "SCMA"},
		{fixture.appointmentB, fixture.facilityB, fixture.serviceUnitB, fixture.scheduleB, "08555008992", "SCMB"},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO appointments (id, facility_id, service_unit_id, practitioner_id, practitioner_schedule_id,
    appointment_time, status, patient_display_name, patient_phone, checkin_code, notes,
    queue_ticket_id, checkin_at, completed_at, cancelled_at, created_at, updated_at)
VALUES ($1, $2, $3, NULL, $4, NOW() + INTERVAL '1 day', 'scheduled', 'Scope Patient',
    $5, $6, '', NULL, NULL, NULL, NULL, NOW(), NOW())`, appointment.id, appointment.facility, appointment.unit, appointment.schedule, appointment.phone, appointment.code); err != nil {
			t.Fatalf("seed appointment: %v", err)
		}
	}
	scopedID := uuid.NewString()
	if err := seedAppUser(ctx, pool, scopedID, "scope-matrix-a"); err != nil {
		t.Fatalf("seed scoped user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, scopedID, fixture.facilityA, []string{
		"facility.read", "facility.manage", "queue.read", "queue.manage",
		"schedule.read", "schedule.manage", "appointment.read", "appointment.manage",
	}); err != nil {
		t.Fatalf("seed scoped role: %v", err)
	}
	zeroID := uuid.NewString()
	if err := seedAppUser(ctx, pool, zeroID, "scope-matrix-zero"); err != nil {
		t.Fatalf("seed zero user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, zeroID, "", []string{
		"facility.read", "facility.manage", "queue.read", "queue.manage",
		"schedule.read", "schedule.manage", "appointment.read", "appointment.manage",
	}); err != nil {
		t.Fatalf("seed zero role: %v", err)
	}
	inactiveID := uuid.NewString()
	deletedID := uuid.NewString()
	if err := seedAppUser(ctx, pool, inactiveID, "scope-matrix-inactive"); err != nil {
		t.Fatalf("seed inactive user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, inactiveID); err != nil {
		t.Fatalf("seed inactive global role: %v", err)
	}
	if err := setUserRoleLifecycle(ctx, pool, inactiveID, "", "inactive", nil); err != nil {
		t.Fatalf("set inactive global role: %v", err)
	}
	if err := seedAppUser(ctx, pool, deletedID, "scope-matrix-deleted"); err != nil {
		t.Fatalf("seed deleted user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, deletedID); err != nil {
		t.Fatalf("seed deleted global role: %v", err)
	}
	deletedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := setUserRoleLifecycle(ctx, pool, deletedID, "", "active", &deletedAt); err != nil {
		t.Fatalf("soft delete global role: %v", err)
	}
	claimsOnlyID := uuid.NewString()
	if err := seedAppUser(ctx, pool, claimsOnlyID, "scope-matrix-claims-only"); err != nil {
		t.Fatalf("seed claims-only user: %v", err)
	}
	// actorA is the faithful production shape: flat keys AND facility
	// provenance, both resolved from user_roles.
	fixture.actorA = dbScopedActor(t, pool, scopedID)
	// The fail-closed actors below deliberately keep every mutation permission
	// in their FLAT key set while carrying no facility provenance at all.
	// That is the strongest adversarial shape: the permission is present and
	// actor.HasPermission reports true, yet authorization must still be denied
	// because the key was never granted at any facility.
	fixture.zeroActor = makeScopedActor(zeroID, allTestPermissions()...)
	fixture.inactiveActor = makeScopedActor(inactiveID, allTestPermissions()...)
	fixture.deletedActor = makeScopedActor(deletedID, allTestPermissions()...)
	fixture.claimsOnlyActor = makeScopedActor(claimsOnlyID, allTestPermissions()...)
	return fixture
}

type adminMutationOperation struct {
	name          string
	method        string
	path          string
	body          string
	allowedStatus int
	deniedStatus  int
	call          func(*AdminHandler, http.ResponseWriter, *http.Request)
}

func adminScopeRequest(t *testing.T, h *AdminHandler, method, path, body string, actor identity.Actor) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	switch {
	case strings.HasPrefix(path, "/api/v1/admin/facilities"):
		h.FacilitiesRouter(rec, req)
	case strings.HasPrefix(path, "/api/v1/admin/service-units"):
		h.ServiceUnitsRouter(rec, req)
	case strings.HasPrefix(path, "/api/v1/admin/queues"):
		h.QueuesRouter(rec, req)
	case strings.HasPrefix(path, "/api/v1/admin/schedules"):
		h.SchedulesRouter(rec, req)
	case strings.HasPrefix(path, "/api/v1/admin/appointments"):
		h.AppointmentsRouter(rec, req)
	default:
		t.Fatalf("unsupported admin path %q", path)
	}
	return rec
}

func adminScopeSnapshot(t *testing.T, pool *pgxpool.Pool, table, id string) string {
	t.Helper()
	queries := map[string]string{
		"facilities":             `SELECT row_to_json(t)::text FROM facilities t WHERE t.id = $1`,
		"service_units":          `SELECT row_to_json(t)::text FROM service_units t WHERE t.id = $1`,
		"queue_tickets":          `SELECT row_to_json(t)::text FROM queue_tickets t WHERE t.id = $1`,
		"practitioner_schedules": `SELECT row_to_json(t)::text FROM practitioner_schedules t WHERE t.id = $1`,
		"appointments":           `SELECT row_to_json(t)::text FROM appointments t WHERE t.id = $1`,
	}
	query, ok := queries[table]
	if !ok {
		t.Fatalf("unsupported snapshot table %q", table)
	}
	var snapshot string
	if err := pool.QueryRow(context.Background(), query, id).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot %s %s: %v", table, id, err)
	}
	return snapshot
}

func assertAdminSnapshotUnchanged(t *testing.T, pool *pgxpool.Pool, table, id, before string) {
	t.Helper()
	after := adminScopeSnapshot(t, pool, table, id)
	if after != before {
		t.Fatalf("%s row %s changed after denied request\nbefore: %s\nafter:  %s", table, id, before, after)
	}
}

func adminScopeCount(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	queries := map[string]string{
		"service_units":          `SELECT count(*) FROM service_units`,
		"practitioner_schedules": `SELECT count(*) FROM practitioner_schedules`,
	}
	query, ok := queries[table]
	if !ok {
		t.Fatalf("unsupported count table %q", table)
	}
	var count int
	if err := pool.QueryRow(context.Background(), query).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestFacilityScope_AdminMutationMatrix(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	fixture := seedAdminScopeMutationFixture(t, pool)
	h := scopedHandler(pool)

	allowed := []adminMutationOperation{
		{name: "update facility", method: http.MethodPatch, path: "/api/v1/admin/facilities/" + fixture.facilityA, body: `{"name":"Allowed Facility A"}`, allowedStatus: http.StatusOK, call: (*AdminHandler).UpdateFacility},
		{name: "deactivate facility", method: http.MethodPatch, path: "/api/v1/admin/facilities/" + fixture.facilityA + "/deactivate", allowedStatus: http.StatusOK, call: (*AdminHandler).DeactivateFacility},
		{name: "update queue", method: http.MethodPatch, path: "/api/v1/admin/queues/" + fixture.queueA + "/status", body: `{"status":"called"}`, allowedStatus: http.StatusOK, call: (*AdminHandler).UpdateQueueStatus},
		{name: "update service unit", method: http.MethodPatch, path: "/api/v1/admin/service-units/" + fixture.serviceUnitA, body: `{"name":"Allowed Service A"}`, allowedStatus: http.StatusOK, call: (*AdminHandler).UpdateServiceUnit},
		{name: "update schedule", method: http.MethodPatch, path: "/api/v1/admin/schedules/" + fixture.scheduleA, body: `{"facility_id":"` + fixture.facilityA + `"}`, allowedStatus: http.StatusOK, call: (*AdminHandler).UpdateSchedule},
		{name: "update appointment", method: http.MethodPatch, path: "/api/v1/admin/appointments/" + fixture.appointmentA + "/status", body: `{"status":"checked_in"}`, allowedStatus: http.StatusOK, call: (*AdminHandler).UpdateAppointmentStatus},
		{name: "create service unit", method: http.MethodPost, path: "/api/v1/admin/service-units", body: `{"facility_id":"` + fixture.facilityA + `","name":"Created Allowed Service"}`, allowedStatus: http.StatusCreated, call: (*AdminHandler).CreateServiceUnit},
		{name: "create schedule", method: http.MethodPost, path: "/api/v1/admin/schedules", body: `{"facility_id":"` + fixture.facilityA + `","practitioner_id":"` + fixture.practitionerA + `","service_unit_id":"` + fixture.serviceUnitA + `","schedule_date":"2027-01-01","start_time":"08:00","end_time":"16:00","slot_minutes":30,"capacity_per_slot":5}`, allowedStatus: http.StatusCreated, call: (*AdminHandler).CreateSchedule},
		{name: "supplied service unit facility", method: http.MethodPatch, path: "/api/v1/admin/service-units/" + fixture.serviceUnitA, body: `{"facility_id":"` + fixture.facilityA + `","name":"Allowed Supplied Service"}`, allowedStatus: http.StatusOK, call: (*AdminHandler).UpdateServiceUnit},
		{name: "supplied schedule facility", method: http.MethodPatch, path: "/api/v1/admin/schedules/" + fixture.scheduleA, body: `{"facility_id":"` + fixture.facilityA + `"}`, allowedStatus: http.StatusOK, call: (*AdminHandler).UpdateSchedule},
	}
	for _, operation := range allowed {
		t.Run("facility A actor can "+operation.name, func(t *testing.T) {
			rec := adminScopeRequest(t, h, operation.method, operation.path, operation.body, fixture.actorA)
			if rec.Code != operation.allowedStatus {
				t.Fatalf("got status %d want %d: %s", rec.Code, operation.allowedStatus, rec.Body.String())
			}
		})
	}

	denied := []struct {
		adminMutationOperation
		table string
		id    string
	}{
		{adminMutationOperation{name: "update facility", method: http.MethodPatch, path: "/api/v1/admin/facilities/" + fixture.facilityB, body: `{"name":"Denied Facility B"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateFacility}, "facilities", fixture.facilityB},
		{adminMutationOperation{name: "deactivate facility", method: http.MethodPatch, path: "/api/v1/admin/facilities/" + fixture.facilityB + "/deactivate", deniedStatus: http.StatusNotFound, call: (*AdminHandler).DeactivateFacility}, "facilities", fixture.facilityB},
		{adminMutationOperation{name: "update queue", method: http.MethodPatch, path: "/api/v1/admin/queues/" + fixture.queueB + "/status", body: `{"status":"called"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateQueueStatus}, "queue_tickets", fixture.queueB},
		{adminMutationOperation{name: "update service unit", method: http.MethodPatch, path: "/api/v1/admin/service-units/" + fixture.serviceUnitB, body: `{"name":"Denied Service B"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateServiceUnit}, "service_units", fixture.serviceUnitB},
		{adminMutationOperation{name: "update schedule", method: http.MethodPatch, path: "/api/v1/admin/schedules/" + fixture.scheduleB, body: `{"facility_id":"` + fixture.facilityA + `"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateSchedule}, "practitioner_schedules", fixture.scheduleB},
		{adminMutationOperation{name: "update appointment", method: http.MethodPatch, path: "/api/v1/admin/appointments/" + fixture.appointmentB + "/status", body: `{"status":"checked_in"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateAppointmentStatus}, "appointments", fixture.appointmentB},
		{adminMutationOperation{name: "supplied service unit facility", method: http.MethodPatch, path: "/api/v1/admin/service-units/" + fixture.serviceUnitA, body: `{"facility_id":"` + fixture.facilityB + `","name":"Denied Supplied Service"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateServiceUnit}, "service_units", fixture.serviceUnitA},
		{adminMutationOperation{name: "supplied schedule facility", method: http.MethodPatch, path: "/api/v1/admin/schedules/" + fixture.scheduleA, body: `{"facility_id":"` + fixture.facilityB + `"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateSchedule}, "practitioner_schedules", fixture.scheduleA},
	}
	for _, operation := range denied {
		t.Run("facility A actor cannot "+operation.name, func(t *testing.T) {
			before := adminScopeSnapshot(t, pool, operation.table, operation.id)
			rec := adminScopeRequest(t, h, operation.method, operation.path, operation.body, fixture.actorA)
			if rec.Code != operation.deniedStatus {
				t.Fatalf("got status %d want %d: %s", rec.Code, operation.deniedStatus, rec.Body.String())
			}
			assertAdminSnapshotUnchanged(t, pool, operation.table, operation.id, before)
		})
	}

	for _, operation := range []adminMutationOperation{
		{name: "create service unit", method: http.MethodPost, path: "/api/v1/admin/service-units", body: `{"facility_id":"` + fixture.facilityB + `","name":"Denied Created Service"}`, deniedStatus: http.StatusForbidden, call: (*AdminHandler).CreateServiceUnit},
		{name: "create schedule", method: http.MethodPost, path: "/api/v1/admin/schedules", body: `{"facility_id":"` + fixture.facilityB + `","service_unit_id":"` + fixture.serviceUnitB + `","schedule_date":"2027-01-02","start_time":"08:00","end_time":"16:00","slot_minutes":30,"capacity_per_slot":5}`, deniedStatus: http.StatusForbidden, call: (*AdminHandler).CreateSchedule},
	} {
		t.Run("facility A actor cannot "+operation.name, func(t *testing.T) {
			table := "service_units"
			if operation.name == "create schedule" {
				table = "practitioner_schedules"
			}
			before := adminScopeCount(t, pool, table)
			rec := adminScopeRequest(t, h, operation.method, operation.path, operation.body, fixture.actorA)
			if rec.Code != operation.deniedStatus {
				t.Fatalf("got status %d want %d: %s", rec.Code, operation.deniedStatus, rec.Body.String())
			}
			if after := adminScopeCount(t, pool, table); after != before {
				t.Fatalf("%s count changed: before=%d after=%d", table, before, after)
			}
		})
	}
}

func TestFacilityScope_AdminActorFailClosedMatrix(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	fixture := seedAdminScopeMutationFixture(t, pool)
	h := scopedHandler(pool)
	operations := []adminMutationOperation{
		{name: "update facility", method: http.MethodPatch, path: "/api/v1/admin/facilities/" + fixture.facilityB, body: `{"name":"Actor Denied"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateFacility},
		{name: "deactivate facility", method: http.MethodPatch, path: "/api/v1/admin/facilities/" + fixture.facilityB + "/deactivate", deniedStatus: http.StatusNotFound, call: (*AdminHandler).DeactivateFacility},
		{name: "update queue", method: http.MethodPatch, path: "/api/v1/admin/queues/" + fixture.queueB + "/status", body: `{"status":"called"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateQueueStatus},
		{name: "update service unit", method: http.MethodPatch, path: "/api/v1/admin/service-units/" + fixture.serviceUnitB, body: `{"name":"Actor Denied"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateServiceUnit},
		{name: "update schedule", method: http.MethodPatch, path: "/api/v1/admin/schedules/" + fixture.scheduleB, body: `{"facility_id":"` + fixture.facilityA + `"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateSchedule},
		{name: "update appointment", method: http.MethodPatch, path: "/api/v1/admin/appointments/" + fixture.appointmentB + "/status", body: `{"status":"checked_in"}`, deniedStatus: http.StatusNotFound, call: (*AdminHandler).UpdateAppointmentStatus},
	}
	actors := []struct {
		name  string
		actor identity.Actor
	}{
		{name: "zero assignment non super admin", actor: fixture.zeroActor},
		{name: "inactive global super admin", actor: fixture.inactiveActor},
		{name: "soft deleted global super admin", actor: fixture.deletedActor},
		{name: "claims only", actor: fixture.claimsOnlyActor},
		{name: "resolver error", actor: makeScopedActor(uuid.NewString(), "facility.manage", "queue.manage", "schedule.manage", "appointment.manage")},
	}
	for _, actorState := range actors {
		t.Run(actorState.name, func(t *testing.T) {
			actor := actorState.actor
			handler := h
			if actorState.name == "resolver error" {
				handler = NewAdminHandler(pool).WithFacilityScopeResolver(staticFacilityScope{result: auth.FacilityScopeResult{Err: errors.New("scope unavailable")}})
			}
			listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
			listReq = listReq.WithContext(identity.ContextWithActor(listReq.Context(), actor))
			listRec := httptest.NewRecorder()
			handler.ListFacilities(listRec, listReq)
			if listRec.Code != http.StatusOK {
				t.Fatalf("list status=%d want 200: %s", listRec.Code, listRec.Body.String())
			}
			var listResponse struct {
				Data []json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(listRec.Body.Bytes(), &listResponse); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			if listResponse.Data == nil || len(listResponse.Data) != 0 {
				t.Fatalf("expected empty list, got %s", listRec.Body.String())
			}
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					table, id := adminMutationTarget(operation.name, fixture)
					before := adminScopeSnapshot(t, pool, table, id)
					rec := adminScopeRequest(t, handler, operation.method, operation.path, operation.body, actor)
					if rec.Code != operation.deniedStatus {
						t.Fatalf("got status %d want %d: %s", rec.Code, operation.deniedStatus, rec.Body.String())
					}
					assertAdminSnapshotUnchanged(t, pool, table, id, before)
				})
			}
		})
	}
}

func TestFacilityScope_CreateFacility(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	scopedFacilityID := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, scopedFacilityID, "Create Facility Scope Seed"); err != nil {
		t.Fatalf("seed scoped facility: %v", err)
	}

	globalUserID := uuid.NewString()
	if err := seedAppUser(ctx, pool, globalUserID, "create-facility-global"); err != nil {
		t.Fatalf("seed global user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, globalUserID); err != nil {
		t.Fatalf("seed global super admin: %v", err)
	}

	scopedUserID := uuid.NewString()
	if err := seedAppUser(ctx, pool, scopedUserID, "create-facility-scoped"); err != nil {
		t.Fatalf("seed scoped user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, scopedUserID, scopedFacilityID, []string{"facility.manage"}); err != nil {
		t.Fatalf("seed scoped user roles: %v", err)
	}

	zeroUserID := uuid.NewString()
	if err := seedAppUser(ctx, pool, zeroUserID, "create-facility-zero"); err != nil {
		t.Fatalf("seed zero user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, zeroUserID, "", []string{"facility.manage"}); err != nil {
		t.Fatalf("seed zero user roles: %v", err)
	}

	resolverErrorHandler := NewAdminHandler(pool).WithFacilityScopeResolver(staticFacilityScope{
		result: auth.FacilityScopeResult{Err: errors.New("scope unavailable")},
	})
	create := func(t *testing.T, h *AdminHandler, actor identity.Actor, marker string) *httptest.ResponseRecorder {
		t.Helper()
		body := `{"name":"` + marker + `","type":"puskesmas","address":"Jl. Regression","kecamatan":"Kecamatan","kabupaten_kota":"Kabupaten","provinsi":"Provinsi","phone":"08123456789","total_beds":1,"available_beds":1,"short_code":"` + marker + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/facilities", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.CreateFacility(rec, req)
		return rec
	}
	count := func(t *testing.T, marker string) int {
		t.Helper()
		var got int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM facilities WHERE name = $1`, marker).Scan(&got); err != nil {
			t.Fatalf("count created facility: %v", err)
		}
		return got
	}

	cases := []struct {
		name      string
		handler   *AdminHandler
		actor     identity.Actor
		wantCode  int
		wantCount int
	}{
		{
			name:      "global super admin",
			handler:   scopedHandler(pool),
			actor:     dbScopedActor(t, pool, globalUserID),
			wantCode:  http.StatusCreated,
			wantCount: 1,
		},
		{
			name:      "dev",
			handler:   scopedHandler(pool),
			actor:     makeDevActor(uuid.NewString(), "facility.manage"),
			wantCode:  http.StatusCreated,
			wantCount: 1,
		},
		{
			name:      "zero assignment non super admin",
			handler:   scopedHandler(pool),
			actor:     makeScopedActor(zeroUserID, "facility.manage"),
			wantCode:  http.StatusNotFound,
			wantCount: 0,
		},
		{
			name:      "facility scoped actor",
			handler:   scopedHandler(pool),
			actor:     dbScopedActor(t, pool, scopedUserID),
			wantCode:  http.StatusNotFound,
			wantCount: 0,
		},
		{
			name:      "resolver error",
			handler:   resolverErrorHandler,
			actor:     dbScopedActor(t, pool, scopedUserID),
			wantCode:  http.StatusNotFound,
			wantCount: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marker := "Create-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			rec := create(t, tc.handler, tc.actor, marker)
			if rec.Code != tc.wantCode {
				t.Fatalf("got status %d want %d: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if got := count(t, marker); got != tc.wantCount {
				t.Fatalf("facility row count=%d want %d", got, tc.wantCount)
			}
		})
	}
}

func adminMutationTarget(name string, fixture adminScopeMutationFixture) (string, string) {
	switch name {
	case "update facility", "deactivate facility":
		return "facilities", fixture.facilityB
	case "update queue":
		return "queue_tickets", fixture.queueB
	case "update service unit":
		return "service_units", fixture.serviceUnitB
	case "update schedule":
		return "practitioner_schedules", fixture.scheduleB
	case "update appointment":
		return "appointments", fixture.appointmentB
	default:
		return "", ""
	}
}

// crossFacilityFixture holds the two facilities and the tickets/rows used by
// the cross-facility privilege tests.
type crossFacilityFixture struct {
	facilityA string
	facilityB string
	queueA    string
	queueB    string
	unitA     string
	unitB     string
}

func seedCrossFacilityFixture(t *testing.T, pool *pgxpool.Pool) crossFacilityFixture {
	t.Helper()
	ctx := context.Background()
	f := crossFacilityFixture{
		facilityA: uuid.NewString(),
		facilityB: uuid.NewString(),
	}
	for _, seed := range []struct{ id, name string }{{f.facilityA, "Cross Facility A"}, {f.facilityB, "Cross Facility B"}} {
		if err := seedFacilityByName(ctx, pool, seed.id, seed.name); err != nil {
			t.Fatalf("seed facility: %v", err)
		}
	}
	f.queueA = seedQueueTicketForFacility(t, pool, f.facilityA)
	f.queueB = seedQueueTicketForFacility(t, pool, f.facilityB)
	for _, unit := range []struct{ id, facility, name, code string }{
		{uuid.NewString(), f.facilityA, "Unit A", "UHA"},
		{uuid.NewString(), f.facilityB, "Unit B", "UHB"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
			 VALUES ($1, $2, $3, $4, '', true)`, unit.id, unit.facility, unit.name, unit.code); err != nil {
			t.Fatalf("seed service unit: %v", err)
		}
		if unit.facility == f.facilityA {
			f.unitA = unit.id
		} else {
			f.unitB = unit.id
		}
	}
	return f
}

// TestFacilityScope_CrossFacilityPrivilege is the end-to-end regression for the
// confirmed defect: a single user holding DIFFERENT roles at DIFFERENT
// facilities is schema-valid (user_roles PK is (user_id, role_id)), and the
// former flat-union permission set let queue.manage granted only at facility B
// authorize PATCH /api/v1/admin/queues/<ticketInA>/status when A was also in
// scope. The same mutation must succeed at B and be denied at A.
func TestFacilityScope_CrossFacilityPrivilege(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()
	fixture := seedCrossFacilityFixture(t, pool)
	h := scopedHandler(pool)

	patchQueueStatus := func(t *testing.T, actor identity.Actor, ticketID string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch,
			"/api/v1/admin/queues/"+ticketID+"/status", strings.NewReader(`{"status":"called"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.QueuesRouter(rec, req)
		return rec
	}
	queueStatus := func(t *testing.T, ticketID string) string {
		t.Helper()
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM queue_tickets WHERE id = $1`, ticketID).Scan(&status); err != nil {
			t.Fatalf("read queue status: %v", err)
		}
		return status
	}

	// CASE1: viewer@A + operator@B.
	t.Run("operator at B cannot mutate a ticket at A", func(t *testing.T) {
		userID := uuid.NewString()
		if err := seedAppUser(ctx, pool, userID, "cross-case1"); err != nil {
			t.Fatalf("seed app user: %v", err)
		}
		seedNamedRoleAtFacility(t, pool, userID, "viewer", fixture.facilityA)
		seedNamedRoleAtFacility(t, pool, userID, "operator", fixture.facilityB)

		actor := dbScopedActor(t, pool, userID)
		// The flat union contains queue.manage, and BOTH facilities are in
		// scope: only the per-facility provenance distinguishes them.
		if !actor.HasPermission("queue.manage") {
			t.Fatalf("precondition: flat permission set should contain queue.manage, got %v", actor.Permissions)
		}
		scope := auth.FacilityScopeForActor(ctx, actor, h.scopeResolver)
		if scope.Err != nil || len(scope.IDs) != 2 {
			t.Fatalf("precondition: both facilities should be in scope, got %+v", scope)
		}

		before := adminScopeSnapshot(t, pool, "queue_tickets", fixture.queueA)
		rec := patchQueueStatus(t, actor, fixture.queueA)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("mutation at facility A: got status %d want 404: %s", rec.Code, rec.Body.String())
		}
		assertAdminSnapshotUnchanged(t, pool, "queue_tickets", fixture.queueA, before)
		if got := queueStatus(t, fixture.queueA); got != "waiting" {
			t.Errorf("ticket A status=%q want waiting", got)
		}

		// The same mutation at the facility where the grant lives must work.
		rec = patchQueueStatus(t, actor, fixture.queueB)
		if rec.Code != http.StatusOK {
			t.Fatalf("mutation at facility B: got status %d want 200: %s", rec.Code, rec.Body.String())
		}
		if got := queueStatus(t, fixture.queueB); got != "called" {
			t.Errorf("ticket B status=%q want called", got)
		}
	})

	// CASE2: facility_admin@A + a narrower role@B must not spill.
	t.Run("facility admin at A does not spill manage permission to B", func(t *testing.T) {
		userID := uuid.NewString()
		if err := seedAppUser(ctx, pool, userID, "cross-case2"); err != nil {
			t.Fatalf("seed app user: %v", err)
		}
		seedNamedRoleAtFacility(t, pool, userID, "facility_admin", fixture.facilityA)
		seedNamedRoleAtFacility(t, pool, userID, "viewer", fixture.facilityB)

		actor := dbScopedActor(t, pool, userID)
		if !actor.HasPermission("schedule.manage") {
			t.Fatalf("precondition: flat permission set should contain schedule.manage, got %v", actor.Permissions)
		}

		// Renaming the service unit at A is allowed.
		req := httptest.NewRequest(http.MethodPatch,
			"/api/v1/admin/service-units/"+fixture.unitA, strings.NewReader(`{"name":"Renamed By Admin A"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.ServiceUnitsRouter(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("update service unit at A: got status %d want 200: %s", rec.Code, rec.Body.String())
		}

		// The same mutation at B must be denied: viewer@B cannot manage.
		before := adminScopeSnapshot(t, pool, "service_units", fixture.unitB)
		reqB := httptest.NewRequest(http.MethodPatch,
			"/api/v1/admin/service-units/"+fixture.unitB, strings.NewReader(`{"name":"Renamed By Admin A"}`))
		reqB.Header.Set("Content-Type", "application/json")
		reqB = reqB.WithContext(identity.ContextWithActor(reqB.Context(), actor))
		recB := httptest.NewRecorder()
		h.ServiceUnitsRouter(recB, reqB)
		if recB.Code != http.StatusNotFound {
			t.Fatalf("update service unit at B: got status %d want 404: %s", recB.Code, recB.Body.String())
		}
		assertAdminSnapshotUnchanged(t, pool, "service_units", fixture.unitB, before)
	})

	// CASE8: a supplied facility_id is authorized against the SUPPLIED target
	// facility, so a re-parenting move into a facility where the caller holds
	// no manage permission is denied even though the source facility is in
	// scope and the flat key set contains schedule.manage.
	t.Run("supplied facility id is authorized at the supplied facility", func(t *testing.T) {
		userID := uuid.NewString()
		if err := seedAppUser(ctx, pool, userID, "cross-case8"); err != nil {
			t.Fatalf("seed app user: %v", err)
		}
		// facility_admin only at A; B is in scope only through a viewer role.
		seedNamedRoleAtFacility(t, pool, userID, "facility_admin", fixture.facilityA)
		seedNamedRoleAtFacility(t, pool, userID, "viewer", fixture.facilityB)

		actor := dbScopedActor(t, pool, userID)
		if !actor.HasPermission("schedule.manage") {
			t.Fatalf("precondition: flat permission set should contain schedule.manage, got %v", actor.Permissions)
		}
		scope := auth.FacilityScopeForActor(ctx, actor, h.scopeResolver)
		// B is in scope, so scope-only enforcement would allow the move.
		if !auth.FacilityScopeResultAllows(scope, uuid.MustParse(fixture.facilityB)) {
			t.Fatalf("precondition: facility B should be in scope, got %+v", scope)
		}

		before := adminScopeSnapshot(t, pool, "service_units", fixture.unitA)
		req := httptest.NewRequest(http.MethodPatch,
			"/api/v1/admin/service-units/"+fixture.unitA,
			strings.NewReader(`{"facility_id":"`+fixture.facilityB+`","name":"Moved Out Of Scope"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.ServiceUnitsRouter(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("re-parent into B: got status %d want 404: %s", rec.Code, rec.Body.String())
		}
		assertAdminSnapshotUnchanged(t, pool, "service_units", fixture.unitA, before)

		// The same move into the facility where the grant lives is allowed.
		okReq := httptest.NewRequest(http.MethodPatch,
			"/api/v1/admin/service-units/"+fixture.unitA,
			strings.NewReader(`{"facility_id":"`+fixture.facilityA+`","name":"Renamed At A"}`))
		okReq.Header.Set("Content-Type", "application/json")
		okReq = okReq.WithContext(identity.ContextWithActor(okReq.Context(), actor))
		okRec := httptest.NewRecorder()
		h.ServiceUnitsRouter(okRec, okReq)
		if okRec.Code != http.StatusOK {
			t.Fatalf("update at granted facility: got status %d want 200: %s", okRec.Code, okRec.Body.String())
		}
	})

	// CASE7: an actor whose keys come only from client claims holds
	// queue.manage flatly but has no provenance, so the mutation is denied even
	// when its own user_roles assignment would place the facility in scope.
	t.Run("claims only actor cannot mutate at any facility", func(t *testing.T) {
		userID := uuid.NewString()
		if err := seedAppUser(ctx, pool, userID, "cross-case7"); err != nil {
			t.Fatalf("seed app user: %v", err)
		}
		seedNamedRoleAtFacility(t, pool, userID, "operator", fixture.facilityB)

		// Every mutation permission is present in the flat set, none has
		// provenance: exactly the shape a forged token would produce.
		forged := makeScopedActor(userID, "queue.manage", "facility.manage", "schedule.manage", "appointment.manage")
		if !forged.HasPermission("queue.manage") {
			t.Fatalf("precondition: forged actor should hold the flat key, got %v", forged.Permissions)
		}

		before := adminScopeSnapshot(t, pool, "queue_tickets", fixture.queueB)
		rec := patchQueueStatus(t, forged, fixture.queueB)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("claims-only mutation: got status %d want 404: %s", rec.Code, rec.Body.String())
		}
		assertAdminSnapshotUnchanged(t, pool, "queue_tickets", fixture.queueB, before)
	})
}
