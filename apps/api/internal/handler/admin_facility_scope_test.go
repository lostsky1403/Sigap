package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

type staticFacilityScope struct {
	result auth.FacilityScopeResult
}

func (s staticFacilityScope) AllowedFacilityIDs(context.Context, string) ([]uuid.UUID, error) {
	return s.result.IDs, s.result.Err
}

func (s staticFacilityScope) CanAccessFacility(_ context.Context, _ string, facilityID uuid.UUID) (bool, error) {
	if s.result.Err != nil {
		return false, s.result.Err
	}
	return auth.FacilityScopeResultAllows(s.result, facilityID), nil
}

func (s staticFacilityScope) ResolveFacilityScope(context.Context, string) auth.FacilityScopeResult {
	return s.result
}

func TestFacilityScope_AdminGlobalSuperAdminConsumers(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facilityA := uuid.NewString()
	facilityB := uuid.NewString()
	serviceUnitA := uuid.NewString()
	serviceUnitB := uuid.NewString()
	queueA := uuid.NewString()
	queueB := uuid.NewString()
	practitionerA := uuid.NewString()
	practitionerB := uuid.NewString()
	scheduleA := uuid.NewString()
	scheduleB := uuid.NewString()
	appointmentA := uuid.NewString()
	appointmentB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facilityA, "Scope Global A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facilityB, "Scope Global B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	for _, row := range []struct {
		id       string
		facility string
		name     string
		code     string
	}{
		{id: serviceUnitA, facility: facilityA, name: "Service A", code: "SVA"},
		{id: serviceUnitB, facility: facilityB, name: "Service B", code: "SVB"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
			 VALUES ($1, $2, $3, $4, '', true)`, row.id, row.facility, row.name, row.code); err != nil {
			t.Fatalf("seed service unit: %v", err)
		}
	}

	patientA := uuid.NewString()
	patientB := uuid.NewString()
	for _, patient := range []struct {
		id    string
		phone string
	}{
		{id: patientA, phone: "08555009991"},
		{id: patientB, phone: "08555009992"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO patients (id, full_name, phone, gender, date_of_birth)
			 VALUES ($1, 'Scope Patient', $2, 'L', '1990-01-01')`, patient.id, patient.phone); err != nil {
			t.Fatalf("seed patient: %v", err)
		}
	}
	for _, ticket := range []struct {
		id       string
		facility string
		patient  string
		number   string
	}{
		{id: queueA, facility: facilityA, patient: patientA, number: "SCO-0001"},
		{id: queueB, facility: facilityB, patient: patientB, number: "SCO-0002"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status)
			 VALUES ($1, $2, $3, 1, $4, 'waiting')`, ticket.id, ticket.facility, ticket.patient, ticket.number); err != nil {
			t.Fatalf("seed queue ticket: %v", err)
		}
	}
	for _, practitioner := range []struct {
		id       string
		facility string
		name     string
		role     string
	}{
		{id: practitionerA, facility: facilityA, name: "Practitioner A", role: "doctor"},
		{id: practitionerB, facility: facilityB, name: "Practitioner B", role: "doctor"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO practitioners (id, facility_id, display_name, role, is_active)
			 VALUES ($1, $2, $3, $4, true)`, practitioner.id, practitioner.facility, practitioner.name, practitioner.role); err != nil {
			t.Fatalf("seed practitioner: %v", err)
		}
	}
	for _, schedule := range []struct {
		id           string
		facility     string
		practitioner string
		unit         string
		date         string
	}{
		{id: scheduleA, facility: facilityA, practitioner: practitionerA, unit: serviceUnitA, date: "2026-12-01"},
		{id: scheduleB, facility: facilityB, practitioner: practitionerB, unit: serviceUnitB, date: "2026-12-02"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO practitioner_schedules (id, facility_id, practitioner_id, service_unit_id,
				schedule_date, start_time, end_time, slot_minutes, capacity_per_slot)
			 VALUES ($1, $2, $3, $4, $5, '08:00', '16:00', 30, 5)`, schedule.id, schedule.facility, schedule.practitioner, schedule.unit, schedule.date); err != nil {
			t.Fatalf("seed schedule: %v", err)
		}
	}
	for _, appointment := range []struct {
		id       string
		facility string
		unit     string
		schedule string
		phone    string
		code     string
	}{
		{id: appointmentA, facility: facilityA, unit: serviceUnitA, schedule: scheduleA, phone: "08555009991", code: "SCO123"},
		{id: appointmentB, facility: facilityB, unit: serviceUnitB, schedule: scheduleB, phone: "08555009992", code: "SCO124"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO appointments (id, facility_id, service_unit_id, practitioner_id, practitioner_schedule_id,
				appointment_time, status, patient_display_name, patient_phone, checkin_code, notes,
				queue_ticket_id, checkin_at, completed_at, cancelled_at, created_at, updated_at)
			 VALUES ($1, $2, $3, NULL, $4, NOW() + INTERVAL '1 day', 'scheduled', 'Scope Patient',
				$5, $6, '', NULL, NULL, NULL, NULL, NOW(), NOW())`, appointment.id, appointment.facility, appointment.unit, appointment.schedule, appointment.phone, appointment.code); err != nil {
			t.Fatalf("seed appointment: %v", err)
		}
	}

	if err := seedAppUser(ctx, pool, appUserID, "global-super-admin"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, appUserID); err != nil {
		t.Fatalf("seed global super admin: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "facility.read", "facility.manage", "queue.read", "queue.manage", "schedule.read", "schedule.manage", "appointment.read", "appointment.manage")
	request := func(method, path, body string, call func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		call(rec, req)
		return rec
	}
	assertStatus := func(rec *httptest.ResponseRecorder, want int, name string) {
		t.Helper()
		if rec.Code != want {
			t.Fatalf("%s: got %d want %d: %s", name, rec.Code, want, rec.Body.String())
		}
	}

	t.Run("all lists are global", func(t *testing.T) {
		cases := []struct {
			name  string
			field string
			path  string
			call  func(http.ResponseWriter, *http.Request)
		}{
			{name: "facilities", field: "id", path: "/api/v1/admin/facilities", call: h.ListFacilities},
			{name: "queues", field: "facility_id", path: "/api/v1/admin/queues", call: h.ListQueueTickets},
			{name: "service units", field: "facility_id", path: "/api/v1/admin/service-units", call: h.ListServiceUnits},
			{name: "schedules", field: "facility_id", path: "/api/v1/admin/schedules", call: h.ListSchedules},
			{name: "appointments", field: "facility_id", path: "/api/v1/admin/appointments", call: h.ListAppointments},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := request(http.MethodGet, tc.path, "", tc.call)
				assertStatus(rec, http.StatusOK, tc.name)
				var response struct {
					Data []map[string]any `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatalf("decode list: %v", err)
				}
				seen := make(map[string]bool)
				for _, row := range response.Data {
					if value, ok := row[tc.field].(string); ok {
						seen[value] = true
					}
				}
				if !seen[facilityA] || !seen[facilityB] {
					t.Fatalf("%s did not include both facilities: %s", tc.name, rec.Body.String())
				}
			})
		}
	})

	t.Run("details are global", func(t *testing.T) {
		cases := []struct {
			name string
			path string
			call func(http.ResponseWriter, *http.Request)
		}{
			{name: "facility", path: "/api/v1/admin/facilities/" + facilityA, call: h.GetFacility},
			{name: "queue", path: "/api/v1/admin/queues/" + queueA, call: h.GetQueueTicket},
			{name: "service unit", path: "/api/v1/admin/service-units/" + serviceUnitA, call: h.GetServiceUnit},
			{name: "schedule", path: "/api/v1/admin/schedules/" + scheduleA, call: h.GetSchedule},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertStatus(request(http.MethodGet, tc.path, "", tc.call), http.StatusOK, tc.name)
			})
		}
	})

	t.Run("mutations are global", func(t *testing.T) {
		cases := []struct {
			name   string
			method string
			path   string
			body   string
			want   int
			call   func(http.ResponseWriter, *http.Request)
		}{
			{name: "update facility", method: http.MethodPatch, path: "/api/v1/admin/facilities/" + facilityB, body: `{"name":"Scope Global B Updated"}`, want: http.StatusOK, call: h.UpdateFacility},
			{name: "deactivate facility", method: http.MethodPatch, path: "/api/v1/admin/facilities/" + facilityB + "/deactivate", want: http.StatusOK, call: h.DeactivateFacility},
			{name: "update queue", method: http.MethodPatch, path: "/api/v1/admin/queues/" + queueA + "/status", body: `{"status":"called"}`, want: http.StatusOK, call: h.UpdateQueueStatus},
			{name: "update service unit", method: http.MethodPatch, path: "/api/v1/admin/service-units/" + serviceUnitA, body: `{"name":"Service A Updated"}`, want: http.StatusOK, call: h.UpdateServiceUnit},
			{name: "update schedule", method: http.MethodPatch, path: "/api/v1/admin/schedules/" + scheduleA, body: `{"facility_id":"` + facilityB + `"}`, want: http.StatusOK, call: h.UpdateSchedule},
			{name: "update appointment", method: http.MethodPatch, path: "/api/v1/admin/appointments/" + appointmentA + "/status", body: `{"status":"cancelled"}`, want: http.StatusOK, call: h.UpdateAppointmentStatus},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertStatus(request(tc.method, tc.path, tc.body, tc.call), tc.want, tc.name)
			})
		}
	})

	t.Run("supplied facility ids permit global administration", func(t *testing.T) {
		newServiceBody := `{"facility_id":"` + facilityB + `","name":"Created Globally"}`
		assertStatus(request(http.MethodPost, "/api/v1/admin/service-units", newServiceBody, h.CreateServiceUnit), http.StatusCreated, "create service unit")
		newScheduleBody := `{"facility_id":"` + facilityB + `","practitioner_id":"` + practitionerB + `","service_unit_id":"` + serviceUnitB + `","schedule_date":"2026-12-02","start_time":"08:00","end_time":"16:00","slot_minutes":30,"capacity_per_slot":5}`
		assertStatus(request(http.MethodPost, "/api/v1/admin/schedules", newScheduleBody, h.CreateSchedule), http.StatusCreated, "create schedule")
		assertStatus(request(http.MethodPatch, "/api/v1/admin/service-units/"+serviceUnitA, `{"facility_id":"`+facilityB+`","name":"Moved Globally"}`, h.UpdateServiceUnit), http.StatusOK, "update supplied service facility")
		assertStatus(request(http.MethodPatch, "/api/v1/admin/schedules/"+scheduleA, `{"facility_id":"`+facilityB+`"}`, h.UpdateSchedule), http.StatusOK, "update supplied schedule facility")
	})
}

func TestFacilityScope_GlobalScopeDoesNotGrantPermission(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()
	appUserID := uuid.NewString()
	if err := seedAppUser(ctx, pool, appUserID, "global-without-permission"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, appUserID); err != nil {
		t.Fatalf("seed global super admin: %v", err)
	}

	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	actor := identity.Actor{
		Type:      identity.ActorUser,
		UserID:    "global-without-permission",
		AppUserID: appUserID,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	identity.RequirePermission(next).ServeHTTP(rec, req)
	if reached || rec.Code != http.StatusForbidden {
		t.Fatalf("unrestricted facility scope bypassed permission: reached=%v status=%d body=%s", reached, rec.Code, rec.Body.String())
	}
}

func TestFacilityScope_AdminConsumersFailClosed(t *testing.T) {
	facilityID := uuid.NewString()
	serviceUnitID := uuid.NewString()
	queueID := uuid.NewString()
	scheduleID := uuid.NewString()
	appointmentID := uuid.NewString()

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		list       bool
		wantStatus int
	}{
		{name: "ListFacilities", method: "ListFacilities", path: "/api/v1/admin/facilities", list: true, wantStatus: http.StatusOK},
		{name: "GetFacility", method: "GetFacility", path: "/api/v1/admin/facilities/" + facilityID, wantStatus: http.StatusNotFound},
		{name: "UpdateFacility", method: "UpdateFacility", path: "/api/v1/admin/facilities/" + facilityID, body: `{"name":"blocked"}`, wantStatus: http.StatusNotFound},
		{name: "DeactivateFacility", method: "DeactivateFacility", path: "/api/v1/admin/facilities/" + facilityID + "/deactivate", wantStatus: http.StatusNotFound},
		{name: "ListQueueTickets", method: "ListQueueTickets", path: "/api/v1/admin/queues", list: true, wantStatus: http.StatusOK},
		{name: "GetQueueTicket", method: "GetQueueTicket", path: "/api/v1/admin/queues/" + queueID, wantStatus: http.StatusNotFound},
		{name: "UpdateQueueStatus", method: "UpdateQueueStatus", path: "/api/v1/admin/queues/" + queueID + "/status", body: `{"status":"called"}`, wantStatus: http.StatusNotFound},
		{name: "ListServiceUnits", method: "ListServiceUnits", path: "/api/v1/admin/service-units", list: true, wantStatus: http.StatusOK},
		{name: "GetServiceUnit", method: "GetServiceUnit", path: "/api/v1/admin/service-units/" + serviceUnitID, wantStatus: http.StatusNotFound},
		{name: "CreateServiceUnit", method: "CreateServiceUnit", path: "/api/v1/admin/service-units", body: `{"facility_id":"` + facilityID + `","name":"Blocked"}`, wantStatus: http.StatusNotFound},
		{name: "UpdateServiceUnit", method: "UpdateServiceUnit", path: "/api/v1/admin/service-units/" + serviceUnitID, body: `{"facility_id":"` + facilityID + `","name":"Blocked"}`, wantStatus: http.StatusNotFound},
		{name: "ListSchedules", method: "ListSchedules", path: "/api/v1/admin/schedules", list: true, wantStatus: http.StatusOK},
		{name: "GetSchedule", method: "GetSchedule", path: "/api/v1/admin/schedules/" + scheduleID, wantStatus: http.StatusNotFound},
		{name: "CreateSchedule", method: "CreateSchedule", path: "/api/v1/admin/schedules", body: `{"facility_id":"` + facilityID + `","service_unit_id":"` + serviceUnitID + `","schedule_date":"2026-12-01","start_time":"08:00","end_time":"16:00","slot_minutes":30,"capacity_per_slot":5}`, wantStatus: http.StatusNotFound},
		{name: "UpdateSchedule", method: "UpdateSchedule", path: "/api/v1/admin/schedules/" + scheduleID, body: `{"facility_id":"` + facilityID + `"}`, wantStatus: http.StatusNotFound},
		{name: "ListAppointments", method: "ListAppointments", path: "/api/v1/admin/appointments", list: true, wantStatus: http.StatusOK},
		{name: "UpdateAppointmentStatus", method: "UpdateAppointmentStatus", path: "/api/v1/admin/appointments/" + appointmentID + "/status", body: `{"status":"completed"}`, wantStatus: http.StatusNotFound},
	}

	states := []struct {
		name   string
		result auth.FacilityScopeResult
	}{
		{name: "zero assignment", result: auth.FacilityScopeResult{}},
		{name: "resolver error", result: auth.FacilityScopeResult{Err: errors.New("scope unavailable")}},
	}

	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			h := NewAdminHandler(&pgxpool.Pool{}).WithFacilityScopeResolver(staticFacilityScope{result: state.result})
			actor := makeScopedActor(uuid.NewString(), "facility.manage", "queue.manage", "schedule.manage", "appointment.manage")

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					var body *strings.Reader
					if tc.body == "" {
						body = strings.NewReader("")
					} else {
						body = strings.NewReader(tc.body)
					}
					req := httptest.NewRequest(http.MethodGet, tc.path, body)
					if tc.body != "" {
						req.Header.Set("Content-Type", "application/json")
					}
					req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
					rec := httptest.NewRecorder()

					switch tc.method {
					case "ListFacilities":
						h.ListFacilities(rec, req)
					case "GetFacility":
						h.GetFacility(rec, req)
					case "UpdateFacility":
						h.UpdateFacility(rec, req)
					case "DeactivateFacility":
						h.DeactivateFacility(rec, req)
					case "ListQueueTickets":
						h.ListQueueTickets(rec, req)
					case "GetQueueTicket":
						h.GetQueueTicket(rec, req)
					case "UpdateQueueStatus":
						h.UpdateQueueStatus(rec, req)
					case "ListServiceUnits":
						h.ListServiceUnits(rec, req)
					case "GetServiceUnit":
						h.GetServiceUnit(rec, req)
					case "CreateServiceUnit":
						h.CreateServiceUnit(rec, req)
					case "UpdateServiceUnit":
						h.UpdateServiceUnit(rec, req)
					case "ListSchedules":
						h.ListSchedules(rec, req)
					case "GetSchedule":
						h.GetSchedule(rec, req)
					case "CreateSchedule":
						h.CreateSchedule(rec, req)
					case "UpdateSchedule":
						h.UpdateSchedule(rec, req)
					case "ListAppointments":
						h.ListAppointments(rec, req)
					case "UpdateAppointmentStatus":
						h.UpdateAppointmentStatus(rec, req)
					}

					if rec.Code != tc.wantStatus {
						t.Fatalf("got %d want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
					}
					if tc.list {
						var response struct {
							Data []json.RawMessage `json:"data"`
						}
						if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
							t.Fatalf("decode list response: %v", err)
						}
						if response.Data == nil || len(response.Data) != 0 {
							t.Fatalf("expected a non-nil empty data array, got %s", rec.Body.String())
						}
					}
				})
			}
		})
	}
}
