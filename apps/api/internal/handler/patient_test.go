package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/limiter"
)

// Deterministic test UUIDs — no real patient data.
const (
	testFacID         = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	testServiceUnitID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	testPatientID     = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	testAppointmentID = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	testQueueTicketID = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	testCheckinCode   = "TESTC1"
	testFormattedNum  = "PATPORTAL-0001"
)

// setupPatientTest returns a pool + handler, or skips if SIGAP_DATABASE_URL is missing.
func setupPatientTest(t *testing.T) (*pgxpool.Pool, *PatientHandler) {
	t.Helper()
	dbURL := os.Getenv("SIGAP_DATABASE_URL")
	if dbURL == "" {
		t.Skip("SIGAP_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rl := limiter.NewRateLimiter(30, 1*time.Minute)
	h := NewPatientHandler(pool, rl)
	return pool, h
}

// seedFacility inserts a minimal facility row for testing.
func seedFacility(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi,
			phone, total_beds, available_beds, short_code)
		 VALUES ($1, 'Faskes Test', 'puskesmas', 'Jl. Test No.1', 'Kec. Test',
			'Kab. Test', 'Prov. Test', '021-1234567', 10, 5, 'FTST')
		 ON CONFLICT (id) DO NOTHING`,
		testFacID)
	return err
}

// seedServiceUnit inserts a minimal service_unit row for testing.
func seedServiceUnit(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name)
		 VALUES ($1, $2, 'Poli Umum Test')
		 ON CONFLICT (id) DO NOTHING`,
		testServiceUnitID, testFacID)
	return err
}

// seedPatient inserts a minimal patient row for testing.
func seedPatient(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO patients (id, full_name, phone)
		 VALUES ($1, 'Pasien Demo', '085550000001')
		 ON CONFLICT (id) DO NOTHING`,
		testPatientID)
	return err
}

// seedAppointment inserts an appointment with the test checkin_code.
func seedAppointment(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO appointments (id, facility_id, service_unit_id, patient_display_name,
			patient_phone, appointment_time, status, checkin_code)
		 VALUES ($1, $2, $3, 'Pasien Demo', '085550000001',
			NOW() + INTERVAL '1 day', 'scheduled', $4)
		 ON CONFLICT (id) DO NOTHING`,
		testAppointmentID, testFacID, testServiceUnitID, testCheckinCode)
	return err
}

// seedQueueTicket inserts a queue_ticket with the test formatted_number.
func seedQueueTicket(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status)
		 VALUES ($1, $2, $3, 1, $4, 'waiting')
		 ON CONFLICT (id) DO NOTHING`,
		testQueueTicketID, testFacID, testPatientID, testFormattedNum)
	return err
}

// cleanupTestData removes seeded rows in FK-safe order.
func cleanupTestData(ctx context.Context, pool *pgxpool.Pool) {
	pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, testAppointmentID)
	pool.Exec(ctx, `DELETE FROM queue_tickets WHERE id = $1`, testQueueTicketID)
	pool.Exec(ctx, `DELETE FROM service_units WHERE id = $1`, testServiceUnitID)
	pool.Exec(ctx, `DELETE FROM patients WHERE id = $1`, testPatientID)
	pool.Exec(ctx, `DELETE FROM facilities WHERE id = $1`, testFacID)
}

func TestPatientStatusLookup_SuccessByCheckinCode(t *testing.T) {
	pool, h := setupPatientTest(t)
	ctx := context.Background()

	if err := seedFacility(ctx, pool); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if err := seedServiceUnit(ctx, pool); err != nil {
		t.Fatalf("seed service_unit: %v", err)
	}
	if err := seedAppointment(ctx, pool); err != nil {
		t.Fatalf("seed appointment: %v", err)
	}
	t.Cleanup(func() { cleanupTestData(context.Background(), pool) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status?code="+testCheckinCode, nil)
	rec := httptest.NewRecorder()
	h.PatientStatusLookup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	success, ok := body["success"].(bool)
	if !ok || !success {
		t.Fatalf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", body["data"])
	}

	if data["facility_name"] != "Faskes Test" {
		t.Errorf("expected facility_name 'Faskes Test', got %v", data["facility_name"])
	}
	if data["appointment_status"] != "scheduled" {
		t.Errorf("expected appointment_status 'scheduled', got %v", data["appointment_status"])
	}
	if data["found_by"] != "checkin_code" {
		t.Errorf("expected found_by 'checkin_code', got %v", data["found_by"])
	}
	if data["checkin_status"] != "not_checked_in" {
		t.Errorf("expected checkin_status 'not_checked_in', got %v", data["checkin_status"])
	}
}

func TestPatientStatusLookup_SuccessByFormattedNumber(t *testing.T) {
	pool, h := setupPatientTest(t)
	ctx := context.Background()

	if err := seedFacility(ctx, pool); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if err := seedPatient(ctx, pool); err != nil {
		t.Fatalf("seed patient: %v", err)
	}
	if err := seedQueueTicket(ctx, pool); err != nil {
		t.Fatalf("seed queue_ticket: %v", err)
	}
	t.Cleanup(func() { cleanupTestData(context.Background(), pool) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status?code="+testFormattedNum, nil)
	rec := httptest.NewRecorder()
	h.PatientStatusLookup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	success, ok := body["success"].(bool)
	if !ok || !success {
		t.Fatalf("expected success=true, got %v", body["success"])
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", body["data"])
	}

	if data["facility_name"] != "Faskes Test" {
		t.Errorf("expected facility_name 'Faskes Test', got %v", data["facility_name"])
	}
	if data["found_by"] != "formatted_number" {
		t.Errorf("expected found_by 'formatted_number', got %v", data["found_by"])
	}
}

func TestPatientStatusLookup_NotFound(t *testing.T) {
	_, h := setupPatientTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status?code=ZZZZZXXXXX999", nil)
	rec := httptest.NewRecorder()
	h.PatientStatusLookup(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
	if body["error"] != "Kode tidak ditemukan." {
		t.Errorf("expected error 'Kode tidak ditemukan.', got %v", body["error"])
	}
}

func TestPatientStatusLookup_EmptyCode(t *testing.T) {
	_, h := setupPatientTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status", nil)
	rec := httptest.NewRecorder()
	h.PatientStatusLookup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
	if body["error"] != "Parameter 'code' diperlukan." {
		t.Errorf("expected error 'Parameter 'code' diperlukan.', got %v", body["error"])
	}
}

func TestPatientStatusLookup_NoPIIInResponse(t *testing.T) {
	pool, h := setupPatientTest(t)
	ctx := context.Background()

	if err := seedFacility(ctx, pool); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if err := seedServiceUnit(ctx, pool); err != nil {
		t.Fatalf("seed service_unit: %v", err)
	}
	if err := seedAppointment(ctx, pool); err != nil {
		t.Fatalf("seed appointment: %v", err)
	}
	t.Cleanup(func() { cleanupTestData(context.Background(), pool) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status?code="+testCheckinCode, nil)
	rec := httptest.NewRecorder()
	h.PatientStatusLookup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	piiFields := []string{
		"patient_phone",
		"patient_display_name",
		"patient_id",
		"recipient_contact",
	}
	for _, field := range piiFields {
		if strings.Contains(body, field) {
			t.Errorf("response must NOT contain PII field %q, but found in body: %s", field, body)
		}
	}
}

func TestPatientStatusLookup_CodeTooLong(t *testing.T) {
	_, h := setupPatientTest(t)

	longCode := strings.Repeat("A", 65)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status?code="+longCode, nil)
	rec := httptest.NewRecorder()
	h.PatientStatusLookup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["error"] != "Kode tidak valid." {
		t.Errorf("expected error 'Kode tidak valid.', got %v", body["error"])
	}
}

func TestPatientStatusLookup_InvalidCharacters(t *testing.T) {
	_, h := setupPatientTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status?code=%3Cscript%3E", nil)
	rec := httptest.NewRecorder()
	h.PatientStatusLookup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["error"] != "Kode tidak valid." {
		t.Errorf("expected error 'Kode tidak valid.', got %v", body["error"])
	}
}

func TestPatientStatusLookup_RateLimited(t *testing.T) {
	dbURL := os.Getenv("SIGAP_DATABASE_URL")
	if dbURL == "" {
		t.Skip("SIGAP_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Limit=1: first request allowed, second denied.
	rl := limiter.NewRateLimiter(1, 1*time.Minute)
	h := NewPatientHandler(pool, rl)

	// First request — should be allowed (may return 404 or 200 depending on DB state, but not 429).
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status?code=RATETEST1", nil)
	rec1 := httptest.NewRecorder()
	h.PatientStatusLookup(rec1, req1)

	if rec1.Code == http.StatusTooManyRequests {
		t.Fatalf("first request should not be rate limited, got 429")
	}

	// Second request from same IP — should be rate limited.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/patient/status?code=RATETEST1", nil)
	rec2 := httptest.NewRecorder()
	h.PatientStatusLookup(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["error"] != "Terlalu banyak permintaan. Coba lagi nanti." {
		t.Errorf("expected rate limit error, got %v", body["error"])
	}
}
