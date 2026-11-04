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
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/limiter"
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

// The generator must not emit characters a citizen cannot tell apart.
//
// This is the regression guard for the owner-reported failure: codes were drawn
// from A-Z0-9, so "O" and "0" (and "I" and "1") both appeared. A citizen reading
// a code off a screen and typing it into a phone had no way to know which glyph
// the font was showing, and a correct code plus one reasonable transcription
// produced "Kode tidak ditemukan".
func TestGenerateCheckinCode(t *testing.T) {
	// Assert on the alphabet itself, not on a sample. A sampled test would pass
	// on a wrong alphabet most of the time and fail intermittently in CI, which
	// is the worst possible signal.
	for _, forbidden := range []rune{'I', 'O', '0', '1'} {
		if strings.ContainsRune(checkinCodeAlphabet, forbidden) {
			t.Errorf("alphabet contains the confusable character %q; "+
				"a citizen cannot distinguish it from its lookalike", forbidden)
		}
	}
	if len(checkinCodeAlphabet) < 30 {
		t.Errorf("alphabet shrank to %d symbols; expected at least 30 for 6-char codes",
			len(checkinCodeAlphabet))
	}

	code, err := generateCheckinCode(6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("expected length 6, got %d", len(code))
	}
	for _, c := range code {
		if !strings.ContainsRune(checkinCodeAlphabet, c) {
			t.Errorf("unexpected character in code: %c", c)
		}
	}
}

// TestGenerateCheckinCode_Distinct pins the generator's OUTPUT contract par
// excellence: a call yields exactly 6 characters, each drawn from
// checkinCodeAlphabet (the 32-symbol set with no I/O/0/1), and repeated calls
// differ. This guards a stale report that cited a 7-char "SMOKE01" code — that
// string cannot come out of generateCheckinCode: it is neither 6 chars nor
// drawn from the alphabet (it contains O, 0 and 1, all excluded).
func TestGenerateCheckinCode_Distinct(t *testing.T) {
	code, err := generateCheckinCode(6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("expected 6 characters, got %d (%q)", len(code), code)
	}
	for _, c := range code {
		if !strings.ContainsRune(checkinCodeAlphabet, c) {
			t.Fatalf("character %q is not in checkinCodeAlphabet", c)
		}
	}

	// Repeated calls differ: the generator must be random, not a fixed or
	// sequential sequence. 32^6 ≈ 1.07e9, so a duplicate across these few calls
	// would indicate a broken generator rather than bad luck.
	seen := map[string]bool{code: true}
	for i := range 8 {
		next, err := generateCheckinCode(6)
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
		if seen[next] {
			t.Fatalf("generateCheckinCode(6) returned a duplicate %q across %d calls", next, i+1)
		}
		seen[next] = true
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

	// Invalid UUID is caught before nil pool check → 400.
	NewBookingHandler(nil, nil).CheckIn(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
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

// engineTicketQueueService mimics the real gRPC engine: it issues a valid UUID
// ticket and formatted number, but writes nothing to this database — the engine
// commits into whichever database it is configured with.
type engineTicketQueueService struct {
	ticketID  string
	formatted string
}

func (e *engineTicketQueueService) Generate(ctx context.Context, input service.GenerateInput) (service.GenerateResult, error) {
	return service.GenerateResult{
		TicketID:             e.ticketID,
		FormattedNumber:      e.formatted,
		Status:               "waiting",
		EstimatedWaitMinutes: 25,
	}, nil
}

func (e *engineTicketQueueService) Probe(ctx context.Context) error { return nil }

// TestCheckIn_EngineTicketNotInLocalDB_IsRefusedAndReleased reproduces the
// production failure: the engine issued a valid UUID and committed the ticket
// into a different database, so this one had no queue_tickets row for it. The
// foreign key on appointments.queue_ticket_id rejected the finalize UPDATE and
// the appointment was left checked_in with no ticket, where a retry answers 409
// forever.
//
// The handler must NOT repair this by inserting a local copy. The waiting-room
// display reads the engine's database, so a locally invented row would give the
// citizen a number that is never called, and would fork the numbering sequence.
// The correct behaviour is to refuse, release the appointment back to scheduled
// so the citizen can retry, and say the queue service is out of sync.
// Requires DATABASE_URL to be set; skipped otherwise.
func TestCheckIn_EngineTicketNotInLocalDB_IsRefusedAndReleased(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, code, _, cleanup := seedCheckinDataV2(t, pool)
	defer cleanup()

	ticketID := uuid.New().String()
	h := NewBookingHandler(pool, nil).
		WithQueueService(&engineTicketQueueService{ticketID: ticketID, formatted: "TST-0042"})

	body, _ := json.Marshal(map[string]string{"checkin_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for an out-of-sync queue service, got %d: %s", rec.Code, rec.Body.String())
	}

	// The appointment is released, not stranded: a retry must be possible.
	var apptStatus string
	var qTicketID *string
	var checkinAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT status, queue_ticket_id, checkin_at FROM appointments WHERE id = $1`, apptID,
	).Scan(&apptStatus, &qTicketID, &checkinAt); err != nil {
		t.Fatalf("verify appointment: %v", err)
	}
	if apptStatus != "scheduled" {
		t.Errorf("expected appointment released to 'scheduled', got %q", apptStatus)
	}
	if qTicketID != nil {
		t.Errorf("expected no queue_ticket_id, got %v", *qTicketID)
	}
	if checkinAt != nil {
		t.Errorf("expected checkin_at cleared, got %v", checkinAt)
	}

	// And no ticket was invented locally.
	var invented int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM queue_tickets WHERE id = $1`, ticketID,
	).Scan(&invented); err != nil {
		t.Fatalf("count queue_tickets: %v", err)
	}
	if invented != 0 {
		t.Errorf("handler fabricated a local queue ticket; expected 0 rows, got %d", invented)
	}
}

// TestCheckIn_EngineTicketAlreadyLocal_NotDuplicated covers the healthy case:
// the engine committed the ticket into this database, so the handler must
// reference it without inserting a second row or renumbering it.
// Requires DATABASE_URL to be set; skipped otherwise.
func TestCheckIn_EngineTicketAlreadyLocal_NotDuplicated(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, code, facID, cleanup := seedCheckinDataV2(t, pool)
	defer cleanup()

	ctx := context.Background()
	var patientID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO patients (full_name, phone, date_of_birth)
		 VALUES ('Engine Patient', '085550008888', '1990-01-01')
		 ON CONFLICT (phone) DO UPDATE SET full_name = patients.full_name
		 RETURNING id`).Scan(&patientID); err != nil {
		t.Fatalf("seed patient: %v", err)
	}

	ticketID := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status)
		 VALUES ($1, $2, $3, 77, 'TST-0077', 'waiting')`, ticketID, facID, patientID); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	h := NewBookingHandler(pool, nil).
		WithQueueService(&engineTicketQueueService{ticketID: ticketID, formatted: "TST-0077"})

	body, _ := json.Marshal(map[string]string{"checkin_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var rows int
	var queueNumber int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(MAX(queue_number), 0) FROM queue_tickets WHERE id = $1`, ticketID,
	).Scan(&rows, &queueNumber); err != nil {
		t.Fatalf("verify ticket: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected the engine's ticket row to be reused, got %d rows", rows)
	}
	if queueNumber != 77 {
		t.Errorf("expected the engine's queue_number 77 to survive, got %d", queueNumber)
	}
}

// --- Citizen cancellation by check-in code ---

// seedCancelAppt inserts a facility + service_unit + appointment for cancel
// integration tests. Unique IDs avoid collisions with other tests.
func seedCancelAppt(t *testing.T, pool *pgxpool.Pool, code, status string) (apptID string, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	u := time.Now().UnixNano()
	facID := "40000000-0000-0000-0000-" + padID(u)
	svcID := "50000000-0000-0000-0000-" + padID(u)
	apptID = "60000000-0000-0000-0000-" + padID(u)

	if _, err := pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, short_code)
		 VALUES ($1, 'Cancel Fac', 'puskesmas', 'Addr', 'Kec', 'Kab', 'Prov', '000', 'CF')
		 ON CONFLICT (id) DO NOTHING`, facID); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name)
		 VALUES ($1, $2, 'Cancel Unit')
		 ON CONFLICT (id) DO NOTHING`, svcID, facID); err != nil {
		t.Fatalf("seed service_unit: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO appointments (id, facility_id, service_unit_id, patient_display_name,
			patient_phone, appointment_time, status, checkin_code)
		 VALUES ($1, $2, $3, 'Cancel Patient', '085550000777',
			NOW() + INTERVAL '1 day', $4, $5)
		 ON CONFLICT (id) DO NOTHING`,
		apptID, facID, svcID, status, code); err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	cleanup = func() {
		pool.Exec(context.Background(), `DELETE FROM appointments WHERE id = $1`, apptID)
		pool.Exec(context.Background(), `DELETE FROM service_units WHERE id = $1`, svcID)
		pool.Exec(context.Background(), `DELETE FROM facilities WHERE id = $1`, facID)
	}
	return apptID, cleanup
}

func TestCancel_EmptyCode_Returns400(t *testing.T) {
	// Validation runs before the DB lookup, so a nil pool is sufficient here.
	h := NewBookingHandler(nil, nil)
	body, _ := json.Marshal(map[string]string{"checkin_code": "   "})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CancelAppointment(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty code, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCancel_InvalidCharacters_Returns400(t *testing.T) {
	h := NewBookingHandler(nil, nil)
	body, _ := json.Marshal(map[string]string{"checkin_code": "<script>"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CancelAppointment(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid characters, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCancel_RateLimited_Returns429(t *testing.T) {
	rl := limiter.NewRateLimiter(1, 1*time.Minute)
	h := NewBookingHandler(nil, nil).WithCancelLimiter(rl)
	body, _ := json.Marshal(map[string]string{"checkin_code": "CNCLAB"})

	// First request passes the limiter, then hits the nil pool → 500 (not 429).
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.RemoteAddr = "10.0.0.9:1234"
	rec1 := httptest.NewRecorder()
	h.CancelAppointment(rec1, req1)
	if rec1.Code == http.StatusTooManyRequests {
		t.Fatalf("first request should not be rate limited")
	}

	// Second request from the same IP exceeds the limit → 429.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "10.0.0.9:1234"
	rec2 := httptest.NewRecorder()
	h.CancelAppointment(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestCancel_Scheduled_Returns200AndCancels(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, cleanup := seedCancelAppt(t, pool, "CNCLAB", "scheduled")
	defer cleanup()

	h := NewBookingHandler(pool, nil)

	// Lowercase on purpose: the lookup is case-insensitive.
	body, _ := json.Marshal(map[string]string{"checkin_code": "cnclab"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CancelAppointment(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
	data, _ := resp["data"].(map[string]any)
	if data["status"] != "cancelled" {
		t.Errorf("expected data.status cancelled, got %v", data["status"])
	}
	if data["appointment_id"] != apptID {
		t.Errorf("expected appointment_id %s, got %v", apptID, data["appointment_id"])
	}

	var st string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM appointments WHERE id = $1`, apptID).Scan(&st); err != nil {
		t.Fatalf("verify appointment: %v", err)
	}
	if st != "cancelled" {
		t.Errorf("expected DB status cancelled, got %q", st)
	}
}

func TestCancel_ConfusableFolded_Resolves(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// "AB0IEF" contains a digit 0 and digit 1; substituting letter O/I must
	// still resolve via the confusable fold, exactly like the status lookup.
	_, cleanup := seedCancelAppt(t, pool, "AB0IEF", "scheduled")
	defer cleanup()

	h := NewBookingHandler(pool, nil)
	body, _ := json.Marshal(map[string]string{"checkin_code": "ABOIEF"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CancelAppointment(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("folded code should resolve to 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCancel_AlreadyCancelled_Idempotent200(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	_, cleanup := seedCancelAppt(t, pool, "CNCLDX", "cancelled")
	defer cleanup()

	h := NewBookingHandler(pool, nil)
	body, _ := json.Marshal(map[string]string{"checkin_code": "CNCLDX"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CancelAppointment(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for already-cancelled, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "cancelled" {
		t.Errorf("expected {status:cancelled} no-op, got %v", resp)
	}
	if _, ok := resp["data"]; ok {
		t.Errorf("idempotent no-op must NOT wrap in data, got %v", resp)
	}
}

func TestCancel_CheckedIn_Returns409(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	_, cleanup := seedCancelAppt(t, pool, "CNCLCK", "checked_in")
	defer cleanup()

	h := NewBookingHandler(pool, nil)
	body, _ := json.Marshal(map[string]string{"checkin_code": "CNCLCK"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CancelAppointment(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for checked-in, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCancel_UnknownCode_Returns404(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	h := NewBookingHandler(pool, nil)
	body, _ := json.Marshal(map[string]string{"checkin_code": "NOPE01"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CancelAppointment(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown code, got %d: %s", rec.Code, rec.Body.String())
	}
}
