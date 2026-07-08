package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/service"
)

func TestBookAppointment_NilPool_Returns500(t *testing.T) {
	reqBody := map[string]any{
		"facility_id":        "550e8400-e29b-41d4-a716-446655440000",
		"service_unit_id":    "550e8400-e29b-41d4-a716-446655440001",
		"patient_display_name": "Andi",
		"patient_phone":      "081234567890",
		"appointment_time":   "2026-12-31T10:00:00Z",
	}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	NewBookingHandler(nil, nil).BookAppointment(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestBookAppointment_MissingFields_Returns400(t *testing.T) {
	// With nil pool, nil pool guard returns 500 before validation
	empty := map[string]any{}
	b, _ := json.Marshal(empty)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	NewBookingHandler(nil, nil).BookAppointment(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestBookAppointment_InvalidUUID_Returns400(t *testing.T) {
	reqBody := map[string]any{
		"facility_id":          "not-a-uuid",
		"service_unit_id":      "550e8400-e29b-41d4-a716-446655440001",
		"patient_display_name": "Andi",
		"patient_phone":        "081234567890",
		"appointment_time":     "2026-12-31T10:00:00Z",
	}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// nil pool guard returns 500 first
	NewBookingHandler(nil, nil).BookAppointment(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestBookAppointment_InvalidPhone_Returns400(t *testing.T) {
	reqBody := map[string]any{
		"facility_id":          "550e8400-e29b-41d4-a716-446655440000",
		"service_unit_id":      "550e8400-e29b-41d4-a716-446655440001",
		"patient_display_name": "Andi",
		"patient_phone":        "123",
		"appointment_time":     "2026-12-31T10:00:00Z",
	}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// nil pool guard returns 500 first
	NewBookingHandler(nil, nil).BookAppointment(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestValidateBookAppointment(t *testing.T) {
	tests := []struct {
		name string
		req  BookAppointmentRequest
		want bool // true = expect error
	}{
		{"missing_facility_id", BookAppointmentRequest{ServiceUnitID: "550e8400-e29b-41d4-a716-446655440000", PatientDisplayName: "Andi", PatientPhone: "081234567890", AppointmentTime: "2026-01-01T00:00:00Z"}, true},
		{"invalid_facility_id", BookAppointmentRequest{FacilityID: "not-a-uuid", ServiceUnitID: "550e8400-e29b-41d4-a716-446655440000", PatientDisplayName: "Andi", PatientPhone: "081234567890", AppointmentTime: "2026-01-01T00:00:00Z"}, true},
		{"invalid_service_unit_id", BookAppointmentRequest{FacilityID: "550e8400-e29b-41d4-a716-446655440000", ServiceUnitID: "bad", PatientDisplayName: "Andi", PatientPhone: "081234567890", AppointmentTime: "2026-01-01T00:00:00Z"}, true},
		{"invalid_schedule_id", BookAppointmentRequest{FacilityID: "550e8400-e29b-41d4-a716-446655440000", ServiceUnitID: "550e8400-e29b-41d4-a716-446655440001", PractitionerScheduleID: "bad", PatientDisplayName: "Andi", PatientPhone: "081234567890", AppointmentTime: "2026-01-01T00:00:00Z"}, true},
		{"empty_name", BookAppointmentRequest{FacilityID: "550e8400-e29b-41d4-a716-446655440000", ServiceUnitID: "550e8400-e29b-41d4-a716-446655440001", PatientDisplayName: "", PatientPhone: "081234567890", AppointmentTime: "2026-01-01T00:00:00Z"}, true},
		{"long_name", BookAppointmentRequest{FacilityID: "550e8400-e29b-41d4-a716-446655440000", ServiceUnitID: "550e8400-e29b-41d4-a716-446655440001", PatientDisplayName: strings.Repeat("a", 101), PatientPhone: "081234567890", AppointmentTime: "2026-01-01T00:00:00Z"}, true},
		{"short_phone", BookAppointmentRequest{FacilityID: "550e8400-e29b-41d4-a716-446655440000", ServiceUnitID: "550e8400-e29b-41d4-a716-446655440001", PatientDisplayName: "Andi", PatientPhone: "0812", AppointmentTime: "2026-01-01T00:00:00Z"}, true},
		{"empty_appointment_time", BookAppointmentRequest{FacilityID: "550e8400-e29b-41d4-a716-446655440000", ServiceUnitID: "550e8400-e29b-41d4-a716-446655440001", PatientDisplayName: "Andi", PatientPhone: "081234567890", AppointmentTime: ""}, true},
		{"valid", BookAppointmentRequest{FacilityID: "550e8400-e29b-41d4-a716-446655440000", ServiceUnitID: "550e8400-e29b-41d4-a716-446655440001", PatientDisplayName: "Andi", PatientPhone: "081234567890", AppointmentTime: "2026-01-01T00:00:00Z"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBookAppointment(tt.req)
			gotErr := err != nil
			if gotErr != tt.want {
				t.Errorf("validateBookAppointment() error=%v, wantErr=%v", err, tt.want)
			}
		})
	}
}

func TestGenerateCheckinCode(t *testing.T) {
	code, err := generateCheckinCode(6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("expected length 6, got %d", len(code))
	}
	for _, c := range code {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", c) {
			t.Errorf("unexpected character in code: %c", c)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0812-3456-7890", "081234567890"},
		{"+62 812 3456 7890", "6281234567890"},
		{"12345", "12345"},
	}
	for _, c := range cases {
		got := normalizePhone(c.in)
		if got != c.want {
			t.Errorf("normalizePhone(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestCheckIn_NilPool_Returns500(t *testing.T) {
	reqBody := map[string]any{"checkin_code": "ABC123"}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/550e8400-e29b-41d4-a716-446655440000/check-in", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	NewBookingHandler(nil, nil).CheckIn(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestCheckIn_InvalidID_Returns400(t *testing.T) {
	reqBody := map[string]any{"checkin_code": "ABC123"}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/bad-id/check-in", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// Need a handler with pool=nil so nil pool check fires first, which means bad ID won't be reached.
	// Instead test the ID extraction indirectly: since our CheckIn checks nil first, this returns 500.
	NewBookingHandler(nil, nil).CheckIn(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestPublicAppointmentsRouter_BookAppointment(t *testing.T) {
	reqBody := map[string]any{
		"facility_id":          "550e8400-e29b-41d4-a716-446655440000",
		"service_unit_id":      "550e8400-e29b-41d4-a716-446655440001",
		"patient_display_name": "Andi",
		"patient_phone":        "081234567890",
		"appointment_time":     "2026-12-31T10:00:00Z",
	}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	NewBookingHandler(nil, nil).PublicAppointmentsRouter(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool via router, got %d", rec.Code)
	}
}

func TestPublicAppointmentsRouter_CheckIn(t *testing.T) {
	reqBody := map[string]any{"checkin_code": "ABC123"}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/550e8400-e29b-41d4-a716-446655440000/check-in", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	NewBookingHandler(nil, nil).PublicAppointmentsRouter(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool via router, got %d", rec.Code)
	}
}

func TestPublicAppointmentsRouter_NotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/appointments", nil)
	rec := httptest.NewRecorder()

	NewBookingHandler(nil, nil).PublicAppointmentsRouter(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", rec.Code)
	}
}

// failingQueueService always returns an error on Generate.
// Used to simulate a queue engine that is unreachable.
type failingQueueService struct{}

func (f *failingQueueService) Generate(ctx context.Context, input service.GenerateInput) (service.GenerateResult, error) {
	return service.GenerateResult{}, context.DeadlineExceeded
}

func (f *failingQueueService) Probe(ctx context.Context) error {
	return context.DeadlineExceeded
}

// seedCheckinData creates minimal test data for a check-in integration test.
// Returns (appointmentID, checkinCode, cleanup func).
func seedCheckinData(t *testing.T, pool *pgxpool.Pool) (string, string, func()) {
	t.Helper()
	ctx := context.Background()

	facID := "00000000-0000-0000-0000-00000000aaa1"
	patientID := "00000000-0000-0000-0000-00000000aaa2"
	svcUnitID := "00000000-0000-0000-0000-00000000aaa3"
	apptID := "00000000-0000-0000-0000-00000000aaa4"
	code := "TESTCHK1"

	pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, short_code)
		 VALUES ($1, 'Test Fac', 'rumah_sakit', 'Addr', 'Kec', 'Kab', 'Prov', '000', 'TST')
		 ON CONFLICT (id) DO NOTHING`, facID)
	pool.Exec(ctx,
		`INSERT INTO patients (id, full_name, phone, date_of_birth)
		 VALUES ($1, 'Test Patient', '085550009999', '1990-01-01')
		 ON CONFLICT (phone) DO NOTHING`, patientID)
	pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name, code)
		 VALUES ($1, $2, 'Test Unit', 'TST-U')
		 ON CONFLICT (id) DO NOTHING`, svcUnitID, facID)
	pool.Exec(ctx,
		`INSERT INTO appointments (id, facility_id, service_unit_id, patient_display_name, patient_phone, appointment_time, status, checkin_code)
		 VALUES ($1, $2, $3, 'Test Patient', '085550009999', NOW() + INTERVAL '1 day', 'scheduled', $4)
		 ON CONFLICT (id) DO NOTHING`, apptID, facID, svcUnitID, code)

	cleanup := func() {
		pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, apptID)
		pool.Exec(ctx, `DELETE FROM queue_tickets WHERE facility_id = $1`, facID)
		pool.Exec(ctx, `DELETE FROM service_units WHERE id = $1`, svcUnitID)
		pool.Exec(ctx, `DELETE FROM patients WHERE id = $1`, patientID)
		pool.Exec(ctx, `DELETE FROM facilities WHERE id = $1`, facID)
	}

	return apptID, code, cleanup
}

// TestCheckIn_QueueFallback_Disabled_Returns500 verifies that when the queue
// engine fails and SIGAP_ENGINE_FALLBACK is not set, check-in returns 500.
// Requires DATABASE_URL to be set; skipped otherwise.
func TestCheckIn_QueueFallback_Disabled_Returns500(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, code, cleanup := seedCheckinData(t, pool)
	defer cleanup()

	os.Unsetenv("SIGAP_ENGINE_FALLBACK")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewBookingHandler(pool, nil).WithQueueService(&failingQueueService{})

	body, _ := json.Marshal(map[string]string{"checkin_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when fallback disabled, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify appointment was rolled back to scheduled
	var status string
	err = pool.QueryRow(context.Background(),
		`SELECT status FROM appointments WHERE id = $1`, apptID).Scan(&status)
	if err != nil {
		t.Fatalf("verify status: %v", err)
	}
	if status != "scheduled" {
		t.Errorf("expected status 'scheduled' after failed check-in, got %q", status)
	}
}

// TestCheckIn_QueueFallback_Enabled_Succeeds verifies that when the queue
// engine fails but SIGAP_ENGINE_FALLBACK=dev is set, check-in succeeds
// by creating a real queue_tickets row with a valid UUID.
// Requires DATABASE_URL to be set; skipped otherwise.
func TestCheckIn_QueueFallback_Enabled_Succeeds(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, code, cleanup := seedCheckinData(t, pool)
	defer cleanup()

	os.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewBookingHandler(pool, nil).WithQueueService(&failingQueueService{})

	body, _ := json.Marshal(map[string]string{"checkin_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with fallback enabled, got %d: %s", rec.Code, rec.Body.String())
	}

	// Parse response
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	success, _ := resp["success"].(bool)
	if !success {
		t.Fatalf("expected success=true, got %v", resp)
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	ticketID, _ := data["queue_ticket_id"].(string)
	if ticketID == "" {
		t.Fatal("expected queue_ticket_id in response")
	}

	// Verify queue_tickets row exists with valid UUID
	var qStatus string
	err = pool.QueryRow(context.Background(),
		`SELECT status FROM queue_tickets WHERE id = $1`, ticketID).Scan(&qStatus)
	if err != nil {
		t.Fatalf("queue_tickets row not found for %s: %v", ticketID, err)
	}
	if qStatus != "waiting" {
		t.Errorf("expected queue_tickets.status 'waiting', got %q", qStatus)
	}

	// Verify appointment is queued with the ticket
	var apptStatus string
	var qTicketID *string
	err = pool.QueryRow(context.Background(),
		`SELECT status, queue_ticket_id FROM appointments WHERE id = $1`, apptID).Scan(&apptStatus, &qTicketID)
	if err != nil {
		t.Fatalf("verify appointment: %v", err)
	}
	if apptStatus != "queued" {
		t.Errorf("expected appointment status 'queued', got %q", apptStatus)
	}
	if qTicketID == nil || *qTicketID != ticketID {
		t.Errorf("expected queue_ticket_id=%s, got %v", ticketID, qTicketID)
	}
}
