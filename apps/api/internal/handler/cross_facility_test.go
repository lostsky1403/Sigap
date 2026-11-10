package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/limiter"
)

// ---------------------------------------------------------------------------
// Cross-facility patient scenarios at the API surface.
//
// The unit under test is the CITIZEN surface introduced by
// patient_appointments.go + the booking.go ownership re-check, exercised
// across TWO organizations and TWO facilities. It deliberately reuses the
// disposable test_scope harness and the org/facility seed helpers rather than
// re-deriving RBAC or the facility-scope matrix (both already covered by
// facility_scope_test.go / org_tenancy_test.go / admin_scope_matrix_test.go).
//
//   Scenario A/B/C — citizen P (synthetic c307) books at org A/facility A and
//                   org B/facility B; ListMyAppointments returns BOTH.
//   Scenario D     — a second citizen cannot retrieve P's appointments.
//   Scenario E     — facility-A staff cannot read facility-B protected records.
//   Scenario F     — an authorized operation is audited with actor + scope.
//   Scenario G     — check-in issues facility-specific queue numbers on the WIB
//                    service day.
//   Negative       — guessed resource id, forged patient_account_id, forged
//                    organization id, and non-owner cancel/check-in are denied
//                    (403/404) without an existence oracle.
// ---------------------------------------------------------------------------

// crossCitizenP mirrors the synthetic citizen seeded in
// packages/db/seed/regional_tenancy.sql (subject local-patient-p1).
const crossCitizenP = "00000000-0000-0000-0000-00000000c307"
const crossCitizenPSubject = "local-patient-p1"

// citizenAccountFixture builds two complete organization+facility+service-unit
// trees (org A/facA/suA, org B/facB/suB) on the throwaway test_scope schema.
func citizenAccountFixture(t *testing.T) (pool *pgxpool.Pool, cleanup func(), orgA, orgB, facA, facB, suA, suB string) {
	t.Helper()
	pool, cleanup = newScopeTestPool(t)
	ctx := context.Background()

	orgA = uuid.NewString()
	orgB = uuid.NewString()
	facA = uuid.NewString()
	facB = uuid.NewString()
	suA = uuid.NewString()
	suB = uuid.NewString()

	seedOrganization(ctx, pool, orgA, "Cross Org A (synthetic)", "Jawa Barat")
	seedOrganization(ctx, pool, orgB, "Cross Org B (synthetic)", "DKI Jakarta")
	seedOrgFacility(ctx, pool, facA, "Cross Facility A", orgA)
	seedOrgFacility(ctx, pool, facB, "Cross Facility B", orgB)
	seedOrgServiceUnit(ctx, pool, suA, facA, "Cross Unit A", "CUA")
	seedOrgServiceUnit(ctx, pool, suB, facB, "Cross Unit B", "CUB")

	return pool, cleanup, orgA, orgB, facA, facB, suA, suB
}

// bookingBody returns a valid public booking payload for a facility/service
// unit pair, with a phone unique enough that a caller can vary it per booking.
func bookingBody(facilityID, serviceUnitID, phone string) map[string]any {
	return map[string]any{
		"facility_id":          facilityID,
		"service_unit_id":      serviceUnitID,
		"patient_display_name": "Citizen Patient (synthetic)",
		"patient_phone":        phone,
		"appointment_time":     time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}
}

// dispatchBookRequest drives the public booking path with the given body and
// optional actor. A zero actor keeps the request anonymous (code-only model);
// a citizen actor claims the appointment (patient_account_id derived from the
// actor, never from the body).
func dispatchBookRequest(h *BookingHandler, actor identity.Actor, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if !actor.IsZero() {
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	}
	rec := httptest.NewRecorder()
	h.BookAppointment(rec, req)
	return rec
}

type crossBookedAppointment struct {
	ID          string `json:"id"`
	CheckinCode string `json:"checkin_code"`
	Status      string `json:"status"`
}

func decodeCrossBooking(t *testing.T, rec *httptest.ResponseRecorder) crossBookedAppointment {
	t.Helper()
	var resp struct {
		Data crossBookedAppointment `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode booking: %v: %s", err, rec.Body.String())
	}
	return resp.Data
}

// TestCrossFacility_CitizenBooksAcrossOrgsAndListsBoth covers scenarios A, B
// and C: the same citizen account books at two facilities belonging to two
// different organizations, each booking is persisted against the chosen
// facility and owned by that account, and ListMyAppointments returns both.
func TestCrossFacility_CitizenBooksAcrossOrgsAndListsBoth(t *testing.T) {
	pool, cleanup, _, _, facA, facB, suA, suB := citizenAccountFixture(t)
	defer cleanup()
	ctx := context.Background()

	if err := seedAppUser(ctx, pool, crossCitizenP, crossCitizenPSubject); err != nil {
		t.Fatal(err)
	}
	citizenP := dbScopedActor(t, pool, crossCitizenP)
	if got := citizenP.PatientAccountID(); got != crossCitizenP {
		t.Fatalf("citizen P must resolve as the owner of %s, got %q", crossCitizenP, got)
	}

	bh := NewBookingHandler(pool, nil)

	// A: book at org A / facility A.
	recA := dispatchBookRequest(bh, citizenP, bookingBody(facA, suA, "085511100001"))
	if recA.Code != http.StatusOK {
		t.Fatalf("book at facA status=%d want 200: %s", recA.Code, recA.Body.String())
	}
	// B: book at org B / facility B.
	recB := dispatchBookRequest(bh, citizenP, bookingBody(facB, suB, "085511100002"))
	if recB.Code != http.StatusOK {
		t.Fatalf("book at facB status=%d want 200: %s", recB.Code, recB.Body.String())
	}

	apptA := decodeCrossBooking(t, recA)
	apptB := decodeCrossBooking(t, recB)

	// Ownership + facility are persisted server-side, never taken from the client.
	for name, tc := range map[string]struct {
		id         string
		wantFac    string
		wantStatus string
	}{
		"A": {apptA.ID, facA, "scheduled"},
		"B": {apptB.ID, facB, "scheduled"},
	} {
		var gotFac, gotOwner, gotStatus string
		if err := pool.QueryRow(ctx,
			`SELECT facility_id::text, COALESCE(patient_account_id::text, ''), status::text
			 FROM appointments WHERE id = $1`, tc.id,
		).Scan(&gotFac, &gotOwner, &gotStatus); err != nil {
			t.Fatalf("read persisted appointment %s: %v", name, err)
		}
		if gotFac != tc.wantFac {
			t.Errorf("booking %s facility=%q want %q", name, gotFac, tc.wantFac)
		}
		if gotOwner != crossCitizenP {
			t.Errorf("booking %s owner=%q want %q", name, gotOwner, crossCitizenP)
		}
		if gotStatus != tc.wantStatus {
			t.Errorf("booking %s status=%q want %q", name, gotStatus, tc.wantStatus)
		}
	}

	// C: ListMyAppointments returns BOTH facilities' appointments for P.
	ph := NewPatientHandler(pool, limiter.NewRateLimiter(30, time.Minute))
	rec := dispatchMyAppointments(ph, citizenP)
	if rec.Code != http.StatusOK {
		t.Fatalf("list my appointments status=%d want 200: %s", rec.Code, rec.Body.String())
	}
	rows := decodeAppointmentList(t, rec)
	if len(rows) != 2 {
		t.Fatalf("want both appointments (two orgs) listed, got %d: %+v", len(rows), rows)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.FacilityName] = true
	}
	if !seen["Cross Facility A"] || !seen["Cross Facility B"] {
		t.Fatalf("expected both facilities in list, got %+v", rows)
	}
}

// TestCrossFacility_SecondCitizenCannotSeeOthersAppointments covers scenario D
// and the forged patient_account_id negative: a second citizen sees none of
// P's appointments, and a booking body that smuggles P's account id is ignored
// (ownership derives from the actor, not the payload).
func TestCrossFacility_SecondCitizenCannotSeeOthersAppointments(t *testing.T) {
	pool, cleanup, _, _, facA, _, suA, _ := citizenAccountFixture(t)
	defer cleanup()
	ctx := context.Background()

	if err := seedAppUser(ctx, pool, crossCitizenP, crossCitizenPSubject); err != nil {
		t.Fatal(err)
	}
	citizenP := dbScopedActor(t, pool, crossCitizenP)

	qID := uuid.NewString()
	if err := seedAppUser(ctx, pool, qID, "local-patient-p2"); err != nil {
		t.Fatal(err)
	}
	citizenQ := dbScopedActor(t, pool, qID)
	if citizenQ.PatientAccountID() == crossCitizenP {
		t.Fatalf("citizen Q must not resolve to P's account")
	}

	bh := NewBookingHandler(pool, nil)

	// P owns one appointment at facA.
	recP := dispatchBookRequest(bh, citizenP, bookingBody(facA, suA, "085511200001"))
	if recP.Code != http.StatusOK {
		t.Fatalf("book as P status=%d want 200: %s", recP.Code, recP.Body.String())
	}

	ph := NewPatientHandler(pool, limiter.NewRateLimiter(30, time.Minute))

	// D: Q's list is scoped to Q alone and never surfaces P's rows.
	recQ := dispatchMyAppointments(ph, citizenQ)
	if recQ.Code != http.StatusOK {
		t.Fatalf("list as Q status=%d want 200: %s", recQ.Code, recQ.Body.String())
	}
	if rows := decodeAppointmentList(t, recQ); len(rows) != 0 {
		t.Fatalf("citizen Q must see zero of P's appointments, got %d: %+v", len(rows), rows)
	}

	// Negative (forged patient_account_id): Q books while smuggling P's id in
	// the body. The handler derives ownership from the Q actor, so the forged
	// field is inert and the row still belongs to Q.
	forged := bookingBody(facA, suA, "085511200002")
	forged["patient_account_id"] = crossCitizenP
	recForged := dispatchBookRequest(bh, citizenQ, forged)
	if recForged.Code != http.StatusOK {
		t.Fatalf("forged booking status=%d want 200: %s", recForged.Code, recForged.Body.String())
	}
	forgedAppt := decodeCrossBooking(t, recForged)
	var owner string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(patient_account_id::text, '') FROM appointments WHERE id = $1`,
		forgedAppt.ID).Scan(&owner); err != nil {
		t.Fatalf("read forged booking owner: %v", err)
	}
	if owner != qID {
		t.Fatalf("forged patient_account_id must be ignored: got owner %q want Q %q", owner, qID)
	}
}

// TestCrossFacility_FacilityStaffCannotReadSiblingRecords covers scenario E: a
// facility-A operator cannot mutate a sibling facility's protected appointment
// nor read its queue ticket; both are answered 404 (no existence oracle).
func TestCrossFacility_FacilityStaffCannotReadSiblingRecords(t *testing.T) {
	pool, cleanup, _, _, facA, facB, _, suB := citizenAccountFixture(t)
	defer cleanup()
	ctx := context.Background()

	opA := uuid.NewString()
	if err := seedAppUser(ctx, pool, opA, "cross-op-a"); err != nil {
		t.Fatal(err)
	}
	seedNamedRoleAtFacility(t, pool, opA, "operator", facA)

	// Protected records that live at facility B (org B).
	apptB, err := seedAppointmentForOwner(ctx, pool, facB, suB, nil, time.Now().UTC().Add(24*time.Hour), "CROSSB")
	if err != nil {
		t.Fatalf("seed facB appointment: %v", err)
	}
	queueB := seedQueueTicketForFacility(t, pool, facB)

	h := scopedHandler(pool)
	actorA := dbScopedActor(t, pool, opA)

	// Facility-scoped operator A must not mutate facB's appointment.
	recAppt := dispatchAppointmentRequest(h, http.MethodPatch, "/api/v1/admin/appointments/"+apptB+"/status", `{"status":"cancelled"}`, actorA)
	if recAppt.Code != http.StatusNotFound {
		t.Fatalf("operator A cross-facility appointment mutation status=%d want 404: %s", recAppt.Code, recAppt.Body.String())
	}

	// Facility-scoped operator A must not read facB's queue ticket.
	recQueue := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+queueB, "", actorA)
	if recQueue.Code != http.StatusNotFound {
		t.Fatalf("operator A cross-facility queue read status=%d want 404: %s", recQueue.Code, recQueue.Body.String())
	}
}

// TestCrossFacility_AuditCarriesActorAndScope covers scenario F: an authorized
// facility-scoped read is recorded with the actor's server-side app user id and
// the facility it acted on, without any PHI.
func TestCrossFacility_AuditCarriesActorAndScope(t *testing.T) {
	pool, cleanup, _, _, facA, _, _, _ := citizenAccountFixture(t)
	defer cleanup()
	ctx := context.Background()

	opA := uuid.NewString()
	if err := seedAppUser(ctx, pool, opA, "cross-audit-op"); err != nil {
		t.Fatal(err)
	}
	seedNamedRoleAtFacility(t, pool, opA, "operator", facA)

	h := NewAdminHandler(pool).
		WithAudit(audit.NewService(pool)).
		WithFacilityScopeResolver(auth.NewDBFacilityScope(pool))

	actorA := dbScopedActor(t, pool, opA)
	rec := dispatchRequest(h, http.MethodGet, "/api/v1/admin/facilities/"+facA, "", actorA)
	if rec.Code != http.StatusOK {
		t.Fatalf("facility.get status=%d want 200: %s", rec.Code, rec.Body.String())
	}

	var actorUserID, action, resourceType string
	var metadata map[string]any
	if err := pool.QueryRow(ctx,
		`SELECT actor_user_id::text, action, resource_type, metadata
		 FROM audit_events
		 WHERE action = 'facility.get'
		 ORDER BY occurred_at DESC
		 LIMIT 1`).Scan(&actorUserID, &action, &resourceType, &metadata); err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	if actorUserID != opA {
		t.Fatalf("audit actor_user_id=%q want %q", actorUserID, opA)
	}
	if resourceType != "facility" {
		t.Fatalf("audit resource_type=%q want facility", resourceType)
	}
	if got, _ := metadata["detail"].(string); got != facA {
		t.Fatalf("audit metadata must carry the acted-on facility id; detail=%v want %q (metadata=%v)", metadata["detail"], facA, metadata)
	}
	if _, hasPHI := metadata["patient"]; hasPHI {
		t.Fatalf("audit metadata must not carry PHI: %v", metadata)
	}
}

// dispatchCheckin drives the public check-in path for an appointment.
func dispatchCheckin(h *BookingHandler, appointmentID, code string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"checkin_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+appointmentID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.99.0.1:1234"
	rec := httptest.NewRecorder()
	h.CheckIn(rec, req)
	return rec
}

// TestCrossFacility_CheckinQueueFacilitySpecificWib covers scenario G: the REAL
// booking→check-in path issues a queue ticket into the appointment's own
// facility, the per-facility queue_number restarts independently, and the
// allocation lands on the WIB service day.
func TestCrossFacility_CheckinQueueFacilitySpecificWib(t *testing.T) {
	pool, cleanup, _, _, facA, facB, suA, suB := citizenAccountFixture(t)
	defer cleanup()
	ctx := context.Background()

	bh := NewBookingHandler(pool, nil)

	recA := dispatchBookRequest(bh, identity.Actor{}, bookingBody(facA, suA, "085511300001"))
	if recA.Code != http.StatusOK {
		t.Fatalf("book facA status=%d want 200: %s", recA.Code, recA.Body.String())
	}
	recB := dispatchBookRequest(bh, identity.Actor{}, bookingBody(facB, suB, "085511300002"))
	if recB.Code != http.StatusOK {
		t.Fatalf("book facB status=%d want 200: %s", recB.Code, recB.Body.String())
	}
	apptA := decodeCrossBooking(t, recA)
	apptB := decodeCrossBooking(t, recB)

	os.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	ch := NewBookingHandler(pool, nil).
		WithQueueService(&failingCheckinQueueService{}).
		WithCheckinLimiter(limiter.NewRateLimiter(10, time.Minute))

	checkin := func(appointment *crossBookedAppointment) (ticketID, status string) {
		t.Helper()
		rec := dispatchCheckin(ch, appointment.ID, appointment.CheckinCode)
		if rec.Code != http.StatusOK {
			t.Fatalf("check-in %s status=%d want 200: %s", appointment.ID, rec.Code, rec.Body.String())
		}
		var resp struct {
			Data struct {
				QueueTicketID    string `json:"queue_ticket_id"`
				FormattedNumber  string `json:"formatted_number"`
				AppointmentStatus string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode check-in: %v: %s", err, rec.Body.String())
		}
		return resp.Data.QueueTicketID, resp.Data.AppointmentStatus
	}

	ticketA, statusA := checkin(&apptA)
	if statusA != "queued" {
		t.Fatalf("facA check-in status=%q want queued", statusA)
	}
	ticketB, statusB := checkin(&apptB)
	if statusB != "queued" {
		t.Fatalf("facB check-in status=%q want queued", statusB)
	}

	assertTicket := func(ticketID, wantFac string) {
		t.Helper()
		var gotFac string
		var queueNumber int
		var registered time.Time
		if err := pool.QueryRow(ctx,
			`SELECT facility_id::text, queue_number, registered_at
			 FROM queue_tickets WHERE id = $1`, ticketID,
		).Scan(&gotFac, &queueNumber, &registered); err != nil {
			t.Fatalf("read queue ticket %s: %v", ticketID, err)
		}
		if gotFac != wantFac {
			t.Errorf("ticket %s facility=%q want %q", ticketID, gotFac, wantFac)
		}
		if queueNumber != 1 {
			t.Errorf("ticket %s queue_number=%d want 1 (independent per-facility counter)", ticketID, queueNumber)
		}
		// WIB-correct: the issued ticket is stamped on today's WIB service day.
		if gotDay := registered.In(serviceDayWIB).Format("2006-01-02"); gotDay != time.Now().In(serviceDayWIB).Format("2006-01-02") {
			t.Errorf("ticket %s registered on WIB day %s want %s", ticketID, gotDay, time.Now().In(serviceDayWIB).Format("2006-01-02"))
		}
	}

	assertTicket(ticketA, facA)
	assertTicket(ticketB, facB)

	// The per-facility, per-WIB-day counter saw exactly one ticket at each
	// facility — grouping by facility proves the sequences are independent.
	if got := queueNumbersWithin(t, ctx, pool, facA, time.Now()); len(got) != 1 || got[0] != 1 {
		t.Fatalf("facA WIB-day queue numbers=%v want [1]", got)
	}
	if got := queueNumbersWithin(t, ctx, pool, facB, time.Now()); len(got) != 1 || got[0] != 1 {
		t.Fatalf("facB WIB-day queue numbers=%v want [1]", got)
	}
}

// dispatchCancel drives the public cancel path with an optional actor.
func dispatchCancel(h *BookingHandler, actor identity.Actor, code string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"checkin_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if !actor.IsZero() {
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	}
	rec := httptest.NewRecorder()
	h.CancelAppointment(rec, req)
	return rec
}

// TestCrossFacility_GuessedResourceIDNotFound covers the guessed-id negative: a
// facility-scoped staff cannot act on an id that does not exist anywhere, and
// the answer is a flat 404 (no existence oracle).
func TestCrossFacility_GuessedResourceIDNotFound(t *testing.T) {
	pool, cleanup, _, _, facA, _, _, _ := citizenAccountFixture(t)
	defer cleanup()
	ctx := context.Background()

	opA := uuid.NewString()
	if err := seedAppUser(ctx, pool, opA, "cross-guess-op"); err != nil {
		t.Fatal(err)
	}
	seedNamedRoleAtFacility(t, pool, opA, "operator", facA)

	h := scopedHandler(pool)
	actorA := dbScopedActor(t, pool, opA)

	guessedAppt := uuid.NewString()
	guessedQueue := uuid.NewString()

	recAppt := dispatchAppointmentRequest(h, http.MethodPatch, "/api/v1/admin/appointments/"+guessedAppt+"/status", `{"status":"cancelled"}`, actorA)
	if recAppt.Code != http.StatusNotFound {
		t.Fatalf("guessed appointment status=%d want 404: %s", recAppt.Code, recAppt.Body.String())
	}

	recQueue := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues/"+guessedQueue, "", actorA)
	if recQueue.Code != http.StatusNotFound {
		t.Fatalf("guessed queue ticket status=%d want 404: %s", recQueue.Code, recQueue.Body.String())
	}
}

// TestCrossFacility_ForgedOrganizationIDDenied covers the forged organization
// id negative: an org-scoped admin cannot read a sibling organization (404,
// indistinguishable from "does not exist").
func TestCrossFacility_ForgedOrganizationIDDenied(t *testing.T) {
	pool, cleanup, orgA, orgB, _, _, _, _ := citizenAccountFixture(t)
	defer cleanup()
	ctx := context.Background()

	orgAdminA := uuid.NewString()
	if err := seedAppUser(ctx, pool, orgAdminA, "cross-org-admin-a"); err != nil {
		t.Fatal(err)
	}
	seedOrgScopedRole(ctx, pool, orgAdminA, "organization_admin", orgA, orgAdminPermissions())

	h := scopedHandler(pool)
	actorA := dbScopedActor(t, pool, orgAdminA)

	// Own org resolves.
	if rec := dispatchOrgRequest(h, http.MethodGet, "/api/v1/admin/organizations/"+orgA, actorA); rec.Code != http.StatusOK {
		t.Fatalf("own org overview status=%d want 200: %s", rec.Code, rec.Body.String())
	}
	// Sibling org is a flat 404 — no existence leak.
	if rec := dispatchOrgRequest(h, http.MethodGet, "/api/v1/admin/organizations/"+orgB, actorA); rec.Code != http.StatusNotFound {
		t.Fatalf("forged org overview status=%d want 404: %s", rec.Code, rec.Body.String())
	}
}

// TestCrossFacility_CancelByNonOwnerDenied covers the unauthorized-mutation
// negative: an owned appointment can only be cancelled by its owner account;
// a different signed-in citizen holding the code is denied (403) and the row
// stays scheduled.
func TestCrossFacility_CancelByNonOwnerDenied(t *testing.T) {
	pool, cleanup, _, _, facA, _, suA, _ := citizenAccountFixture(t)
	defer cleanup()
	ctx := context.Background()

	if err := seedAppUser(ctx, pool, crossCitizenP, crossCitizenPSubject); err != nil {
		t.Fatal(err)
	}
	citizenP := dbScopedActor(t, pool, crossCitizenP)

	qID := uuid.NewString()
	if err := seedAppUser(ctx, pool, qID, "local-patient-p2x"); err != nil {
		t.Fatal(err)
	}
	citizenQ := dbScopedActor(t, pool, qID)

	bh := NewBookingHandler(pool, nil)

	// P owns an appointment and receives its code.
	apptP := decodeCrossBooking(t, dispatchBookRequest(bh, citizenP, bookingBody(facA, suA, "085511400001")))

	// Q (different citizen) attempts to cancel P's appointment with the code.
	rec := dispatchCancel(bh, citizenQ, apptP.CheckinCode)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cancel by non-owner status=%d want 403: %s", rec.Code, rec.Body.String())
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM appointments WHERE id = $1`, apptP.ID).Scan(&status); err != nil {
		t.Fatalf("read appointment status: %v", err)
	}
	if status != "scheduled" {
		t.Fatalf("non-owner cancel must leave the appointment scheduled, got %q", status)
	}
}