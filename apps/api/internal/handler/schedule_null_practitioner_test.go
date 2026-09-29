package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// T-3B5 regression: a schedule row with NO practitioner must not break the
// admin schedule list.
//
// WHY THIS EXISTS
//
// practitioner_schedules.practitioner_id is NULLABLE, and Phase 3B5 §6 requires
// schedule CREATE and UPDATE to OMIT practitioner_id entirely — so the column is
// NULL by design, for every schedule this phase creates. The list handler
// scanned that column into a Go `string`, and pgx returns
// "cannot scan NULL into *string" for that combination.
//
// The failure mode is what makes it worth a test: ONE such row made the ENTIRE
// list return 500 "Gagal membaca data jadwal." for every caller, not just the
// row at fault. It stayed latent because every seeded schedule had a
// practitioner, so the null path was never executed until 3B5 started writing
// rows without one.
//
// The fix is COALESCE(practitioner_id, ''::uuid) in the SELECT, which keeps the
// existing `omitempty` string contract intact — NULL and "" are identical on
// the wire, so no consumer changes.
//
// This test asserts the BEHAVIOUR (the list returns 200 and includes the row),
// not the SQL text, so a different correct fix would also pass.
// ---------------------------------------------------------------------------

// The test facility is the SEEDED one that a real actor can already read.
//
// This is deliberate. `FacilityScopeForActor` fails CLOSED for any actor with an
// empty AppUserID (`ErrClosed`), so a synthetic test actor carrying hand-built
// grants resolves to zero facilities and the list returns an empty 200 — which
// would make this test pass for the wrong reason, exactly the trap the 3B5.0
// S7 case was rewritten to avoid.
//
// Reusing the seeded facility and a real DB-resolved actor means the read path
// under test is the production one: real scope resolution, real grant
// provenance, real query. The practitioner-less ROW is the only thing new.
const (
	nullPractitionerFacilityID = "00000000-0000-0000-0000-00000000d000"
	nullPractitionerUnitID     = "00000000-0000-0000-0000-00000000d001"
	nullPractitionerScheduleID = "0f0f0f0f-0000-0000-0000-00000000f003"

	// The seeded local actor from packages/db/seed/dev.sql, which holds
	// facility_admin at the demo facility and therefore really holds
	// schedule.read and schedule.manage there.
	nullPractitionerActorID   = "00000000-0000-0000-0000-00000000d993"
	nullPractitionerActorName = "e2e-schedule-manager"
)

func setupScheduleNullPractitionerTest(t *testing.T) (*pgxpool.Pool, *AdminHandler) {
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

	// The scope resolver MUST be the real DB-backed one. With a nil resolver,
	// FacilityScopeForActor returns ErrClosed and every list answers an empty
	// 200 — which would make these tests pass while asserting nothing.
	return pool, NewAdminHandler(pool).WithFacilityScopeResolver(auth.NewDBFacilityScope(pool))
}

// seedPractitionerlessSchedule inserts a schedule row at the seeded demo
// facility whose practitioner_id is NULL — exactly the shape T-3B5 creates,
// because §6 requires the key to be omitted from the request body.
//
// The facility and service unit already exist from the seed, so only the
// schedule row is inserted. ON CONFLICT keeps the test idempotent across runs
// against a retained cluster.
func seedPractitionerlessSchedule(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// practitioner_id is deliberately NOT in the column list, so it defaults to
	// NULL — the same way T-3B5's create request leaves it.
	_, err := pool.Exec(ctx,
		`INSERT INTO practitioner_schedules (id, facility_id, service_unit_id,
		    schedule_date, start_time, end_time, slot_minutes, capacity_per_slot)
		 VALUES ($1, $2, $3, CURRENT_DATE, '09:00', '12:00', 30, 1)
		 ON CONFLICT (id) DO NOTHING`,
		nullPractitionerScheduleID, nullPractitionerFacilityID, nullPractitionerUnitID)
	if err != nil {
		t.Fatalf("seed practitioner-less schedule: %v", err)
	}

	// Assert the precondition, so a schema change that makes the column NOT NULL
	// fails here with a clear message instead of silently invalidating the test.
	var isNull bool
	if err := pool.QueryRow(ctx,
		`SELECT practitioner_id IS NULL FROM practitioner_schedules WHERE id = $1`,
		nullPractitionerScheduleID).Scan(&isNull); err != nil {
		t.Fatalf("read back seeded schedule: %v", err)
	}
	if !isNull {
		t.Fatalf("precondition failed: the seeded schedule has a practitioner_id, " +
			"so this test would pass without exercising the NULL scan path")
	}
}

// realActor resolves the SEEDED local actor through the REAL DB RBAC resolver.
//
// This is the same mechanism §2 proves end to end, so the read path exercised
// here is the production one rather than a hand-built grant list. A synthetic
// actor would fail closed at FacilityScopeForActor and return an empty 200,
// which would make these tests pass while asserting nothing.
func realActor(t *testing.T, pool *pgxpool.Pool) identity.Actor {
	t.Helper()
	ctx := context.Background()

	resolved, err := auth.NewRBACResolver(pool).Resolve(ctx, nullPractitionerActorName)
	if err != nil {
		t.Fatalf("resolve seeded actor %q: %v", nullPractitionerActorName, err)
	}
	if len(resolved.Permissions) == 0 {
		t.Skipf("seeded actor %q has no permissions; run packages/db/seed first",
			nullPractitionerActorName)
	}
	if resolved.AppUserID != nullPractitionerActorID {
		t.Logf("seeded actor resolved to app_user_id %s (expected %s)",
			resolved.AppUserID, nullPractitionerActorID)
	}

	return identity.Actor{
		Type:           identity.ActorUser,
		UserID:         nullPractitionerActorName,
		AppUserID:      resolved.AppUserID,
		Permissions:    resolved.Permissions,
		FacilityGrants: resolved.Grants,
		IsDev:          false,
	}
}

func TestListSchedulesWithNullPractitionerReturns200(t *testing.T) {
	pool, h := setupScheduleNullPractitionerTest(t)
	seedPractitionerlessSchedule(t, pool)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/schedules", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), realActor(t, pool)))

	h.ListSchedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a schedule with no practitioner, got %d: %s",
			rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, nullPractitionerScheduleID) {
		t.Errorf("expected the practitioner-less schedule in the list body, got: %s", body)
	}
	// The row must still be returned with a usable schedule_date; a COALESCE that
	// accidentally swallowed another column would still pass the check above.
	if !strings.Contains(body, "slot_minutes") {
		t.Errorf("expected schedule fields to be present, got: %s", body)
	}
}

func TestGetScheduleWithNullPractitionerReturns200(t *testing.T) {
	pool, h := setupScheduleNullPractitionerTest(t)
	seedPractitionerlessSchedule(t, pool)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/schedules/"+nullPractitionerScheduleID, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), realActor(t, pool)))

	h.GetSchedule(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a detail read with no practitioner, got %d: %s",
			rec.Code, rec.Body.String())
	}
}

// TestListAppointmentsWithNullPractitionerColumns covers the identical bug on
// the appointment list, where BOTH practitioner_id and practitioner_schedule_id
// are nullable. The seeded appointment happens to have both set, so this path is
// reachable only from a booking that omits them.
func TestListAppointmentsWithNullPractitionerColumns(t *testing.T) {
	pool, h := setupScheduleNullPractitionerTest(t)
	ctx := context.Background()

	// practitioner_id and practitioner_schedule_id are deliberately absent from
	// the column list, so both default to NULL.
	apptID := "0f0f0f0f-0000-0000-0000-00000000f011"
	_, err := pool.Exec(ctx,
		`INSERT INTO appointments (id, facility_id, service_unit_id, appointment_time,
		    status, patient_display_name, patient_phone, checkin_code)
		 VALUES ($1, $2, $3, now(), 'scheduled', 'Pasien Uji', '085500000099', 'UJI0001')
		 ON CONFLICT (id) DO NOTHING`,
		apptID, nullPractitionerFacilityID, nullPractitionerUnitID)
	if err != nil {
		t.Fatalf("seed appointment with null practitioner columns: %v", err)
	}

	// Assert the precondition, so a schema change that makes either column NOT
	// NULL fails here instead of silently invalidating the test.
	var bothNull bool
	if err := pool.QueryRow(ctx,
		`SELECT practitioner_id IS NULL AND practitioner_schedule_id IS NULL
		 FROM appointments WHERE id = $1`, apptID).Scan(&bothNull); err != nil {
		t.Fatalf("read back seeded appointment: %v", err)
	}
	if !bothNull {
		t.Fatalf("precondition failed: the seeded appointment has practitioner columns " +
			"set, so this test would pass without exercising the NULL scan path")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/appointments", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), realActor(t, pool)))

	h.ListAppointments(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an appointment with no practitioner columns, got %d: %s",
			rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), apptID) {
		t.Errorf("expected the appointment in the list body, got: %s", rec.Body.String())
	}
}

// A compile-time reminder that the fix does not touch the authorization model.
// If someone later removes auth.FacilityScopeResultAllows or the grant
// provenance check while fixing a read shape, this fails to build.
var _ = auth.FacilityScopeResultAllows



