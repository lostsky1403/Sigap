package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/migrate"
)

type stubFacilityScope struct {
	result FacilityScopeResult
}

func (s stubFacilityScope) AllowedFacilityIDs(context.Context, string) ([]uuid.UUID, error) {
	return s.result.IDs, s.result.Err
}

func (s stubFacilityScope) CanAccessFacility(_ context.Context, _ string, facilityID uuid.UUID) (bool, error) {
	if s.result.Err != nil {
		return false, s.result.Err
	}
	return FacilityScopeResultAllows(s.result, facilityID), nil
}

func (s stubFacilityScope) ResolveFacilityScope(context.Context, string) FacilityScopeResult {
	return s.result
}

func TestFacilityScopeResultSemantics(t *testing.T) {
	facilityID := uuid.New()
	otherFacilityID := uuid.New()
	scopeErr := errors.New("scope unavailable")

	cases := []struct {
		name             string
		actor            identity.Actor
		resolver         FacilityScope
		result           *FacilityScopeResult
		wantIDs          []uuid.UUID
		wantUnrestricted bool
		wantErr          bool
		wantAllows       bool
		checkID          uuid.UUID
	}{
		{
			name:             "dev actor is unrestricted",
			actor:            identity.Actor{IsDev: true},
			resolver:         stubFacilityScope{result: FacilityScopeResult{Err: scopeErr}},
			wantUnrestricted: true,
			wantAllows:       true,
		},
		{
			name:       "active scoped facilities are enforced",
			actor:      identity.Actor{AppUserID: uuid.NewString()},
			resolver:   stubFacilityScope{result: FacilityScopeResult{IDs: []uuid.UUID{facilityID}}},
			wantIDs:    []uuid.UUID{facilityID},
			wantAllows: true,
		},
		{
			name:       "empty scoped facilities fail closed",
			actor:      identity.Actor{AppUserID: uuid.NewString()},
			resolver:   stubFacilityScope{result: FacilityScopeResult{}},
			wantAllows: false,
		},
		{
			name:       "resolver error fails closed",
			actor:      identity.Actor{AppUserID: uuid.NewString()},
			resolver:   stubFacilityScope{result: FacilityScopeResult{Err: scopeErr}},
			wantErr:    true,
			wantAllows: false,
		},
		{
			name:             "global super admin result is unrestricted with empty ids",
			actor:            identity.Actor{AppUserID: uuid.NewString()},
			result:           &FacilityScopeResult{Unrestricted: true},
			wantUnrestricted: true,
			wantAllows:       true,
		},
		{
			name:       "out of scope facility is denied",
			actor:      identity.Actor{AppUserID: uuid.NewString()},
			resolver:   stubFacilityScope{result: FacilityScopeResult{IDs: []uuid.UUID{facilityID}}},
			wantIDs:    []uuid.UUID{facilityID},
			checkID:    otherFacilityID,
			wantAllows: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := tc.resolver
			if tc.result != nil {
				resolver = stubFacilityScope{result: *tc.result}
			}
			result := FacilityScopeForActor(context.Background(), tc.actor, resolver)
			if result.Unrestricted != tc.wantUnrestricted {
				t.Errorf("Unrestricted=%v want %v", result.Unrestricted, tc.wantUnrestricted)
			}
			if len(result.IDs) != len(tc.wantIDs) {
				t.Fatalf("IDs=%v want %v", result.IDs, tc.wantIDs)
			}
			if (result.Err != nil) != tc.wantErr {
				t.Errorf("Err=%v want error=%v", result.Err, tc.wantErr)
			}
			checkID := tc.checkID
			if checkID == uuid.Nil {
				checkID = facilityID
			}
			if got := FacilityScopeResultAllows(result, checkID); got != tc.wantAllows {
				t.Errorf("Allows=%v want %v", got, tc.wantAllows)
			}
			if tc.actor.AppUserID != "" && !tc.actor.IsDev && !tc.wantUnrestricted {
				if got := CanAccessFacilityForActor(context.Background(), tc.actor, resolver, checkID); got != tc.wantAllows {
					t.Errorf("CanAccessFacilityForActor=%v want %v", got, tc.wantAllows)
				}
			}
		})
	}
}

func TestFacilityScopeResultOptionalFacilityFailsClosed(t *testing.T) {
	actor := identity.Actor{AppUserID: uuid.NewString()}
	resolver := stubFacilityScope{result: FacilityScopeResult{}}
	result := FacilityScopeForActor(context.Background(), actor, resolver)

	if FacilityScopeResultAllowsOptional(result, nil) {
		t.Error("nil facility must not become unrestricted for an empty scoped result")
	}

	other := &FacilityScopeResult{Unrestricted: true}
	if !FacilityScopeResultAllowsOptional(*other, nil) {
		t.Error("an explicitly unrestricted result must allow a nil facility")
	}
}

func TestFacilityScope_LocalSeedIdentities(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping local seed identity test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "test_local_seed, public"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS test_local_seed CASCADE`); err != nil {
		t.Fatalf("drop test_local_seed schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA test_local_seed`); err != nil {
		t.Fatalf("create test_local_seed schema: %v", err)
	}

	dir, err := migrate.DefaultDir()
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	if _, err := migrate.Run(ctx, pool, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedDir := filepath.Join(dir, "..", "seed")
	for run := 0; run < 2; run++ {
		if run == 1 {
			if _, err := pool.Exec(ctx, `
				INSERT INTO user_roles (user_id, role_id, facility_id, status, deleted_at)
				SELECT seeded.user_id, r.id, seeded.facility_id, 'active', NULL
				FROM (
					VALUES
						('00000000-0000-0000-0000-00000000d990'::uuid, 'facility_admin', '00000000-0000-0000-0000-00000000d000'::uuid),
						('00000000-0000-0000-0000-00000000d991'::uuid, 'super_admin', NULL::uuid),
						('00000000-0000-0000-0000-00000000d992'::uuid, 'super_admin', NULL::uuid)
				) AS seeded(user_id, role_name, facility_id)
				JOIN roles r ON r.name = seeded.role_name`); err != nil {
				t.Fatalf("insert conflicting synthetic user roles: %v", err)
			}
		}
		for _, name := range []string{"dev.sql", "rbac.sql", "demo.sql"} {
			seed, err := os.ReadFile(filepath.Join(seedDir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if _, err := pool.Exec(ctx, string(seed)); err != nil {
				t.Fatalf("apply %s on run %d: %v", name, run+1, err)
			}
		}
	}

	var globalRole, scopedRole, zeroRole string
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = '00000000-0000-0000-0000-00000000d990'::uuid),
			(SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = '00000000-0000-0000-0000-00000000d991'::uuid),
			(SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = '00000000-0000-0000-0000-00000000d992'::uuid)`).Scan(&globalRole, &scopedRole, &zeroRole); err != nil {
		t.Fatalf("query seeded roles: %v", err)
	}
	if globalRole != "super_admin" || scopedRole != "facility_admin" || zeroRole != "facility_admin" {
		t.Fatalf("seeded roles=%q/%q/%q want super_admin/facility_admin/facility_admin", globalRole, scopedRole, zeroRole)
	}

	var syntheticRoleCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM user_roles
		WHERE user_id IN (
			'00000000-0000-0000-0000-00000000d990'::uuid,
			'00000000-0000-0000-0000-00000000d991'::uuid,
			'00000000-0000-0000-0000-00000000d992'::uuid
		)`).Scan(&syntheticRoleCount); err != nil {
		t.Fatalf("count synthetic user roles: %v", err)
	}
	if syntheticRoleCount != 3 {
		t.Fatalf("synthetic user role count=%d want 3", syntheticRoleCount)
	}

	resolver := NewDBFacilityScope(pool).(FacilityScopeResolver)
	cases := []struct {
		name             string
		userID           string
		wantIDs          []uuid.UUID
		wantUnrestricted bool
	}{
		{
			name:             "global super admin",
			userID:           "00000000-0000-0000-0000-00000000d990",
			wantUnrestricted: true,
		},
		{
			name:   "facility-scoped admin",
			userID: "00000000-0000-0000-0000-00000000d991",
			wantIDs: []uuid.UUID{
				uuid.MustParse("00000000-0000-0000-0000-00000000d000"),
			},
		},
		{
			name:   "zero-scope admin",
			userID: "00000000-0000-0000-0000-00000000d992",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := resolver.ResolveFacilityScope(ctx, tc.userID)
			if result.Err != nil {
				t.Fatalf("resolve facility scope: %v", result.Err)
			}
			if result.Unrestricted != tc.wantUnrestricted {
				t.Fatalf("Unrestricted=%v want %v", result.Unrestricted, tc.wantUnrestricted)
			}
			if len(result.IDs) != len(tc.wantIDs) {
				t.Fatalf("IDs=%v want %v", result.IDs, tc.wantIDs)
			}
			for i := range tc.wantIDs {
				if result.IDs[i] != tc.wantIDs[i] {
					t.Fatalf("IDs[%d]=%v want %v", i, result.IDs[i], tc.wantIDs[i])
				}
			}
		})
	}
}

func TestFacilityScopeResult_DBRestrictsGlobalSuperAdmin(t *testing.T) {
	pool, cleanup := newTestResolverPool(t)
	defer cleanup()
	resolver := NewDBFacilityScope(pool)
	ctx := context.Background()

	cases := []struct {
		name             string
		disableUser      bool
		assignmentStatus string
		deleteAssignment bool
		scopedFacilityID string
		permissions      []string
		wantIDs          int
		wantUnrestricted bool
	}{
		{name: "active global assignment", assignmentStatus: "active", wantUnrestricted: true},
		{name: "inactive global assignment", assignmentStatus: "inactive"},
		{name: "soft deleted global assignment", assignmentStatus: "active", deleteAssignment: true},
		{name: "disabled application user", disableUser: true, assignmentStatus: "active"},
		{name: "facility scoped super admin role", assignmentStatus: "active", scopedFacilityID: uuid.NewString(), wantIDs: 1},
		{name: "client claims only", assignmentStatus: "", permissions: []string{"facility.manage"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subject := "scope-" + uuid.NewString()
			appUserID := seedRBACUser(t, pool, subject)
			if tc.disableUser {
				if _, err := pool.Exec(ctx, `UPDATE app_users SET status = 'disabled' WHERE id = $1`, appUserID); err != nil {
					t.Fatalf("disable app user: %v", err)
				}
			}

			if tc.assignmentStatus != "" {
				var roleID string
				if err := pool.QueryRow(ctx,
					`INSERT INTO roles (name, description)
					 VALUES ('super_admin', 'test global super admin')
					 ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
					 RETURNING id::text`).Scan(&roleID); err != nil {
					t.Fatalf("seed super_admin role: %v", err)
				}

				var facilityID any
				if tc.scopedFacilityID != "" {
					if _, err := pool.Exec(ctx,
						`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, short_code)
						 VALUES ($1, $2, 'puskesmas', 'Jl. Test', 'Kec. Test', 'Kab. Test', 'Prov. Test', '021-12345678', $3)`,
						tc.scopedFacilityID, "Scope "+tc.name, uuid.NewString()[:8]); err != nil {
						t.Fatalf("seed scoped facility: %v", err)
					}
					facilityID = tc.scopedFacilityID
				}

				var query string
				if tc.deleteAssignment {
					query = `INSERT INTO user_roles (user_id, role_id, facility_id, status, deleted_at) VALUES ($1, $2, $3, $4, NOW())`
				} else {
					query = `INSERT INTO user_roles (user_id, role_id, facility_id, status, deleted_at) VALUES ($1, $2, $3, $4, NULL)`
				}
				if _, err := pool.Exec(ctx, query, appUserID, roleID, facilityID, tc.assignmentStatus); err != nil {
					t.Fatalf("seed super_admin assignment: %v", err)
				}
			}

			actor := identity.Actor{
				Type:        identity.ActorUser,
				AppUserID:   appUserID,
				Permissions: tc.permissions,
			}
			result := FacilityScopeForActor(ctx, actor, resolver)
			if result.Err != nil {
				t.Fatalf("resolve scope: %v", result.Err)
			}
			if len(result.IDs) != tc.wantIDs {
				t.Errorf("IDs=%v want %d", result.IDs, tc.wantIDs)
			}
			if result.Unrestricted != tc.wantUnrestricted {
				t.Errorf("Unrestricted=%v want %v", result.Unrestricted, tc.wantUnrestricted)
			}
		})
	}
}
