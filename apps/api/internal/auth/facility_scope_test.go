package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/sigap/sigap/apps/api/internal/identity"
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
