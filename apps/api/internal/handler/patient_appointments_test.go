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

	"github.com/sigap/sigap/apps/api/internal/identity"
)

// dispatchMyAppointments drives the appointments list endpoint with a given
// actor bound to the request context.
func dispatchMyAppointments(h *PatientHandler, actor identity.Actor) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/patient/appointments", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.ListMyAppointments(rec, req)
	return rec
}

// seedCitizenAppUser inserts an active app_users row to satisfy the
// appointments.patient_account_id FK. The subject column is left NULL: the
// actor under test is constructed directly, not resolved from the DB.
func seedCitizenAppUser(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	id := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO app_users (id, email, display_name, status)
		 VALUES ($1, $2, 'Citizen Test', 'active')
		 ON CONFLICT (id) DO NOTHING`, id, id+"@test.local")
	return id, err
}

// seedAppointmentForOwner inserts one appointment. ownerID may be nil for the
// anonymous (NULL patient_account_id) state.
func seedAppointmentForOwner(
	ctx context.Context,
	pool *pgxpool.Pool,
	facilityID, serviceUnitID string,
	ownerID *string,
	at time.Time,
	code string,
) (string, error) {
	apptID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO appointments
		   (id, facility_id, service_unit_id, patient_display_name,
		    patient_phone, appointment_time, status, checkin_code, patient_account_id)
		 VALUES ($1, $2, $3, 'Citizen Test', '085550000001', $4, 'scheduled', $5, $6)`,
		apptID, facilityID, serviceUnitID, at, code, ownerID)
	return apptID, err
}

// seedListFixture creates one facility and one service unit for the ownership
// scope test. Returns the two ids and a cleanup function.
func seedListFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (facilityID, serviceUnitID string, cleanup func()) {
	t.Helper()
	facilityID = uuid.NewString()
	serviceUnitID = uuid.NewString()

	if _, err := pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, short_code)
		 VALUES ($1, 'List Fac', 'puskesmas', 'Addr', 'Kec', 'Kab', 'Prov', '000', 'LF')
		 ON CONFLICT (id) DO NOTHING`, facilityID); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name)
		 VALUES ($1, $2, 'List Unit')
		 ON CONFLICT (id) DO NOTHING`, serviceUnitID, facilityID); err != nil {
		t.Fatalf("seed service_unit: %v", err)
	}

	cleanup = func() {
		ctx := context.Background()
		pool.Exec(ctx, `DELETE FROM service_units WHERE id = $1`, serviceUnitID)
		pool.Exec(ctx, `DELETE FROM facilities WHERE id = $1`, facilityID)
	}
	return facilityID, serviceUnitID, cleanup
}

// --- Non-DB: the ownership guard fires before any pool access ---

func TestListMyAppointments_Unauthenticated_Returns401(t *testing.T) {
	h := NewPatientHandler(nil, nil)
	rec := dispatchMyAppointments(h, identity.Actor{})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for no actor, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListMyAppointments_StaffActor_Returns401(t *testing.T) {
	h := NewPatientHandler(nil, nil)
	// A staff actor holds a permission key, so PatientAccountID() must fail
	// closed even though AppUserID is populated.
	staff := identity.Actor{
		Type:        identity.ActorUser,
		UserID:      "test-staff",
		AppUserID:   uuid.NewString(),
		Permissions: []string{"appointment.read"},
	}
	rec := dispatchMyAppointments(h, staff)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for staff actor, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListMyAppointments_DevActor_Returns401(t *testing.T) {
	h := NewPatientHandler(nil, nil)
	dev := identity.Actor{
		Type:      identity.ActorDev,
		UserID:    "dev",
		AppUserID: uuid.NewString(),
		IsDev:     true,
	}
	rec := dispatchMyAppointments(h, dev)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for dev actor, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Integration: account scoping (gated on SIGAP_DATABASE_URL) ---

// decodeAppointmentList decodes the wire `data` array of a successful list
// response. It returns nil when the backend answered `data: null`.
func decodeAppointmentList(t *testing.T, rec *httptest.ResponseRecorder) []PatientAppointment {
	t.Helper()
	var body struct {
		Success bool                 `json:"success"`
		Data    []PatientAppointment `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !body.Success {
		t.Fatalf("expected success=true, got %v", body.Success)
	}
	return body.Data
}

func TestListMyAppointments_ReturnsOnlyOwned(t *testing.T) {
	pool, h := setupPatientTest(t)
	ctx := context.Background()

	facilityID, serviceUnitID, cleanupFixture := seedListFixture(t, ctx, pool)
	defer cleanupFixture()

	ownerID, err := seedCitizenAppUser(ctx, pool)
	if err != nil {
		t.Fatalf("seed owner app_user: %v", err)
	}
	otherID, err := seedCitizenAppUser(ctx, pool)
	if err != nil {
		t.Fatalf("seed other app_user: %v", err)
	}

	base := time.Now().UTC()
	ownedAppt, err := seedAppointmentForOwner(ctx, pool, facilityID, serviceUnitID, &ownerID, base, "OWNED1")
	if err != nil {
		t.Fatalf("seed owned appointment: %v", err)
	}
	otherAppt, err := seedAppointmentForOwner(ctx, pool, facilityID, serviceUnitID, &otherID, base.Add(time.Hour), "OTHER1")
	if err != nil {
		t.Fatalf("seed other appointment: %v", err)
	}
	anonAppt, err := seedAppointmentForOwner(ctx, pool, facilityID, serviceUnitID, nil, base.Add(2*time.Hour), "ANON01")
	if err != nil {
		t.Fatalf("seed anonymous appointment: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, ownedAppt)
		pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, otherAppt)
		pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, anonAppt)
		pool.Exec(ctx, `DELETE FROM app_users WHERE id = $1`, ownerID)
		pool.Exec(ctx, `DELETE FROM app_users WHERE id = $1`, otherID)
	})

	actor := identity.Actor{Type: identity.ActorUser, UserID: "test-owner", AppUserID: ownerID}
	rec := dispatchMyAppointments(h, actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rows := decodeAppointmentList(t, rec)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 owned appointment, got %d", len(rows))
	}
	if rows[0].ID != ownedAppt {
		t.Errorf("expected only the owned appointment %s, got %s", ownedAppt, rows[0].ID)
	}
	if rows[0].FacilityName != "List Fac" {
		t.Errorf("expected facility_name 'List Fac', got %q", rows[0].FacilityName)
	}
	if rows[0].ServiceUnitName != "List Unit" {
		t.Errorf("expected service_unit_name 'List Unit', got %q", rows[0].ServiceUnitName)
	}
	if rows[0].Status != "scheduled" {
		t.Errorf("expected appointment_status 'scheduled', got %q", rows[0].Status)
	}
}

func TestListMyAppointments_NewestFirst(t *testing.T) {
	pool, h := setupPatientTest(t)
	ctx := context.Background()

	facilityID, serviceUnitID, cleanupFixture := seedListFixture(t, ctx, pool)
	defer cleanupFixture()

	ownerID, err := seedCitizenAppUser(ctx, pool)
	if err != nil {
		t.Fatalf("seed owner app_user: %v", err)
	}

	base := time.Now().UTC()
	older, err := seedAppointmentForOwner(ctx, pool, facilityID, serviceUnitID, &ownerID, base, "OLDER1")
	if err != nil {
		t.Fatalf("seed older appointment: %v", err)
	}
	newer, err := seedAppointmentForOwner(ctx, pool, facilityID, serviceUnitID, &ownerID, base.Add(3*time.Hour), "NEWER1")
	if err != nil {
		t.Fatalf("seed newer appointment: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, older)
		pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, newer)
		pool.Exec(ctx, `DELETE FROM app_users WHERE id = $1`, ownerID)
	})

	actor := identity.Actor{Type: identity.ActorUser, UserID: "test-owner", AppUserID: ownerID}
	rec := dispatchMyAppointments(h, actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rows := decodeAppointmentList(t, rec)
	if len(rows) != 2 {
		t.Fatalf("expected 2 appointments, got %d", len(rows))
	}
	if rows[0].ID != newer || rows[1].ID != older {
		t.Errorf("expected newest first (newer=%s, older=%s), got %s then %s", newer, older, rows[0].ID, rows[1].ID)
	}
}

func TestListMyAppointments_EmptyReturnsEmptyList(t *testing.T) {
	pool, h := setupPatientTest(t)
	ctx := context.Background()

	ownerID, err := seedCitizenAppUser(ctx, pool)
	if err != nil {
		t.Fatalf("seed owner app_user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM app_users WHERE id = $1`, ownerID)
	})

	actor := identity.Actor{Type: identity.ActorUser, UserID: "test-owner", AppUserID: ownerID}
	rec := dispatchMyAppointments(h, actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rows := decodeAppointmentList(t, rec)
	if rows == nil {
		t.Fatalf("expected an empty array (not null), got nil")
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 appointments, got %d", len(rows))
	}
}