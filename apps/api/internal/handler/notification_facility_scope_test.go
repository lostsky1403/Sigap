package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/notification"
)

type notificationStaticScope struct {
	result auth.FacilityScopeResult
}

func (s notificationStaticScope) AllowedFacilityIDs(context.Context, string) ([]uuid.UUID, error) {
	return s.result.IDs, s.result.Err
}

func (s notificationStaticScope) CanAccessFacility(_ context.Context, _ string, facilityID uuid.UUID) (bool, error) {
	if s.result.Err != nil {
		return false, s.result.Err
	}
	return auth.FacilityScopeResultAllows(s.result, facilityID), nil
}

func (s notificationStaticScope) ResolveFacilityScope(context.Context, string) auth.FacilityScopeResult {
	return s.result
}

func scopedNotificationsHandler(pool *pgxpool.Pool) *NotificationsHandler {
	return NewNotificationsHandler(notification.NewService(pool)).WithFacilityScopeResolver(auth.NewDBFacilityScope(pool))
}

func notificationRequest(t *testing.T, h *NotificationsHandler, method, path string, actor identity.Actor) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(identity.ContextWithActor(req.Context(), actor))
	rec := httptest.NewRecorder()
	h.NotificationsRouter(rec, req)
	return rec
}

func seedScopeNotification(t *testing.T, pool *pgxpool.Pool, facilityID *string, status notification.Status, attemptCount int) string {
	t.Helper()
	id := uuid.NewString()
	var facility any
	if facilityID != nil {
		facility = *facilityID
	}
	nextAttempt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := pool.Exec(context.Background(), `
INSERT INTO notification_outbox
    (id, facility_id, channel, template_key, subject, body_template,
     recipient_type, recipient_contact_masked, recipient_contact_hash,
     status, attempt_count, next_attempt_at, last_error_code,
     created_at, updated_at)
VALUES ($1, $2, 'dev', $3, 'Subject', 'Body template', 'patient', '+62••••0001',
        decode('1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef', 'hex'),
        $4, $5, $6, 'seed_error', $6, $6)`, id, facility, "scope.notification."+id, status, attemptCount, nextAttempt); err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	return id
}

func notificationRowSnapshot(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var snapshot string
	if err := pool.QueryRow(context.Background(), `
SELECT row_to_json(n)::text
FROM notification_outbox n
WHERE n.id = $1`, id).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot notification %s: %v", id, err)
	}
	return snapshot
}

func notificationCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM notification_outbox`).Scan(&count); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return count
}

func seedNotificationActors(t *testing.T, pool *pgxpool.Pool, facilityA string) (scoped, global, zero identity.Actor) {
	t.Helper()
	ctx := context.Background()
	scopedID := uuid.NewString()
	globalID := uuid.NewString()
	zeroID := uuid.NewString()
	if err := seedAppUser(ctx, pool, scopedID, "notification-scoped"); err != nil {
		t.Fatalf("seed scoped user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, scopedID, facilityA, []string{"notification.read", "notification.manage"}); err != nil {
		t.Fatalf("seed scoped role: %v", err)
	}
	if err := seedAppUser(ctx, pool, globalID, "notification-global"); err != nil {
		t.Fatalf("seed global user: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, globalID); err != nil {
		t.Fatalf("seed global role: %v", err)
	}
	if err := seedAppUser(ctx, pool, zeroID, "notification-zero"); err != nil {
		t.Fatalf("seed zero user: %v", err)
	}
	if err := seedUserRoles(ctx, pool, zeroID, "", []string{"notification.read", "notification.manage"}); err != nil {
		t.Fatalf("seed zero role: %v", err)
	}
	return makeScopedActor(scopedID, "notification.read", "notification.manage"),
		makeScopedActor(globalID, "notification.read", "notification.manage"),
		makeScopedActor(zeroID, "notification.read", "notification.manage")
}

func assertNotificationStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("got status %d want %d: %s", rec.Code, want, rec.Body.String())
	}
}

func assertNotificationRowUnchanged(t *testing.T, pool *pgxpool.Pool, id, before string) {
	t.Helper()
	after := notificationRowSnapshot(t, pool, id)
	if after != before {
		t.Fatalf("notification row %s changed after denied request\nbefore: %s\nafter:  %s", id, before, after)
	}
}

func TestFacilityScope_NotificationRetryAuthorizationOrdering(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()
	facilityA := uuid.NewString()
	facilityB := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facilityA, "Notification Retry A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facilityB, "Notification Retry B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	scopedActor, globalActor, zeroActor := seedNotificationActors(t, pool, facilityA)
	h := scopedNotificationsHandler(pool)

	outOfScopeID := seedScopeNotification(t, pool, &facilityB, notification.StatusFailed, 2)
	inScopeID := seedScopeNotification(t, pool, &facilityA, notification.StatusFailed, 1)
	wrongStateID := seedScopeNotification(t, pool, &facilityA, notification.StatusDelivered, 3)
	globalID := seedScopeNotification(t, pool, &facilityB, notification.StatusFailed, 4)
	zeroID := seedScopeNotification(t, pool, &facilityB, notification.StatusFailed, 5)
	absentID := uuid.NewString()

	t.Run("out of scope retry is denied before mutation", func(t *testing.T) {
		before := notificationRowSnapshot(t, pool, outOfScopeID)
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+outOfScopeID+"/retry", scopedActor)
		assertNotificationStatus(t, rec, http.StatusNotFound)
		assertNotificationRowUnchanged(t, pool, outOfScopeID, before)
	})

	t.Run("in scope retry succeeds and mutates", func(t *testing.T) {
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+inScopeID+"/retry", scopedActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		var status string
		var attemptCount int
		var lastError *string
		if err := pool.QueryRow(ctx, `SELECT status, attempt_count, last_error_code FROM notification_outbox WHERE id = $1`, inScopeID).Scan(&status, &attemptCount, &lastError); err != nil {
			t.Fatalf("read retried notification: %v", err)
		}
		if status != string(notification.StatusPending) || attemptCount != 2 || lastError != nil {
			t.Fatalf("retry state status=%s attempts=%d last_error=%v", status, attemptCount, lastError)
		}
	})

	t.Run("absent id is denied without mutation", func(t *testing.T) {
		before := notificationCount(t, pool)
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+absentID+"/retry", scopedActor)
		assertNotificationStatus(t, rec, http.StatusNotFound)
		if after := notificationCount(t, pool); after != before {
			t.Fatalf("notification count changed for absent id: before=%d after=%d", before, after)
		}
	})

	t.Run("in scope wrong state conflicts without mutation", func(t *testing.T) {
		before := notificationRowSnapshot(t, pool, wrongStateID)
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+wrongStateID+"/retry", scopedActor)
		assertNotificationStatus(t, rec, http.StatusConflict)
		assertNotificationRowUnchanged(t, pool, wrongStateID, before)
	})

	t.Run("active global super admin may retry any facility", func(t *testing.T) {
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+globalID+"/retry", globalActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM notification_outbox WHERE id = $1`, globalID).Scan(&status); err != nil {
			t.Fatalf("read globally retried notification: %v", err)
		}
		if status != string(notification.StatusPending) {
			t.Fatalf("global retry status=%s want pending", status)
		}
	})

	t.Run("zero assignment non super admin cannot retry", func(t *testing.T) {
		before := notificationRowSnapshot(t, pool, zeroID)
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+zeroID+"/retry", zeroActor)
		assertNotificationStatus(t, rec, http.StatusNotFound)
		assertNotificationRowUnchanged(t, pool, zeroID, before)
	})
}

func TestFacilityScope_NotificationCancelAuthorizationOrdering(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()
	facilityA := uuid.NewString()
	facilityB := uuid.NewString()
	if err := seedFacilityByName(ctx, pool, facilityA, "Notification Cancel A"); err != nil {
		t.Fatalf("seed facility A: %v", err)
	}
	if err := seedFacilityByName(ctx, pool, facilityB, "Notification Cancel B"); err != nil {
		t.Fatalf("seed facility B: %v", err)
	}
	scopedActor, globalActor, zeroActor := seedNotificationActors(t, pool, facilityA)
	h := scopedNotificationsHandler(pool)

	outOfScopeID := seedScopeNotification(t, pool, &facilityB, notification.StatusPending, 1)
	inScopeID := seedScopeNotification(t, pool, &facilityA, notification.StatusFailed, 2)
	deliveredID := seedScopeNotification(t, pool, &facilityA, notification.StatusDelivered, 3)
	globalID := seedScopeNotification(t, pool, &facilityB, notification.StatusPending, 4)
	zeroID := seedScopeNotification(t, pool, &facilityB, notification.StatusFailed, 5)

	t.Run("out of scope cancel is denied before mutation", func(t *testing.T) {
		before := notificationRowSnapshot(t, pool, outOfScopeID)
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+outOfScopeID+"/cancel", scopedActor)
		assertNotificationStatus(t, rec, http.StatusNotFound)
		assertNotificationRowUnchanged(t, pool, outOfScopeID, before)
	})

	t.Run("in scope cancel succeeds and remains idempotent", func(t *testing.T) {
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+inScopeID+"/cancel", scopedActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		rec = notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+inScopeID+"/cancel", scopedActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM notification_outbox WHERE id = $1`, inScopeID).Scan(&status); err != nil {
			t.Fatalf("read cancelled notification: %v", err)
		}
		if status != string(notification.StatusCancelled) {
			t.Fatalf("cancel status=%s want cancelled", status)
		}
	})

	t.Run("in scope delivered cancel conflicts without mutation", func(t *testing.T) {
		before := notificationRowSnapshot(t, pool, deliveredID)
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+deliveredID+"/cancel", scopedActor)
		assertNotificationStatus(t, rec, http.StatusConflict)
		assertNotificationRowUnchanged(t, pool, deliveredID, before)
	})

	t.Run("active global super admin may cancel any facility", func(t *testing.T) {
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+globalID+"/cancel", globalActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM notification_outbox WHERE id = $1`, globalID).Scan(&status); err != nil {
			t.Fatalf("read globally cancelled notification: %v", err)
		}
		if status != string(notification.StatusCancelled) {
			t.Fatalf("global cancel status=%s want cancelled", status)
		}
	})

	t.Run("zero assignment non super admin cannot cancel", func(t *testing.T) {
		before := notificationRowSnapshot(t, pool, zeroID)
		rec := notificationRequest(t, h, http.MethodPost, "/api/v1/admin/notifications/"+zeroID+"/cancel", zeroActor)
		assertNotificationStatus(t, rec, http.StatusNotFound)
		assertNotificationRowUnchanged(t, pool, zeroID, before)
	})
}

func TestFacilityScope_NotificationSummaryActorMatrix(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()
	facilityA := uuid.NewString()
	facilityB := uuid.NewString()
	for _, facility := range []struct{ id, name string }{{facilityA, "Notification Summary A"}, {facilityB, "Notification Summary B"}} {
		if err := seedFacilityByName(ctx, pool, facility.id, facility.name); err != nil {
			t.Fatalf("seed facility: %v", err)
		}
	}
	for _, status := range []notification.Status{
		notification.StatusPending,
		notification.StatusProcessing,
		notification.StatusDelivered,
		notification.StatusFailed,
		notification.StatusCancelled,
	} {
		seedScopeNotification(t, pool, &facilityA, status, 1)
	}
	seedScopeNotification(t, pool, &facilityB, notification.StatusFailed, 1)
	seedScopeNotification(t, pool, nil, notification.StatusDelivered, 1)

	scopedActor, globalActor, zeroActor := seedNotificationActors(t, pool, facilityA)
	inactiveID := uuid.NewString()
	deletedID := uuid.NewString()
	claimsOnlyID := uuid.NewString()
	for _, user := range []struct{ id, subject string }{{inactiveID, "notification-inactive"}, {deletedID, "notification-deleted"}, {claimsOnlyID, "notification-claims-only"}} {
		if err := seedAppUser(ctx, pool, user.id, user.subject); err != nil {
			t.Fatalf("seed app user: %v", err)
		}
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, inactiveID); err != nil {
		t.Fatalf("seed inactive global role: %v", err)
	}
	if err := setUserRoleLifecycle(ctx, pool, inactiveID, "", "inactive", nil); err != nil {
		t.Fatalf("set inactive global role: %v", err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, deletedID); err != nil {
		t.Fatalf("seed deleted global role: %v", err)
	}
	deletedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := setUserRoleLifecycle(ctx, pool, deletedID, "", "active", &deletedAt); err != nil {
		t.Fatalf("soft delete global role: %v", err)
	}
	inactiveActor := makeScopedActor(inactiveID, "notification.read")
	deletedActor := makeScopedActor(deletedID, "notification.read")
	claimsOnlyActor := makeScopedActor(claimsOnlyID, "notification.read", "notification.manage")
	devActor := makeDevActor(uuid.NewString(), "notification.read")

	facilityAExpected := notification.ZeroSummary()
	facilityAExpected[string(notification.StatusPending)] = 1
	facilityAExpected[string(notification.StatusProcessing)] = 1
	facilityAExpected[string(notification.StatusDelivered)] = 1
	facilityAExpected[string(notification.StatusFailed)] = 1
	facilityAExpected[string(notification.StatusCancelled)] = 1
	globalExpected := notification.ZeroSummary()
	globalExpected[string(notification.StatusPending)] = 1
	globalExpected[string(notification.StatusProcessing)] = 1
	globalExpected[string(notification.StatusDelivered)] = 2
	globalExpected[string(notification.StatusFailed)] = 2
	globalExpected[string(notification.StatusCancelled)] = 1
	zeroExpected := notification.ZeroSummary()

	cases := []struct {
		name       string
		handler    *NotificationsHandler
		actor      identity.Actor
		query      string
		expected   map[string]int
		wantStatus int
	}{
		{name: "facility scoped actor sees own facility", handler: scopedNotificationsHandler(pool), actor: scopedActor, expected: facilityAExpected, wantStatus: http.StatusOK},
		{name: "facility scoped actor filter stays scoped", handler: scopedNotificationsHandler(pool), actor: scopedActor, query: "?facility_id=" + facilityA, expected: facilityAExpected, wantStatus: http.StatusOK},
		{name: "dev actor sees global counts", handler: scopedNotificationsHandler(pool), actor: devActor, expected: globalExpected, wantStatus: http.StatusOK},
		{name: "active global super admin sees global counts", handler: scopedNotificationsHandler(pool), actor: globalActor, expected: globalExpected, wantStatus: http.StatusOK},
		{name: "global super admin filter narrows explicitly", handler: scopedNotificationsHandler(pool), actor: globalActor, query: "?facility_id=" + facilityA, expected: facilityAExpected, wantStatus: http.StatusOK},
		{name: "zero assignment non super admin is all zero", handler: scopedNotificationsHandler(pool), actor: zeroActor, expected: zeroExpected, wantStatus: http.StatusOK},
		{name: "inactive global super admin is all zero", handler: scopedNotificationsHandler(pool), actor: inactiveActor, expected: zeroExpected, wantStatus: http.StatusOK},
		{name: "soft deleted global super admin is all zero", handler: scopedNotificationsHandler(pool), actor: deletedActor, expected: zeroExpected, wantStatus: http.StatusOK},
		{name: "claim only permissions do not grant scope", handler: scopedNotificationsHandler(pool), actor: claimsOnlyActor, expected: zeroExpected, wantStatus: http.StatusOK},
		{name: "resolver failure is all zero", handler: NewNotificationsHandler(notification.NewService(pool)).WithFacilityScopeResolver(notificationStaticScope{result: auth.FacilityScopeResult{Err: errors.New("scope unavailable")}}), actor: scopedActor, expected: zeroExpected, wantStatus: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := notificationRequest(t, tc.handler, http.MethodGet, "/api/v1/admin/notifications/summary"+tc.query, tc.actor)
			assertNotificationStatus(t, rec, tc.wantStatus)
			var response struct {
				Data map[string]int `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode summary: %v", err)
			}
			if !reflect.DeepEqual(response.Data, tc.expected) {
				t.Fatalf("summary=%v want=%v", response.Data, tc.expected)
			}
		})
	}
}

func TestFacilityScope_NotificationListPostFetchFilter(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()
	facilityA := uuid.NewString()
	facilityB := uuid.NewString()
	for _, facility := range []struct{ id, name string }{{facilityA, "Notification List A"}, {facilityB, "Notification List B"}} {
		if err := seedFacilityByName(ctx, pool, facility.id, facility.name); err != nil {
			t.Fatalf("seed facility: %v", err)
		}
	}
	seedScopeNotification(t, pool, &facilityA, notification.StatusPending, 1)
	seedScopeNotification(t, pool, &facilityB, notification.StatusFailed, 1)
	seedScopeNotification(t, pool, nil, notification.StatusDelivered, 1)
	scopedActor, globalActor, zeroActor := seedNotificationActors(t, pool, facilityA)

	decode := func(t *testing.T, rec *httptest.ResponseRecorder) []notification.OutboxRow {
		t.Helper()
		var response struct {
			Data []notification.OutboxRow `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode notification list: %v", err)
		}
		return response.Data
	}
	assertFacilities := func(t *testing.T, rows []notification.OutboxRow, allowed map[string]bool) {
		t.Helper()
		if len(rows) != len(allowed) {
			t.Fatalf("notification rows=%d want=%d", len(rows), len(allowed))
		}
		for _, row := range rows {
			if row.FacilityID == nil || !allowed[row.FacilityID.String()] {
				t.Fatalf("out-of-scope notification row: %+v", row)
			}
		}
	}

	t.Run("scoped post fetch filter keeps only allowed facility", func(t *testing.T) {
		rec := notificationRequest(t, scopedNotificationsHandler(pool), http.MethodGet, "/api/v1/admin/notifications", scopedActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		assertFacilities(t, decode(t, rec), map[string]bool{facilityA: true})
	})

	t.Run("out of scope supplied filter is ignored without widening scope", func(t *testing.T) {
		rec := notificationRequest(t, scopedNotificationsHandler(pool), http.MethodGet, "/api/v1/admin/notifications?facility_id="+facilityB, scopedActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		assertFacilities(t, decode(t, rec), map[string]bool{facilityA: true})
	})

	t.Run("global empty ids remain unrestricted after post fetch filter", func(t *testing.T) {
		rec := notificationRequest(t, scopedNotificationsHandler(pool), http.MethodGet, "/api/v1/admin/notifications", globalActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		if got := len(decode(t, rec)); got != 3 {
			t.Fatalf("global notification rows=%d want 3", got)
		}
	})

	t.Run("zero assignment returns a non nil empty list", func(t *testing.T) {
		rec := notificationRequest(t, scopedNotificationsHandler(pool), http.MethodGet, "/api/v1/admin/notifications", zeroActor)
		assertNotificationStatus(t, rec, http.StatusOK)
		rows := decode(t, rec)
		if rows == nil || len(rows) != 0 {
			t.Fatalf("expected non-nil empty list, got %#v", rows)
		}
	})
}

func TestFacilityScope_NotificationDetailMatrix(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()
	facilityA := uuid.NewString()
	facilityB := uuid.NewString()
	for _, facility := range []struct{ id, name string }{{facilityA, "Notification Detail A"}, {facilityB, "Notification Detail B"}} {
		if err := seedFacilityByName(ctx, pool, facility.id, facility.name); err != nil {
			t.Fatalf("seed facility: %v", err)
		}
	}
	rowA := seedScopeNotification(t, pool, &facilityA, notification.StatusPending, 1)
	rowB := seedScopeNotification(t, pool, &facilityB, notification.StatusPending, 1)
	scopedActor, globalActor, zeroActor := seedNotificationActors(t, pool, facilityA)

	cases := []struct {
		name   string
		actor  identity.Actor
		id     string
		status int
	}{
		{name: "scoped actor reads own facility", actor: scopedActor, id: rowA, status: http.StatusOK},
		{name: "scoped actor cannot read other facility", actor: scopedActor, id: rowB, status: http.StatusNotFound},
		{name: "global super admin reads any facility", actor: globalActor, id: rowB, status: http.StatusOK},
		{name: "zero assignment cannot read", actor: zeroActor, id: rowB, status: http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := notificationRequest(t, scopedNotificationsHandler(pool), http.MethodGet, "/api/v1/admin/notifications/"+tc.id, tc.actor)
			assertNotificationStatus(t, rec, tc.status)
		})
	}
}
