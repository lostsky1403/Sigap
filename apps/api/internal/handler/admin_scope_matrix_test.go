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
	fixture.actorA = makeScopedActor(scopedID, "facility.read", "facility.manage", "queue.read", "queue.manage", "schedule.read", "schedule.manage", "appointment.read", "appointment.manage")
	fixture.zeroActor = makeScopedActor(zeroID, "facility.read", "facility.manage", "queue.read", "queue.manage", "schedule.read", "schedule.manage", "appointment.read", "appointment.manage")
	fixture.inactiveActor = makeScopedActor(inactiveID, "facility.read", "facility.manage", "queue.read", "queue.manage", "schedule.read", "schedule.manage", "appointment.read", "appointment.manage")
	fixture.deletedActor = makeScopedActor(deletedID, "facility.read", "facility.manage", "queue.read", "queue.manage", "schedule.read", "schedule.manage", "appointment.read", "appointment.manage")
	fixture.claimsOnlyActor = makeScopedActor(claimsOnlyID, "facility.read", "facility.manage", "queue.read", "queue.manage", "schedule.read", "schedule.manage", "appointment.read", "appointment.manage")
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
