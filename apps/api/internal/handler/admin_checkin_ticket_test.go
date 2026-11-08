package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedAdminCheckinData creates a facility, service unit, patient and a
// scheduled appointment for the admin counter check-in integration tests. The
// patient row is seeded so recordAdminQueueTicket's phone-upsert updates it
// rather than minting a stray patient, keeping cleanup deterministic.
func seedAdminCheckinData(t *testing.T, pool *pgxpool.Pool) (apptID, facID string, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	uniqueID := time.Now().UnixNano()
	facIDStr := "80000000-0000-0000-0000-" + padID(uniqueID)
	svcUnitID := "81000000-0000-0000-0000-" + padID(uniqueID)
	patientID := "82000000-0000-0000-0000-" + padID(uniqueID)
	apptIDStr := "83000000-0000-0000-0000-" + padID(uniqueID)
	phone := "0855" + padID(uniqueID)[:8]

	if _, err := pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, short_code)
		 VALUES ($1, 'Admin Fac', 'rumah_sakit', 'Addr', 'Kec', 'Kab', 'Prov', '000', 'ADM')
		 ON CONFLICT (id) DO NOTHING`, facIDStr); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO patients (id, full_name, phone, date_of_birth)
		 VALUES ($1, 'Admin Patient', $2, '1990-01-01')
		 ON CONFLICT (phone) DO NOTHING`, patientID, phone); err != nil {
		t.Fatalf("seed patient: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name, code)
		 VALUES ($1, $2, 'Admin Unit', 'ADM-U')
		 ON CONFLICT (id) DO NOTHING`, svcUnitID, facIDStr); err != nil {
		t.Fatalf("seed service unit: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO appointments (id, facility_id, service_unit_id, patient_display_name, patient_phone, appointment_time, status, checkin_code)
		 VALUES ($1, $2, $3, 'Admin Patient', $4, NOW() + INTERVAL '1 day', 'scheduled', 'ADMCHK1')
		 ON CONFLICT (id) DO NOTHING`, apptIDStr, facIDStr, svcUnitID, phone); err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	cleanupFn := func() {
		pool.Exec(ctx, `DELETE FROM appointments WHERE id = $1`, apptIDStr)
		pool.Exec(ctx, `DELETE FROM queue_tickets WHERE facility_id = $1`, facIDStr)
		pool.Exec(ctx, `DELETE FROM service_units WHERE id = $1`, svcUnitID)
		pool.Exec(ctx, `DELETE FROM patients WHERE id = $1`, patientID)
		pool.Exec(ctx, `DELETE FROM facilities WHERE id = $1`, facIDStr)
	}

	return apptIDStr, facIDStr, cleanupFn
}

// TestAdminCheckIn_MintsQueueTicketAndAttaches proves the N4 fix: an admin
// setting checked_in (from scheduled) mints a queue_tickets row and atomically
// attaches it to the appointment, mirroring the public check-in path. The dev
// fallback (SIGAP_ENGINE_FALLBACK=dev) is exercised because no Rust engine is
// reachable in CI.
func TestAdminCheckIn_MintsQueueTicketAndAttaches(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, _, cleanup := seedAdminCheckinData(t, pool)
	defer cleanup()

	os.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewAdminHandler(pool).WithQueueService(&failingQueueService{})
	actor := makeDevActor(uuid.NewString(), "appointment.manage")

	rec := dispatchAppointmentRequest(h, http.MethodPatch, "/api/v1/admin/appointments/"+apptID+"/status", `{"status":"checked_in"}`, actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	ticketID, _ := data["queue_ticket_id"].(string)
	if ticketID == "" {
		t.Fatalf("expected queue_ticket_id in response, got %v", data)
	}

	// The queue_tickets row exists in THIS database.
	var qStatus string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM queue_tickets WHERE id = $1`, ticketID).Scan(&qStatus); err != nil {
		t.Fatalf("queue_tickets row not found for %s: %v", ticketID, err)
	}
	if qStatus != "waiting" {
		t.Errorf("expected queue_tickets.status 'waiting', got %q", qStatus)
	}

	// The appointment references the ticket and remains checked_in.
	var apptStatus string
	var qTicketID *string
	if err := pool.QueryRow(context.Background(),
		`SELECT status, queue_ticket_id FROM appointments WHERE id = $1`, apptID).Scan(&apptStatus, &qTicketID); err != nil {
		t.Fatalf("verify appointment: %v", err)
	}
	if apptStatus != "checked_in" {
		t.Errorf("expected appointment status 'checked_in', got %q", apptStatus)
	}
	if qTicketID == nil || *qTicketID != ticketID {
		t.Errorf("expected queue_ticket_id=%s, got %v", ticketID, qTicketID)
	}
}

// TestAdminCheckIn_SecondAttemptDoesNotMintSecondTicket proves duplicate
// issuance is guarded: after a successful checked_in, a second checked_in
// attempt is refused (the transition is no longer valid) and no second
// queue_tickets row is minted.
func TestAdminCheckIn_SecondAttemptDoesNotMintSecondTicket(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, facID, cleanup := seedAdminCheckinData(t, pool)
	defer cleanup()

	os.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewAdminHandler(pool).WithQueueService(&failingQueueService{})
	actor := makeDevActor(uuid.NewString(), "appointment.manage")
	path := "/api/v1/admin/appointments/" + apptID + "/status"

	if rec := dispatchAppointmentRequest(h, http.MethodPatch, path, `{"status":"checked_in"}`, actor); rec.Code != http.StatusOK {
		t.Fatalf("first attempt expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if rec := dispatchAppointmentRequest(h, http.MethodPatch, path, `{"status":"checked_in"}`, actor); rec.Code != http.StatusBadRequest {
		t.Fatalf("second attempt expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM queue_tickets WHERE facility_id = $1`, facID).Scan(&count); err != nil {
		t.Fatalf("count queue_tickets: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 queue ticket, got %d", count)
	}
}

// TestAdminCheckIn_EngineUnavailableRollsBackToScheduled proves that when the
// queue engine is unreachable (no dev fallback), the appointment is released
// back to scheduled — never stranded at checked_in with no ticket — so a retry
// can mint a number.
func TestAdminCheckIn_EngineUnavailableRollsBackToScheduled(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, _, cleanup := seedAdminCheckinData(t, pool)
	defer cleanup()

	os.Unsetenv("SIGAP_ENGINE_FALLBACK")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewAdminHandler(pool).WithQueueService(&failingQueueService{})
	actor := makeDevActor(uuid.NewString(), "appointment.manage")

	rec := dispatchAppointmentRequest(h, http.MethodPatch, "/api/v1/admin/appointments/"+apptID+"/status", `{"status":"checked_in"}`, actor)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when engine unavailable, got %d: %s", rec.Code, rec.Body.String())
	}

	var apptStatus string
	var qTicketID *string
	var checkinAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT status, queue_ticket_id, checkin_at FROM appointments WHERE id = $1`, apptID).Scan(&apptStatus, &qTicketID, &checkinAt); err != nil {
		t.Fatalf("verify appointment: %v", err)
	}
	if apptStatus != "scheduled" {
		t.Errorf("expected appointment rolled back to 'scheduled', got %q", apptStatus)
	}
	if qTicketID != nil {
		t.Errorf("expected no queue_ticket_id, got %v", *qTicketID)
	}
	if checkinAt != nil {
		t.Errorf("expected checkin_at cleared, got %v", checkinAt)
	}
}
// TestAdminCheckIn_ConcurrentOnlyOneTicket proves the guarded transition: two
// goroutines check in the SAME scheduled appointment at once; exactly one
// succeeds (200) and exactly one queue_tickets row is minted. The loser is
// refused with 409 and must not leave a phantom ticket on the board. The
// sequential second-attempt test below covers the post-commit refusal; this one
// covers the true race the optimistic `AND status = $current` predicate closes.
func TestAdminCheckIn_ConcurrentOnlyOneTicket(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	apptID, facID, cleanup := seedAdminCheckinData(t, pool)
	defer cleanup()

	os.Setenv("SIGAP_ENGINE_FALLBACK", "dev")
	defer os.Unsetenv("SIGAP_ENGINE_FALLBACK")

	h := NewAdminHandler(pool).WithQueueService(&failingQueueService{})

	const workers = 2
	codes := make([]int, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			actor := makeDevActor(uuid.NewString(), "appointment.manage")
			rec := dispatchAppointmentRequest(h, http.MethodPatch,
				"/api/v1/admin/appointments/"+apptID+"/status",
				`{"status":"checked_in"}`, actor)
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()

	okCount, refusalCount := 0, 0
	for _, c := range codes {
		if c == http.StatusOK {
			okCount++
		} else if c >= 400 && c < 500 {
			// The loser is refused. WHICH 4xx depends on how the race resolved:
			// 409 if both reads saw 'scheduled' and the optimistic guard caught
			// it, or 400 if the loser read the already-committed 'checked_in'
			// and the transition check refused it. Both are safe; neither mints.
			refusalCount++
		}
	}
	if okCount != 1 || refusalCount != 1 {
		t.Errorf("expected exactly one 200 and one 4xx refusal, got codes=%v", codes)
	}

	var ticketCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM queue_tickets WHERE facility_id = $1`, facID).Scan(&ticketCount); err != nil {
		t.Fatalf("count queue_tickets: %v", err)
	}
	if ticketCount != 1 {
		t.Errorf("expected exactly one queue_tickets row for the facility, got %d", ticketCount)
	}
}
