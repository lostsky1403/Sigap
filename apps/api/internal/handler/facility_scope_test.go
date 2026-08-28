package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/migrate"
)

// ---------------------------------------------------------------------------
// Test harness: DB-backed facility scope + static actor provider
// ---------------------------------------------------------------------------

// scopeTestDBURL returns the PostgreSQL DSN for facility scope integration
// tests, or "" when not configured so the test suite can skip cleanly.
func scopeTestDBURL() string {
	return os.Getenv("DATABASE_URL")
}

// migrationFileName resolves a migration version to its exact filename using
// the same discovery the migration runner uses. (Mirrors the unexported helper
// in the auth package's tests, replicated here for the handler test package.)
func migrationFileName(dir string, version int) (string, error) {
	migrations, err := migrate.DiscoverMigrations(dir)
	if err != nil {
		return "", err
	}
	for _, m := range migrations {
		if m.Version == version {
			return filepath.Base(m.Path), nil
		}
	}
	return "", fmt.Errorf("migration %d not found", version)
}

// applyScopeMigrations builds the full schema the facility-scope tests depend
// on from real migration files: 0001 (facilities/queue_tickets), 0003
// (identity/RBAC), 0005 (service_units/appointments), 0006 (notifications),
// 0008 (app_users.subject), and 0009 (user_roles lifecycle). It drops and
// rebuilds a dedicated, throwaway schema (test_scope) so the shared "public"
// schema — which other suites (e.g. booking check-in) rely on as a pre-built
// full schema — is never dropped or raced against. The pool's search_path is
// set to test_scope so every unqualified relation reference lands there.
// Migration 0007 is skipped (pre-existing defect in checkin_constraints,
// out of scope for AUDIT-202).
func applyScopeMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) error {
	t.Helper()
	dir, err := migrate.DefaultDir()
	if err != nil {
		t.Fatalf("migrate dir: %v", err)
	}

	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS test_scope CASCADE"); err != nil {
		return fmt.Errorf("drop test_scope schema: %w", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA test_scope"); err != nil {
		return fmt.Errorf("create test_scope schema: %w", err)
	}

	order := []int{1, 3, 5, 6, 8, 9}
	for _, v := range order {
		name, mErr := migrationFileName(dir, v)
		if mErr != nil {
			return mErr
		}
		content, rErr := os.ReadFile(filepath.Join(dir, name))
		if rErr != nil {
			return fmt.Errorf("read migration %d: %w", v, rErr)
		}
		if _, eErr := pool.Exec(ctx, string(content)); eErr != nil {
			return fmt.Errorf("apply migration %d: %w", v, eErr)
		}
	}
	return nil
}

// newScopeTestPool connects to the dedicated integration DB and applies all
// scope-relevant migrations. Returns the pool and a cleanup function.
func newScopeTestPool(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	dbURL := scopeTestDBURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping facility scope integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	// Route every unqualified relation/function reference to the throwaway
	// test_scope schema. "public" is kept on the path only so that any
	// extension-backed functions (e.g. gen_random_uuid) resolve correctly;
	// DDL always targets test_scope because it is first in the list.
	cfg.ConnConfig.RuntimeParams["search_path"] = "test_scope, public"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}

	if err := applyScopeMigrations(t, context.Background(), pool); err != nil {
		pool.Close()
		t.Fatalf("apply migrations: %v", err)
	}

	cleanup := func() { pool.Close() }
	return pool, cleanup
}

// seedFacilityByName inserts a facility with the given UUID and returns it.
func seedFacilityByName(ctx context.Context, pool *pgxpool.Pool, id, name string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota,
			provinsi, phone, total_beds, available_beds, short_code)
		 VALUES ($1, $2, 'puskesmas', 'Jl. Test No.1', 'Kec. Test', 'Kab. Test',
			'Prov. Test', '021-12345678', 10, 5, 'FTST')
		 ON CONFLICT (id) DO NOTHING`,
		id, name)
	return err
}

// seedAppUser creates an app_users row with the given UUID and returns it.
func seedAppUser(ctx context.Context, pool *pgxpool.Pool, appUserID, subject string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO app_users (id, email, status, subject)
		 VALUES ($1, $2, 'active', $3)
		 ON CONFLICT (id) DO NOTHING`,
		appUserID, subject+"@example.test", subject)
	return err
}

// seedUserRoles creates a user_roles row linking appUserID to a facility via
// a role that has the given permission keys. status defaults to 'active'.
func seedUserRoles(ctx context.Context, pool *pgxpool.Pool, appUserID, facilityID string, perms []string) error {
	roleID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO roles (id, name, description, is_system)
		 VALUES ($1, $2, 'test role', false)`,
		roleID, "role_"+uuid.NewString()); err != nil {
		return fmt.Errorf("insert role: %w", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id, facility_id, status, deleted_at)
		 VALUES ($1, $2, $3, 'active', NULL)`,
		appUserID, roleID, facilityID); err != nil {
		return fmt.Errorf("insert user_roles: %w", err)
	}

	for _, perm := range perms {
		var permID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO permissions (key, description) VALUES ($1, 'test permission')
			 ON CONFLICT (key) DO NOTHING
			 RETURNING id::text`, perm).Scan(&permID); err != nil {
			if qErr := pool.QueryRow(ctx,
				`SELECT id::text FROM permissions WHERE key = $1`, perm).Scan(&permID); qErr != nil {
				return fmt.Errorf("get permission %s: %w", perm, qErr)
			}
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)
			 ON CONFLICT (role_id, permission_id) DO NOTHING`,
			roleID, permID); err != nil {
			return fmt.Errorf("insert role_permission: %w", err)
		}
	}
	return nil
}

// setRoleStatus updates the status and/or deleted_at of user_roles for a user
// in a facility, for lifecycle tests.
func setUserRoleLifecycle(ctx context.Context, pool *pgxpool.Pool, appUserID, facilityID, status string, deletedAt *time.Time) error {
	q := `UPDATE user_roles SET status = $1, deleted_at = $2
	      WHERE user_id = $3 AND facility_id = $4`
	_, err := pool.Exec(ctx, q, status, deletedAt, appUserID, facilityID)
	return err
}

// scopedHandler builds an AdminHandler wired with the DB-backed FacilityScope
// for a given pool. Used by DB-backed integration tests.
func scopedHandler(pool *pgxpool.Pool) *AdminHandler {
	return NewAdminHandler(pool).WithFacilityScopeResolver(auth.NewDBFacilityScope(pool))
}

// dispatchRequest runs an AdminHandler method directly (bypassing the router
// dispatch) with the given actor attached to the request context via a
// staticActorProvider-free approach: we use identity.ContextWithActor.
func dispatchRequest(h *AdminHandler, method, path string, body string, actor identity.Actor) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if body != "" {
		req.Body = nopCloser{Reader: stringReader(body)}
		req.Header.Set("Content-Type", "application/json")
	}
	ctx := identity.ContextWithActor(req.Context(), actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.FacilitiesRouter(rec, req)
	if rec.Code == http.StatusMethodNotAllowed {
		h.ServiceUnitsRouter(rec, req)
	}
	if rec.Code == http.StatusMethodNotAllowed {
		h.QueuesRouter(rec, req)
	}
	if rec.Code == http.StatusMethodNotAllowed {
		h.SchedulesRouter(rec, req)
	}
	if rec.Code == http.StatusMethodNotAllowed {
		h.AppointmentsRouter(rec, req)
	}
	return rec
}

// dispatchServiceUnitRequest dispatches to the service units router.
func dispatchServiceUnitRequest(h *AdminHandler, method, path, body string, actor identity.Actor) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if body != "" {
		req.Body = nopCloser{Reader: stringReader(body)}
		req.Header.Set("Content-Type", "application/json")
	}
	ctx := identity.ContextWithActor(req.Context(), actor)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServiceUnitsRouter(rec, req)
	return rec
}

// dispatchAppointmentRequest dispatches to the appointments router.
func dispatchAppointmentRequest(h *AdminHandler, method, path, body string, actor identity.Actor) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if body != "" {
		req.Body = nopCloser{Reader: stringReader(body)}
		req.Header.Set("Content-Type", "application/json")
	}
	ctx := identity.ContextWithActor(req.Context(), actor)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.AppointmentsRouter(rec, req)
	return rec
}

// dispatchQueueRequest dispatches to the queues router.
func dispatchQueueRequest(h *AdminHandler, method, path, body string, actor identity.Actor) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if body != "" {
		req.Body = nopCloser{Reader: stringReader(body)}
		req.Header.Set("Content-Type", "application/json")
	}
	ctx := identity.ContextWithActor(req.Context(), actor)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.QueuesRouter(rec, req)
	return rec
}

func stringReader(s string) interface {
	Read(p []byte) (int, error)
} {
	return &simpleReader{s: s}
}

type simpleReader struct {
	s   string
	pos int
}

func (r *simpleReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.s) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, r.s[r.pos:])
	r.pos += n
	return n, nil
}

type nopCloser struct {
	Reader interface {
		Read(p []byte) (int, error)
	}
}

func (nc nopCloser) Read(p []byte) (int, error) {
	return nc.Reader.Read(p)
}

func (nopCloser) Close() error { return nil }

// makeScopedActor creates an actor with the given AppUserID and permissions.
// The actor is NOT dev (IsDev=false), so facility scope enforcement applies.
func makeScopedActor(appUserID string, perms ...string) identity.Actor {
	return identity.Actor{
		Type:        identity.ActorUser,
		UserID:      "sub:" + appUserID,
		Permissions: perms,
		AppUserID:   appUserID,
		IsDev:       false,
	}
}

// makeDevActor creates a dev actor that bypasses facility scope checks.
func makeDevActor(appUserID string, perms ...string) identity.Actor {
	return identity.Actor{
		Type:        identity.ActorDev,
		UserID:      "dev:" + appUserID,
		Permissions: perms,
		AppUserID:   appUserID,
		IsDev:       true,
	}
}

// extractFacilityIDsFromBody decodes the JSON response body and extracts
// the "data" array of facility IDs from a ListFacilities response.
func extractFacilityIDsFromListBody(t *testing.T, body []byte) []string {
	t.Helper()
	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal list body: %v", err)
	}
	ids := make([]string, 0, len(resp.Data))
	for _, f := range resp.Data {
		ids = append(ids, f.ID)
	}
	return ids
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests 1-5: Facilities (read, list, update)
// ---------------------------------------------------------------------------

func TestFacilityScope_FacilityReadAllowed(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-scope"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "facility.read")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities/"+facA, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.GetFacility(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for in-scope facility read, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFacilityScope_FacilityCrossScopeDenied(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-cross"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "facility.read")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities/"+facB, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.GetFacility(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for out-of-scope facility, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFacilityScope_FacilityUpdateAllowed(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-manage"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.manage"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "facility.manage")

	body := `{"name":"Fasilitas A Updated"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/facilities/"+facA, stringReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.UpdateFacility(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for in-scope facility update, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFacilityScope_FacilityCrossScopeUpdateRejected(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-xscope-update"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.manage"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "facility.manage")

	body := `{"name":"Fasilitas B Hacked"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/facilities/"+facB, stringReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.UpdateFacility(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for out-of-scope facility update, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFacilityScope_ListFacilitiesOnlyOwn(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-list"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "facility.read")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.ListFacilities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for facility list, got %d: %s", rec.Code, rec.Body.String())
	}

	ids := extractFacilityIDsFromListBody(t, rec.Body.Bytes())
	if len(ids) != 1 {
		t.Fatalf("expected exactly 1 facility in list, got %d: %v", len(ids), ids)
	}
	if ids[0] != facA {
		t.Errorf("expected facility %s in list, got %s", facA, ids[0])
	}
}

// ---------------------------------------------------------------------------
// Tests 6-7: Service units (scoped by facility)
// ---------------------------------------------------------------------------

func TestFacilityScope_ServiceUnitUnderAllowedFacility(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	suA := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
		 VALUES ($1, $2, 'Poli Umum', 'PU', '', true)
		 ON CONFLICT (id) DO NOTHING`,
		suA, facA); err != nil {
		t.Fatalf("seed service_unit: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-su-ok"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "schedule.read")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-units/"+suA, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.GetServiceUnit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for service unit under allowed facility, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFacilityScope_ServiceUnitUnderDeniedFacility(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	suB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
		 VALUES ($1, $2, 'Poli Gigi', 'PG', '', true)
		 ON CONFLICT (id) DO NOTHING`,
		suB, facB); err != nil {
		t.Fatalf("seed service_unit: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-su-deny"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "schedule.read")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-units/"+suB, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.GetServiceUnit(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for service unit under denied facility, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Tests 8-9: Appointments & queue tickets (scoped by facility)
// ---------------------------------------------------------------------------

func TestFacilityScope_AppointmentUnderDeniedFacility(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	suB := uuid.NewString()
	apptB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
		 VALUES ($1, $2, 'Poli Gigi', 'PG', '', true)
		 ON CONFLICT (id) DO NOTHING`,
		suB, facB); err != nil {
		t.Fatalf("seed service_unit: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO appointments (id, facility_id, service_unit_id, patient_display_name,
			patient_phone, appointment_time, status, checkin_code, notes,
			queue_ticket_id, checkin_at, completed_at, cancelled_at, created_at, updated_at)
		 VALUES ($1, $2, $3, 'Pasien Demo', '085550000001',
			NOW() + INTERVAL '1 day', 'scheduled', 'ABC123', '',
			NULL, NULL, NULL, NULL, NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`,
		apptB, facB, suB); err != nil {
		t.Fatalf("seed appointment: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-appt-deny"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "appointment.manage")

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/appointments/"+apptB+"/status", stringReader(`{"status":"completed"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.UpdateAppointmentStatus(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for appointment under denied facility, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFacilityScope_QueueTicketUnderDeniedFacility(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	patientB := uuid.NewString()
	qtB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO patients (id, full_name, phone, gender, date_of_birth)
		 VALUES ($1, 'Pasien B', '085550000002', 'L', '1990-01-01')
		 ON CONFLICT (id) DO NOTHING`,
		patientB); err != nil {
		t.Fatalf("seed patient: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status)
		 VALUES ($1, $2, $3, 1, 'QT-0001', 'waiting')
		 ON CONFLICT (id) DO NOTHING`,
		qtB, facB, patientB); err != nil {
		t.Fatalf("seed queue_ticket: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-queue-deny"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "queue.read")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/queues/"+qtB, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.GetQueueTicket(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for queue ticket under denied facility, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 10: Forged facility_id in create body
// ---------------------------------------------------------------------------

func TestFacilityScope_ForgedFacilityIDInCreateBody(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-forge"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"schedule.manage"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "schedule.manage")

	// User is assigned only facility A, but forges facility_id=B in body.
	body := fmt.Sprintf(`{"facility_id":"%s","service_unit_id":"%s","schedule_date":"2026-12-01","start_time":"08:00","end_time":"16:00","slot_minutes":30,"capacity_per_slot":5}`,
		facB, uuid.NewString())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/schedules", stringReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.CreateSchedule(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forged facility_id in create body, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify no service_unit was created pointing to facility B by this user.
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM practitioner_schedules WHERE facility_id = $1`, facB).Scan(&count)
	if err != nil {
		t.Fatalf("count schedules: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 schedules for forged facility_id B, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Test 11: Disabled/soft-deleted user_role → no scope
// ---------------------------------------------------------------------------

func TestFacilityScope_DisabledUserRoleGrantsNoScope(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-disabled"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}
	// Disable the role assignment.
	if err := setUserRoleLifecycle(ctx, pool, appUserID, facA, "inactive", nil); err != nil {
		t.Fatalf("set role inactive: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "facility.read")

	// List should return empty (no facilities in scope).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.ListFacilities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ids := extractFacilityIDsFromListBody(t, rec.Body.Bytes())
	if len(ids) != 0 {
		t.Errorf("expected 0 facilities for disabled role, got %d: %v", len(ids), ids)
	}
}

func TestFacilityScope_SoftDeletedUserRoleGrantsNoScope(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-deleted"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}
	// Soft-delete the role assignment.
	if err := setUserRoleLifecycle(ctx, pool, appUserID, facA, "active", &time.Time{}); err != nil {
		t.Fatalf("set role deleted: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "facility.read")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.ListFacilities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ids := extractFacilityIDsFromListBody(t, rec.Body.Bytes())
	if len(ids) != 0 {
		t.Errorf("expected 0 facilities for soft-deleted role, got %d: %v", len(ids), ids)
	}
}

// ---------------------------------------------------------------------------
// Test 12: DB scope resolver error → fail closed
// ---------------------------------------------------------------------------

func TestFacilityScope_ResolverErrorFailsClosed(t *testing.T) {
	ctx := context.Background()

	// Use a nil pool — DBFacilityScope with a closed/nil pool returns ErrClosed.
	resolver := auth.NewDBFacilityScope(nil)
	actor := identity.Actor{
		Type:        identity.ActorUser,
		UserID:      "sub:test-user",
		Permissions: []string{"facility.read"},
		AppUserID:   uuid.NewString(),
		IsDev:       false,
	}

	// AllowedFacilityIDsForActor should fail closed when resolver pool is nil.
	allowed, _, err := auth.AllowedFacilityIDsForActor(ctx, actor, resolver)
	if err == nil {
		t.Error("expected fail-closed (err != nil) with nil pool resolver, got accepted")
	}
	if allowed != nil {
		t.Errorf("expected nil allowed facilities on resolver error, got %v", allowed)
	}

	// CanAccessFacilityForActor should fail closed with nil pool.
	facID := uuid.New()
	result := auth.CanAccessFacilityForActor(ctx, actor, resolver, facID)
	if result {
		t.Error("expected false for CanAccessFacility with nil pool resolver, got true")
	}
}

// Also verify fail-closed at the FacilityScope interface level directly.
func TestFacilityScope_ResolverDirectErrorFailsClosed(t *testing.T) {
	ctx := context.Background()
	resolver := auth.NewDBFacilityScope(nil)

	ids, err := resolver.AllowedFacilityIDs(ctx, "some-app-user-id")
	if err == nil {
		t.Error("expected error from AllowedFacilityIDs with nil pool")
	}
	if ids != nil {
		t.Errorf("expected nil ids on error, got %v", ids)
	}

	ok, err := resolver.CanAccessFacility(ctx, "some-app-user-id", uuid.New())
	if err == nil {
		t.Error("expected error from CanAccessFacility with nil pool")
	}
	if ok {
		t.Error("expected false from CanAccessFacility on error")
	}
}

// ---------------------------------------------------------------------------
// Test 13: Dev identity local flow → demo functional
// ---------------------------------------------------------------------------

func TestFacilityScope_DevActorBypassesScope(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas Dev"); err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "dev-bypass"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeDevActor(appUserID, "facility.read")

	// Dev actor with NO user_roles should still see all facilities.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.ListFacilities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for dev actor, got %d: %s", rec.Code, rec.Body.String())
	}

	ids := extractFacilityIDsFromListBody(t, rec.Body.Bytes())
	if !containsString(ids, facA) {
		t.Errorf("dev actor should see facility A (%s), got: %v", facA, ids)
	}
}

// ---------------------------------------------------------------------------
// Test 14: Same permission, two users, different scopes → isolated
// ---------------------------------------------------------------------------

func TestFacilityScope_MultiUserIsolation(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	userA := uuid.NewString()
	userB := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if err := seedAppUser(ctx, pool, userA, "charlie-isolated"); err != nil {
		t.Fatalf("seed app user A: %v", err)
	}
	if err := seedAppUser(ctx, pool, userB, "delta-isolated"); err != nil {
		t.Fatalf("seed app user B: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userA, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles A: %v", err)
	}
	if err := seedUserRoles(ctx, pool, userB, facB, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles B: %v", err)
	}

	h := scopedHandler(pool)

	// User A should only see facility A.
	actorA := makeScopedActor(userA, "facility.read")
	reqA := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	reqA = reqA.WithContext(identity.ContextWithActor(reqA.Context(), actorA))
	recA := httptest.NewRecorder()
	h.ListFacilities(recA, reqA)

	if recA.Code != http.StatusOK {
		t.Fatalf("expected 200 for user A list, got %d", recA.Code)
	}
	idsA := extractFacilityIDsFromListBody(t, recA.Body.Bytes())
	if !containsString(idsA, facA) {
		t.Errorf("user A should see facility A, got: %v", idsA)
	}
	if containsString(idsA, facB) {
		t.Errorf("user A should NOT see facility B, got: %v", idsA)
	}

	// User B should only see facility B.
	actorB := makeScopedActor(userB, "facility.read")
	reqB := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	reqB = reqB.WithContext(identity.ContextWithActor(reqB.Context(), actorB))
	recB := httptest.NewRecorder()
	h.ListFacilities(recB, reqB)

	if recB.Code != http.StatusOK {
		t.Fatalf("expected 200 for user B list, got %d", recB.Code)
	}
	idsB := extractFacilityIDsFromListBody(t, recB.Body.Bytes())
	if !containsString(idsB, facB) {
		t.Errorf("user B should see facility B, got: %v", idsB)
	}
	if containsString(idsB, facA) {
		t.Errorf("user B should NOT see facility A, got: %v", idsB)
	}
}

// ---------------------------------------------------------------------------
// Test 15: IDOR — out-of-scope resource returns 404, no existence oracle
// ---------------------------------------------------------------------------

func TestFacilityScope_IDORNoExistenceOracle(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	suB := uuid.NewString()
	appUserID := uuid.NewString()

	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO service_units (id, facility_id, name, code, description, is_active)
		 VALUES ($1, $2, 'Poli Gigi', 'PG', '', true)
		 ON CONFLICT (id) DO NOTHING`,
		suB, facB); err != nil {
		t.Fatalf("seed service_unit: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "alice-idor"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles: %v", err)
	}

	h := scopedHandler(pool)
	actor := makeScopedActor(appUserID, "schedule.read")

	// Try to access a service unit under facility B (exists in DB but out of scope).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-units/"+suB, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.GetServiceUnit(rec, req)

	// Should be 404 — same code as a truly non-existent resource, so the
	// caller cannot distinguish "exists but not yours" from "does not exist".
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for out-of-scope resource (no IDOR oracle), got %d: %s", rec.Code, rec.Body.String())
	}

	// Also verify that a genuinely non-existent resource returns 404.
	ghostID := uuid.NewString()
	reqGhost := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-units/"+ghostID, nil)
	reqGhost = reqGhost.WithContext(identity.ContextWithActor(reqGhost.Context(), actor))
	recGhost := httptest.NewRecorder()
	h.GetServiceUnit(recGhost, reqGhost)

	if recGhost.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent resource, got %d", recGhost.Code)
	}
}

// ---------------------------------------------------------------------------
// Test 6.17 (Security proof): A allowed, B denied; DB grant → B allowed; A still allowed
// ---------------------------------------------------------------------------

func TestFacilityScope_SecurityProof_DBAuthoritativeScope(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	facA := uuid.NewString()
	facB := uuid.NewString()
	appUserID := uuid.NewString()

	// Seed two facilities and a user assigned to facility A only.
	if err := seedFacilityByName(ctx, pool, facA, "Fasilitas A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facB, "Fasilitas B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	if err := seedAppUser(ctx, pool, appUserID, "proof-user"); err != nil {
		t.Fatalf("seed app user: %v", err)
	}

	// Grant facility A scope (active role with facility.read).
	if err := seedUserRoles(ctx, pool, appUserID, facA, []string{"facility.read"}); err != nil {
		t.Fatalf("seed user_roles facility A: %v", err)
	}

	h := scopedHandler(pool)
	// Same actor/JWT throughout — only the DB changes.
	actor := makeScopedActor(appUserID, "facility.read")

	// --- Step 1: GET facility A → allowed (200) ---
	t.Run("GET facility A allowed with A assignment only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities/"+facA, nil)
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.GetFacility(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for facility A (in scope), got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Step 2: GET facility B → denied (404) — before DB grant ---
	t.Run("GET facility B denied before DB grant", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities/"+facB, nil)
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.GetFacility(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for facility B (out of scope before grant), got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Step 3: Add DB role grant for facility B (same app user, same "JWT") ---
	if err := seedUserRoles(ctx, pool, appUserID, facB, []string{"facility.read"}); err != nil {
		t.Fatalf("grant facility B scope: %v", err)
	}

	// --- Step 4: GET facility B → now allowed (200) ---
	t.Run("GET facility B allowed after DB grant (same actor)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities/"+facB, nil)
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.GetFacility(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for facility B after DB grant, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Step 5: GET facility A → still allowed (200) — no regression ---
	t.Run("GET facility A still allowed after B grant", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities/"+facA, nil)
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.GetFacility(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for facility A after B grant, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Step 6: List facilities → both A and B returned ---
	t.Run("list returns both A and B after grant", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
		req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
		rec := httptest.NewRecorder()
		h.ListFacilities(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for facility list after grant, got %d", rec.Code)
		}
		ids := extractFacilityIDsFromListBody(t, rec.Body.Bytes())
		if len(ids) != 2 {
			t.Fatalf("expected 2 facilities in list after grant, got %d: %v", len(ids), ids)
		}
		if !containsString(ids, facA) {
			t.Errorf("expected facility A in list, got: %v", ids)
		}
		if !containsString(ids, facB) {
			t.Errorf("expected facility B in list, got: %v", ids)
		}
	})
}
