package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/notification"
)

// NotificationsHandler exposes the four admin endpoints that drive the
// notification outbox. All endpoints sit behind RequirePermission gates
// declared in router.go; this handler does not perform its own authn or
// authz.
//
// Endpoints:
//
//	GET  /api/v1/admin/notifications              -> ListNotifications
//	GET  /api/v1/admin/notifications/{id}         -> GetNotification
//	POST /api/v1/admin/notifications/{id}/retry  -> RetryNotification
//	POST /api/v1/admin/notifications/{id}/cancel -> CancelNotification
type NotificationsHandler struct {
	svc           *notification.Service
	audit         *audit.Service
	scopeResolver auth.FacilityScope
	grantResolver auth.FacilityGrantResolver
}

// NewNotificationsHandler constructs the handler bound to the given
// service. The service must not be nil.
func NewNotificationsHandler(svc *notification.Service) *NotificationsHandler {
	if svc == nil {
		panic("handler.NewNotificationsHandler: nil service")
	}
	return &NotificationsHandler{svc: svc}
}

// WithFacilityScopeResolver attaches the facility scope resolver used to
// enforce facility-scoped authorization on notification endpoints. When nil,
// facility scope resolution fails closed (empty scope) — callers must wire
// the resolver in production.
func (h *NotificationsHandler) WithFacilityScopeResolver(r auth.FacilityScope) *NotificationsHandler {
	h.scopeResolver = r
	// Keep permission provenance as live as the scope, so a granted or revoked
	// notification.read takes effect on the very next request rather than at
	// the next re-authentication.
	if _, ok := r.(auth.DBFacilityScopeMarker); ok {
		h.grantResolver = auth.NewDBFacilityGrantResolver(h.svc.Pool())
	}
	return h
}

// WithFacilityGrantResolver attaches a live facility grant resolver. When nil,
// notification read authorization falls back to the actor's authentication-time
// grants, which is correct but not immediate.
func (h *NotificationsHandler) WithFacilityGrantResolver(r auth.FacilityGrantResolver) *NotificationsHandler {
	h.grantResolver = r
	return h
}

// readSet resolves the facilities whose notification.read is authorized for this
// actor, using live provenance, and returns the actor carrying those grants.
func (h *NotificationsHandler) readSet(r *http.Request, actor identity.Actor, permission string) (identity.Actor, auth.FacilityScopeResult, auth.FacilityReadSet) {
	live, err := auth.ActorWithLiveGrants(r.Context(), actor, h.grantResolver)
	if err != nil {
		return actor, auth.FacilityScopeResult{Err: err}, auth.FacilityReadSet{Err: err}
	}
	scope := auth.FacilityScopeForActor(r.Context(), live, h.scopeResolver)
	return live, scope, auth.AuthorizedFacilityIDsForPermission(live, scope, permission)
}

// WithAudit attaches an optional audit service for access logging.
func (h *NotificationsHandler) WithAudit(a *audit.Service) *NotificationsHandler {
	h.audit = a
	return h
}

// ListNotifications handles GET /api/v1/admin/notifications with an
// optional ?facility_id= and ?limit= query. The response is JSON with the
// masked contact for every row. The raw contact, the hash, and any
// other PII are NEVER serialised.
//
// Facility authorization: client-supplied ?facility_id= is never trusted. The
// query is restricted to the facilities where notification.read is GRANTED,
// which is the intersection of the actor's scope and its notification.read
// provenance. Corrected scope alone is not sufficient: an actor scoped to
// facilities A and B whose notification.read comes only from B must see B
// notifications and never A. A client-supplied facility_id is only honoured
// when it survives that intersection; otherwise it is silently ignored.
func (h *NotificationsHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	_, _, readSet := h.readSet(r, actor, "notification.read")
	if readSet.Err != nil || !readSet.HasFacilities() {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": []notification.OutboxRow{}})
		detail := "no facilities authorized for notification.read"
		status := "ok"
		if readSet.Err != nil {
			detail = "scope resolution failed"
			status = "error"
		}
		h.log(actor, "notification.list", status, detail)
		return
	}

	facilityIDs := readSet.IDs
	unrestricted := readSet.Unrestricted
	if v := r.URL.Query().Get("facility_id"); v != "" {
		if parsed, err := uuid.Parse(v); err == nil && readSetContains(readSet, parsed) {
			facilityIDs = []uuid.UUID{parsed}
			unrestricted = false
		}
	}

	status := r.URL.Query().Get("status")
	if status != "" && !isAllowedStatus(status) {
		writeError(w, http.StatusBadRequest, "Status tidak valid.")
		h.log(actor, "notification.list", "error", "invalid status")
		return
	}
	channel := r.URL.Query().Get("channel")
	if channel != "" && !isAllowedChannel(channel) {
		writeError(w, http.StatusBadRequest, "Channel tidak valid.")
		h.log(actor, "notification.list", "error", "invalid channel")
		return
	}
	templateKey := strings.TrimSpace(r.URL.Query().Get("template_key"))
	if len(templateKey) > 128 {
		writeError(w, http.StatusBadRequest, "template_key terlalu panjang (maks 128 karakter).")
		h.log(actor, "notification.list", "error", "template_key too long")
		return
	}
	createdFrom, err := parseOptionalTime(r, "created_from")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		h.log(actor, "notification.list", "error", "invalid created_from")
		return
	}
	createdTo, err := parseOptionalTime(r, "created_to")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		h.log(actor, "notification.list", "error", "invalid created_to")
		return
	}

	rawRows, err := h.svc.List(r.Context(), notification.ListParams{
		FacilityIDs:  facilityIDs,
		Unrestricted: unrestricted,
		Limit:        limit,
		Status:       status,
		Channel:      channel,
		TemplateKey:  templateKey,
		CreatedFrom:  createdFrom,
		CreatedTo:    createdTo,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data notifikasi.")
		h.log(actor, "notification.list", "error", err.Error())
		return
	}

	rows := rawRows
	if !unrestricted {
		rows = make([]notification.OutboxRow, 0, len(rawRows))
		for _, row := range rawRows {
			if readSetAllowsOptional(readSet, row.FacilityID) {
				rows = append(rows, row)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": rows})
	h.log(actor, "notification.list", "ok", fmt.Sprintf("count=%d", len(rows)))
}

// GetNotificationSummary handles GET /api/v1/admin/notifications/summary
// and returns aggregated per-status counts as a flat object. The shape
// is a map[string]int keyed by status, e.g. {"pending":5,"delivered":23,...}.
// Every declared status is always present in the response (zero when
// no rows match), so the UI can render the full card set without a
// second pass.
//
// Facility authorization: the summary aggregates ONLY the facilities where
// both conditions hold — the facility is in the actor's scope AND
// notification.read is granted at that facility. Corrected scope alone is not
// sufficient, because the scope ID set can be wider than the set of facilities
// the actor actually holds notification.read for. A zero-assignment
// non-super_admin therefore gets an all-zero summary, never an error and never
// another facility's counts.
func (h *NotificationsHandler) GetNotificationSummary(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())

	_, _, readSet := h.readSet(r, actor, "notification.read")
	if readSet.Err != nil || !readSet.HasFacilities() {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": notification.ZeroSummary()})
		detail := "no facilities authorized for notification.read"
		status := "ok"
		if readSet.Err != nil {
			detail = "scope resolution failed"
			status = "error"
		}
		h.log(actor, "notification.summary", status, detail)
		return
	}

	facilityIDs := readSet.IDs
	unrestricted := readSet.Unrestricted
	if v := r.URL.Query().Get("facility_id"); v != "" {
		if parsed, err := uuid.Parse(v); err == nil && readSetContains(readSet, parsed) {
			facilityIDs = []uuid.UUID{parsed}
			unrestricted = false
		}
	}

	counts, err := h.svc.Summary(r.Context(), notification.SummaryParams{
		FacilityIDs:  facilityIDs,
		Unrestricted: unrestricted,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil ringkasan notifikasi.")
		h.log(actor, "notification.summary", "error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": counts})
	h.log(actor, "notification.summary", "ok", "")
}

// readSetContains reports whether a facility survived the scope AND
// notification.read-provenance intersection. It is the membership test for a
// client-supplied ?facility_id=, and it never widens the authorized set.
func readSetContains(set auth.FacilityReadSet, facilityID uuid.UUID) bool {
	if set.Err != nil {
		return false
	}
	if set.Unrestricted {
		return true
	}
	for _, id := range set.IDs {
		if id == facilityID {
			return true
		}
	}
	return false
}

// readSetAllowsOptional is readSetContains for a nullable stored facility_id. A
// row with no facility is readable only by an unrestricted global grant.
func readSetAllowsOptional(set auth.FacilityReadSet, facilityID *uuid.UUID) bool {
	if set.Err != nil {
		return false
	}
	if set.Unrestricted {
		return true
	}
	if facilityID == nil {
		return false
	}
	return readSetContains(set, *facilityID)
}

func isAllowedStatus(s string) bool {
	for _, v := range notification.AllStatuses() {
		if v == s {
			return true
		}
	}
	return false
}

func isAllowedChannel(c string) bool {
	for _, v := range notification.AllChannels() {
		if v == c {
			return true
		}
	}
	return false
}

// parseOptionalTime reads an RFC3339 timestamp from the named query
// parameter. An empty value yields a zero time.Time ("no bound"). An
// invalid value returns an error with a user-friendly Indonesian
// message — the handler maps that to a 400 response.
func parseOptionalTime(r *http.Request, name string) (time.Time, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("Format tanggal tidak valid. Gunakan ISO 8601 (YYYY-MM-DDTHH:mm:ssZ).")
	}
	return t, nil
}

// GetNotification handles GET /api/v1/admin/notifications/{id}.
// Facility authorization: the notification's stored facility must be in the
// actor's scope AND carry notification.read provenance for that same facility.
// A row at a facility where the actor holds no notification.read returns 404
// (no existence leak) even when the facility itself is in scope.
func (h *NotificationsHandler) GetNotification(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id, ok := extractNotificationID(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "ID notifikasi tidak valid.")
		h.log(actor, "notification.get", "error", "invalid id")
		return
	}
	row, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, notification.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
			h.log(actor, "notification.get", "error", "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil notifikasi.")
		h.log(actor, "notification.get", "error", err.Error())
		return
	}
	liveActor, scope, _ := h.readSet(r, actor, "notification.read")
	if decision := auth.AuthorizeFacilityRead(liveActor, scope, "notification.read", row.FacilityID); !decision.Allowed {
		writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
		h.log(actor, "notification.get", "forbidden", decision.Reason)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": row})
	h.log(actor, "notification.get", "ok", "")
}

// RetryNotification handles POST /api/v1/admin/notifications/{id}/retry.
// Returns 409 Conflict when the current status is not in {failed, pending}.
// Returns 404 when the row does not exist or is out of scope.
func (h *NotificationsHandler) RetryNotification(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id, ok := extractNotificationID(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "ID notifikasi tidak valid.")
		h.log(actor, "notification.retry", "error", "invalid id")
		return
	}
	row, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, notification.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
			h.log(actor, "notification.retry", "error", "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil notifikasi.")
		h.log(actor, "notification.retry", "error", err.Error())
		return
	}
	scope := auth.FacilityScopeForActor(r.Context(), actor, h.scopeResolver)
	if !auth.FacilityScopeResultAllowsOptional(scope, row.FacilityID) {
		writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
		h.log(actor, "notification.retry", "forbidden", "out_of_scope")
		return
	}
	// Scope alone is not enough: notification.manage must be held at the
	// facility the row belongs to, so a manage grant at another facility
	// cannot drive a retry here.
	if row.FacilityID != nil {
		if decision := auth.AuthorizeFacilityMutation(actor, scope, "notification.manage", *row.FacilityID); !decision.Allowed {
			writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
			h.log(actor, "notification.retry", "forbidden", decision.Reason)
			return
		}
	}

	row, err = h.svc.Retry(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, notification.ErrNotFound):
			writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
			h.log(actor, "notification.retry", "error", "not found")
		case errors.Is(err, notification.ErrInvalidState):
			writeError(w, http.StatusConflict, "Retry tidak diizinkan untuk status saat ini.")
			h.log(actor, "notification.retry", "conflict", "invalid state")
		default:
			writeError(w, http.StatusInternalServerError, "Gagal melakukan retry.")
			h.log(actor, "notification.retry", "error", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": row})
	h.log(actor, "notification.retry", "ok", "")
}

// CancelNotification handles POST /api/v1/admin/notifications/{id}/cancel.
// Idempotent on already-cancelled rows. Returns 409 Conflict on delivered.
// Facility scope: the notification must belong to an allowed facility.
func (h *NotificationsHandler) CancelNotification(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id, ok := extractNotificationID(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "ID notifikasi tidak valid.")
		h.log(actor, "notification.cancel", "error", "invalid id")
		return
	}
	row, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, notification.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
			h.log(actor, "notification.cancel", "error", "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil notifikasi.")
		h.log(actor, "notification.cancel", "error", err.Error())
		return
	}
	scope := auth.FacilityScopeForActor(r.Context(), actor, h.scopeResolver)
	if !auth.FacilityScopeResultAllowsOptional(scope, row.FacilityID) {
		writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
		h.log(actor, "notification.cancel", "forbidden", "out_of_scope")
		return
	}
	// Scope alone is not enough: notification.manage must be held at the
	// facility the row belongs to, so a manage grant at another facility
	// cannot drive a cancel here.
	if row.FacilityID != nil {
		if decision := auth.AuthorizeFacilityMutation(actor, scope, "notification.manage", *row.FacilityID); !decision.Allowed {
			writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
			h.log(actor, "notification.cancel", "forbidden", decision.Reason)
			return
		}
	}

	row, err = h.svc.Cancel(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, notification.ErrNotFound):
			writeError(w, http.StatusNotFound, "Notifikasi tidak ditemukan.")
			h.log(actor, "notification.cancel", "error", "not found")
		case errors.Is(err, notification.ErrInvalidState):
			writeError(w, http.StatusConflict, "Cancel tidak diizinkan untuk status saat ini.")
			h.log(actor, "notification.cancel", "conflict", "invalid state")
		default:
			writeError(w, http.StatusInternalServerError, "Gagal melakukan cancel.")
			h.log(actor, "notification.cancel", "error", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": row})
	h.log(actor, "notification.cancel", "ok", "")
}

// NotificationsRouter dispatches /api/v1/admin/notifications/* to the
// right handler based on path and method.
func (h *NotificationsHandler) NotificationsRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// Strip the prefix.
	const prefix = "/api/v1/admin/notifications/"
	rest := strings.TrimPrefix(path, prefix)
	if rest == path {
		// No prefix match → exactly /api/v1/admin/notifications.
		rest = ""
	}
	switch r.Method {
	case http.MethodGet:
		if rest == "" || rest == "/" {
			h.ListNotifications(w, r)
			return
		}
		if rest == "summary" {
			h.GetNotificationSummary(w, r)
			return
		}
		id, ok := parseNotificationIDFromTail(rest)
		if !ok {
			writeError(w, http.StatusNotFound, "Endpoint tidak ditemukan.")
			return
		}
		// Rewrite the URL so the per-id handler can extract it.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/api/v1/admin/notifications/" + id.String()
		h.serveByID(w, r2, id, "get")
	case http.MethodPost:
		// /retry or /cancel
		switch {
		case strings.HasSuffix(rest, "/retry"):
			id, ok := parseNotificationIDFromTail(strings.TrimSuffix(rest, "/retry"))
			if !ok {
				writeError(w, http.StatusNotFound, "Endpoint tidak ditemukan.")
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/api/v1/admin/notifications/" + id.String()
			h.serveByID(w, r2, id, "retry")
		case strings.HasSuffix(rest, "/cancel"):
			id, ok := parseNotificationIDFromTail(strings.TrimSuffix(rest, "/cancel"))
			if !ok {
				writeError(w, http.StatusNotFound, "Endpoint tidak ditemukan.")
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/api/v1/admin/notifications/" + id.String()
			h.serveByID(w, r2, id, "cancel")
		default:
			writeError(w, http.StatusNotFound, "Endpoint tidak ditemukan.")
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *NotificationsHandler) serveByID(w http.ResponseWriter, r *http.Request, id uuid.UUID, op string) {
	switch op {
	case "get":
		// Re-dispatch to GetNotification by parsing the path again.
		// GetNotification already extracts the id from the path.
		h.GetNotification(w, r)
	case "retry":
		h.RetryNotification(w, r)
	case "cancel":
		h.CancelNotification(w, r)
	default:
		writeError(w, http.StatusNotFound, "Endpoint tidak ditemukan.")
	}
}

// extractNotificationID parses the {id} segment from a path of the
// form /api/v1/admin/notifications/{id}. It returns the UUID and ok=true
// on success.
func extractNotificationID(path string) (uuid.UUID, bool) {
	const prefix = "/api/v1/admin/notifications/"
	rest := strings.TrimPrefix(path, prefix)
	if rest == path {
		return uuid.Nil, false
	}
	// Reject paths with sub-segments (we expect exactly /{id}).
	if strings.Contains(rest, "/") {
		return uuid.Nil, false
	}
	return parseNotificationIDFromTail(rest)
}

func parseNotificationIDFromTail(rest string) (uuid.UUID, bool) {
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(rest)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// log writes a sanitised audit event for a notification admin action.
// Metadata keys are restricted to the safe set documented in the spec;
// raw contact, recipient hash, patient display name, and message body
// MUST NEVER appear here.
func (h *NotificationsHandler) log(actor identity.Actor, action, status, detail string) {
	if h.audit == nil {
		return
	}
	metadata := map[string]any{
		"status": status,
	}
	if detail != "" {
		metadata["detail"] = detail
	}
	h.audit.LogEvent(context.Background(), audit.Event{
		Action:       action,
		ResourceType: "notification",
		ActorType:    string(actor.Type),
		ActorUserID:  actor.UserID,
		Metadata:     audit.SanitizeMetadata(metadata),
	})
}
