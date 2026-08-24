package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// --- Unit tests (no database required) ---

func TestCheckIn_RateLimiter_BlocksAfterThreshold(t *testing.T) {
	rl := limiter.NewRateLimiter(3, 1*time.Minute) // 3 attempts per minute
	h := NewBookingHandler(nil, nil).WithCheckinLimiter(rl)

	apptID := "550e8400-e29b-41d4-a716-446655440000"
	body, _ := json.Marshal(map[string]string{"checkin_code": "ABC123"})

	// First 3 attempts should pass rate limiter (then hit nil pool → 500).
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.100:12345"
		rec := httptest.NewRecorder()

		h.CheckIn(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("attempt %d: expected 500 (nil pool), got %d", i+1, rec.Code)
		}
	}

	// 4th attempt should be rate-limited → 429.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.100:12345"
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after threshold, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCheckIn_RateLimiter_DifferentIPs_NotBlocked(t *testing.T) {
	rl := limiter.NewRateLimiter(2, 1*time.Minute)
	h := NewBookingHandler(nil, nil).WithCheckinLimiter(rl)

	apptID := "550e8400-e29b-41d4-a716-446655440000"
	body, _ := json.Marshal(map[string]string{"checkin_code": "ABC123"})

	// IP1 uses 2 attempts (at limit).
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		h.CheckIn(rec, req)
	}

	// IP1 should now be rate-limited.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.CheckIn(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 expected 429, got %d", rec.Code)
	}

	// IP2 should NOT be blocked.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "10.0.0.2:5678"
	rec2 := httptest.NewRecorder()
	h.CheckIn(rec2, req2)
	if rec2.Code == http.StatusTooManyRequests {
		t.Errorf("IP2 should not be blocked, got 429")
	}
}

func TestCheckIn_RateLimiter_DifferentAppointments_NotBlocked(t *testing.T) {
	rl := limiter.NewRateLimiter(1, 1*time.Minute)
	h := NewBookingHandler(nil, nil).WithCheckinLimiter(rl)

	body, _ := json.Marshal(map[string]string{"checkin_code": "ABC123"})

	// Appointment 1 uses its attempt.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/550e8400-e29b-41d4-a716-446655440000/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.CheckIn(rec, req)

	// Appointment 2 from same IP should NOT be blocked.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/550e8400-e29b-41d4-a716-446655440001/check-in", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "10.0.0.1:1234"
	rec2 := httptest.NewRecorder()
	h.CheckIn(rec2, req2)
	if rec2.Code == http.StatusTooManyRequests {
		t.Errorf("different appointment should not be blocked by same IP's attempts")
	}
}

func TestCheckIn_NilLimiter_PassesThrough(t *testing.T) {
	// When checkinLimiter is nil, no rate limiting is applied.
	h := NewBookingHandler(nil, nil) // no checkinLimiter

	apptID := "550e8400-e29b-41d4-a716-446655440000"
	body, _ := json.Marshal(map[string]string{"checkin_code": "ABC123"})

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		h.CheckIn(rec, req)
		// All should hit nil pool → 500, never 429.
		if rec.Code == http.StatusTooManyRequests {
			t.Errorf("attempt %d: should not get 429 without limiter", i+1)
		}
	}
}

func TestCheckIn_InvalidUUID_Returns400(t *testing.T) {
	// Invalid UUID is caught before nil pool check → 400.
	h := NewBookingHandler(nil, nil)
	body, _ := json.Marshal(map[string]string{"checkin_code": "ABC123"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/not-a-uuid/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

// --- Integration tests (require DATABASE_URL) ---

// seedCheckinDataV2 creates test data for check-in integration tests.
// Uses unique IDs to avoid conflicts with other tests.
func seedCheckinDataV2(t *testing.T, pool *pgxpool.Pool) (apptID, code, facID string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	uniqueID := time.Now().UnixNano()
	facIDStr := "10000000-0000-0000-0000-" + padID(uniqueID)
	svcUnitID := "20000000-0000-0000-0000-" + padID(uniqueID)
	apptIDStr := "30000000-0000-0000-0000-" + padID(uniqueID)
	codeStr := "CHK" + padID(uniqueID)[:3]

	pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, short_code)
		 VALUES ($1, 'Test Fac V2', 'rumah_sakit', 'Addr', 'Kec', 'Kab', 'Prov', '000', 'TS2')
		 ON CONFLICT (id) DO NOTHING`, facIDStr)
	pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name, code)
		 VALUES ($1, $2, 'Test Unit V2', 'TSU2')
		 ON CONFLICT (id) DO NOTHING`, svcUnitID, facIDStr)
	pool.Exec(ctx,
		`INSERT INTO appointments (id, facility_id, service_unit_id, patient_display_name, patient_phone, appointment_time, status, checkin_code)
		 VALUES ($1, $2, $3, 'Test Patient V2', '085550008888', NOW() + INTERVAL '1 day', 'scheduled', $4)
		 ON CONFLICT (id) DO NOTHING`, apptIDStr, facIDStr, svcUnitID, codeStr)

	cleanupFn := func() {
		pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, apptIDStr)
		pool.Exec(ctx, `DELETE FROM queue_tickets WHERE facility_id = $1`, facIDStr)
		pool.Exec(ctx, `DELETE FROM service_units WHERE id = $1`, svcUnitID)
		pool.Exec(ctx, `DELETE FROM facilities WHERE id = $1`, facIDStr)
	}

	return apptIDStr, codeStr, facIDStr, cleanupFn
}

func padID(n int64) string {
	s := make([]byte, 12)
	for i := 11; i >= 0; i-- {
		s[i] = byte('0' + n%10)
		n /= 10
	}
	return string(s)
}

// TestCheckIn_AtomicTransition_Success verifies basic check-in succeeds.
func TestCheckIn_AtomicTransition_Success(t *testing.T) {
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

	os.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewBookingHandler(pool, nil).
		WithQueueService(&failingQueueService{}).
		WithCheckinLimiter(limiter.NewRateLimiter(10, 1*time.Minute))

	body, _ := json.Marshal(map[string]string{"checkin_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["status"] != "queued" {
		t.Errorf("expected status 'queued', got %v", data["status"])
	}
}

// TestCheckIn_AtomicTransition_Idempotency verifies second attempt returns 409.
func TestCheckIn_AtomicTransition_Idempotency(t *testing.T) {
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

	os.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewBookingHandler(pool, nil).
		WithQueueService(&failingQueueService{}).
		WithCheckinLimiter(limiter.NewRateLimiter(10, 1*time.Minute))

	body, _ := json.Marshal(map[string]string{"checkin_code": code})

	// First attempt: success.
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.RemoteAddr = "10.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	h.CheckIn(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first attempt: expected 200, got %d: %s", rec1.Code, rec1.Body.String())
	}

	// Second attempt: conflict.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "10.0.0.1:1234"
	rec2 := httptest.NewRecorder()
	h.CheckIn(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Errorf("second attempt: expected 409, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

// TestCheckIn_AtomicTransition_WrongCode verifies wrong code returns 401.
func TestCheckIn_AtomicTransition_WrongCode(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, _, _, cleanup := seedCheckinDataV2(t, pool)
	defer cleanup()

	h := NewBookingHandler(pool, nil).
		WithQueueService(&failingQueueService{}).
		WithCheckinLimiter(limiter.NewRateLimiter(10, 1*time.Minute))

	body, _ := json.Marshal(map[string]string{"checkin_code": "WRONG1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong code, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify appointment is still 'scheduled'.
	var status string
	pool.QueryRow(context.Background(), `SELECT status FROM appointments WHERE id = $1`, apptID).Scan(&status)
	if status != "scheduled" {
		t.Errorf("appointment should still be 'scheduled', got %q", status)
	}
}

// TestCheckIn_AtomicTransition_NonexistentAppointment verifies 404 for bad ID.
func TestCheckIn_AtomicTransition_NonexistentAppointment(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	h := NewBookingHandler(pool, nil).
		WithQueueService(&failingQueueService{}).
		WithCheckinLimiter(limiter.NewRateLimiter(10, 1*time.Minute))

	fakeID := "99999999-9999-9999-9999-999999999999"
	body, _ := json.Marshal(map[string]string{"checkin_code": "ABC123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+fakeID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent appointment, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCheckIn_ConcurrentAttempts_ExactlyOneWins verifies that concurrent
// check-in attempts against the same appointment result in exactly one
// success.  This is the core AUDIT-1004 regression test.
func TestCheckIn_ConcurrentAttempts_ExactlyOneWins(t *testing.T) {
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

	os.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	// Use a high rate limit so it doesn't interfere with concurrency test.
	h := NewBookingHandler(pool, nil).
		WithQueueService(&failingQueueService{}).
		WithCheckinLimiter(limiter.NewRateLimiter(100, 1*time.Minute))

	body, _ := json.Marshal(map[string]string{"checkin_code": code})

	const numAttempts = 10
	var successCount atomic.Int32
	var conflictCount atomic.Int32
	var otherCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(numAttempts)

	for i := 0; i < numAttempts; i++ {
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "10.0.0.1:1234"
			rec := httptest.NewRecorder()

			h.CheckIn(rec, req)

			switch rec.Code {
			case http.StatusOK:
				successCount.Add(1)
			case http.StatusConflict:
				conflictCount.Add(1)
			default:
				otherCount.Add(1)
				t.Logf("goroutine %d: unexpected status %d: %s", idx, rec.Code, rec.Body.String())
			}
		}(i)
	}

	wg.Wait()

	if successCount.Load() != 1 {
		t.Errorf("expected exactly 1 success, got %d", successCount.Load())
	}
	if conflictCount.Load() != int32(numAttempts-1) {
		t.Errorf("expected %d conflicts, got %d", numAttempts-1, conflictCount.Load())
	}
	if otherCount.Load() != 0 {
		t.Errorf("expected 0 other responses, got %d", otherCount.Load())
	}

	// Verify exactly one queue ticket was created.
	var ticketCount int
	pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM queue_tickets WHERE facility_id = (SELECT facility_id FROM appointments WHERE id = $1)`,
		apptID).Scan(&ticketCount)
	if ticketCount != 1 {
		t.Errorf("expected exactly 1 queue ticket, got %d", ticketCount)
	}

	// Verify appointment final state.
	var status string
	pool.QueryRow(context.Background(), `SELECT status FROM appointments WHERE id = $1`, apptID).Scan(&status)
	if status != "queued" {
		t.Errorf("expected final status 'queued', got %q", status)
	}
}

// TestCheckIn_QueueFailure_RollsBackStatus verifies that when queue generation
// fails and fallback is disabled, the appointment rolls back to 'scheduled'.
func TestCheckIn_QueueFailure_RollsBackStatus(t *testing.T) {
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

	os.Unsetenv("SIGAP_ENGINE_FALLBACK")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewBookingHandler(pool, nil).
		WithQueueService(&failingQueueService{}).
		WithCheckinLimiter(limiter.NewRateLimiter(10, 1*time.Minute))

	body, _ := json.Marshal(map[string]string{"checkin_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments/"+apptID+"/check-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()

	h.CheckIn(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for queue failure, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify appointment was rolled back to 'scheduled'.
	var status string
	pool.QueryRow(context.Background(), `SELECT status FROM appointments WHERE id = $1`, apptID).Scan(&status)
	if status != "scheduled" {
		t.Errorf("expected status 'scheduled' after failed queue, got %q", status)
	}
}

// failingQueueService always returns an error on Generate.
type failingCheckinQueueService struct{}

func (f *failingCheckinQueueService) Generate(ctx context.Context, input service.GenerateInput) (service.GenerateResult, error) {
	return service.GenerateResult{}, context.DeadlineExceeded
}

func (f *failingCheckinQueueService) Probe(ctx context.Context) error {
	return context.DeadlineExceeded
}
