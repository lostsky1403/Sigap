package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// This file adds the organization directory and overview handlers. They are
// runtime-complete but their net/http route registration lives in
// cmd/server/main.go (outside the scope of this change's target files). Until
// `registerAdminRoutes` registers `/api/v1/admin/organizations`, these
// handlers are reachable only through the handler tests and the
// OrganizationsRouter method below.

// organizationResponse is the tenant directory row. Non-sensitive metadata only:
// no patient data, no PHI.
type organizationResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Region    string     `json:"region"`
	IsActive  bool       `json:"is_active"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// organizationOverview is the tenant-aware operational stats aggregate for one
// organization. Every count is derived from facilities.organization_id and is
// an integer aggregate — never a patient record, never a medical record, never
// PII.
type organizationOverview struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Region            string `json:"region"`
	IsActive          bool   `json:"is_active"`
	FacilityCount     int    `json:"facility_count"`
	ServiceUnitCount  int    `json:"service_unit_count"`
	ActiveQueueCount  int    `json:"active_queue_count"`
	AppointmentCount  int    `json:"appointment_count"`
}

// extractOrganizationID parses the organization UUID from paths like
// /api/v1/admin/organizations/{id}.
func extractOrganizationID(path string) string {
	prefix := "/api/v1/admin/organizations/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	if i := strings.IndexByte(remainder, '/'); i >= 0 {
		remainder = remainder[:i]
	}
	remainder = strings.TrimSpace(remainder)
	if _, err := uuid.Parse(remainder); err != nil {
		return ""
	}
	return remainder
}

// logOrgAccess records an audit event for an organization endpoint access with
// privacy-safe metadata that carries the actor and the resolved scope. The org
// id travels in metadata (there is no audit_events.organization_id column yet;
// see regional-architecture-rfc.md §2.7).
func (h *AdminHandler) logOrgAccess(r *http.Request, actor identity.Actor, action, status, detail, organizationID string) {
	if h.audit == nil {
		return
	}
	md := audit.SanitizeMetadata(map[string]any{
		"status": status,
		"detail": detail,
	})
	if organizationID != "" {
		md["organization_id"] = organizationID
	}
	h.audit.LogEvent(r.Context(), audit.Event{
		Action:       action,
		ResourceType: "organization",
		ResourceID:   organizationID,
		ActorType:    string(actor.Type),
		ActorUserID:  actor.AuditUserID(),
		RequestID:    identity.RequestIDFromContext(r.Context()),
		Metadata:     md,
	})
}

// ListOrganizations handles GET /api/v1/admin/organizations.
//
// Two independent gates apply. The route-level organization.read check is
// COARSE. This handler then narrows to the organizations actually in the
// actor's organization scope (an org-scoped organization_admin sees only its
// own org; a regional_admin/super_admin sees every org).
func (h *AdminHandler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if !actor.IsDev && !actor.HasPermission("organization.read") {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": []organizationResponse{}})
		h.logOrgAccess(r, actor, "organization.list", "denied", "missing organization.read", "")
		return
	}

	scope := auth.OrganizationScopeForActor(ctx, actor, h.orgScope)
	if scope.Err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": []organizationResponse{}})
		h.logOrgAccess(r, actor, "organization.list", "error", "scope resolution failed", "")
		return
	}

	var rows pgx.Rows
	var err error
	if scope.Unrestricted {
		rows, err = h.pool.Query(ctx,
			`SELECT id, name, region, is_active, created_at, updated_at
			 FROM organizations ORDER BY name`)
	} else if len(scope.OrgIDs) > 0 {
		rows, err = h.pool.Query(ctx,
			`SELECT id, name, region, is_active, created_at, updated_at
			 FROM organizations WHERE id = ANY($1) ORDER BY name`, scope.OrgIDs)
	} else {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": []organizationResponse{}})
		h.logOrgAccess(r, actor, "organization.list", "ok", "no organizations in scope", "")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data organisasi.")
		h.logOrgAccess(r, actor, "organization.list", "error", err.Error(), "")
		return
	}
	defer rows.Close()

	results := []organizationResponse{}
	for rows.Next() {
		var o organizationResponse
		if err := rows.Scan(&o.ID, &o.Name, &o.Region, &o.IsActive, &o.CreatedAt, &o.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data organisasi.")
			h.logOrgAccess(r, actor, "organization.list", "error", err.Error(), "")
			return
		}
		results = append(results, o)
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": results})
	h.logOrgAccess(r, actor, "organization.list", "ok", "count:"+itoa(len(results)), "")
}

// GetOrganizationOverview handles GET /api/v1/admin/organizations/{id}.
//
// Authorization is deny-by-default and re-resolved on every request:
//
//  1. coarse organization.read gate;
//  2. organization scope resolution — the target org must be inside the
//     actor's org scope, otherwise 404 (existence is not leaked across orgs);
//  3. facility-scoped read set resolution for "facility.read", which bounds
//     every aggregate to the facilities the actor is actually authorized to
//     observe.
//
// The overview returns non-sensitive integer aggregates only. There is no
// medical-record read anywhere in this path (that permission does not exist).
func (h *AdminHandler) GetOrganizationOverview(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id := extractOrganizationID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID organisasi tidak valid.")
		h.logOrgAccess(r, actor, "organization.get", "error", "missing id", "")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if !actor.IsDev && !actor.HasPermission("organization.read") {
		writeError(w, http.StatusNotFound, "Organisasi tidak ditemukan.")
		h.logOrgAccess(r, actor, "organization.get", "not_found", "missing organization.read", id)
		return
	}

	target, err := uuid.Parse(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID organisasi tidak valid.")
		h.logOrgAccess(r, actor, "organization.get", "error", "invalid id", id)
		return
	}

	// Phase 2: the target organization must be inside the actor's org scope. A
	// forged organization_id for a sibling org is denied here, before any query
	// about that org.
	orgScope := auth.OrganizationScopeForActor(ctx, actor, h.orgScope)
	if !actor.IsDev && !auth.OrganizationScopeResultAllows(orgScope, target) {
		writeError(w, http.StatusNotFound, "Organisasi tidak ditemukan.")
		h.logOrgAccess(r, actor, "organization.get", "not_found", "organization out of scope", id)
		return
	}

	// Phase 3: bound the aggregates to the facilities where facility.read is
	// GRANTED (provenance-aware, live), never a blind org-wide join.
	readSet, err := h.listFacilityReadSet(r, actor, "facility.read")
	if err != nil || (!readSet.Unrestricted && len(readSet.IDs) == 0) {
		detail := "no facilities authorized for facility.read"
		status := "ok"
		if err != nil {
			detail = "scope resolution failed"
			status = "error"
		}
		writeError(w, http.StatusNotFound, "Organisasi tidak ditemukan.")
		h.logOrgAccess(r, actor, "organization.get", status, detail, id)
		return
	}

	var ov organizationOverview
	err = h.pool.QueryRow(ctx, `
		SELECT
			o.id, o.name, o.region, o.is_active,
			COALESCE(f.facility_count, 0),
			COALESCE(su.service_unit_count, 0),
			COALESCE(q.active_queue_count, 0),
			COALESCE(a.appointment_count, 0)
		FROM organizations o
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS facility_count
			FROM facilities f
			WHERE f.organization_id = o.id
			  AND ($2::bool OR f.id = ANY($3))
		) f ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS service_unit_count
			FROM service_units su
			JOIN facilities fc ON fc.id = su.facility_id
			WHERE fc.organization_id = o.id
			  AND ($2::bool OR fc.id = ANY($3))
		) su ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS active_queue_count
			FROM queue_tickets qt
			JOIN facilities fq ON fq.id = qt.facility_id
			WHERE fq.organization_id = o.id
			  AND ($2::bool OR fq.id = ANY($3))
			  AND qt.status IN ('waiting', 'called', 'in_service')
		) q ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS appointment_count
			FROM appointments ap
			JOIN facilities fa ON fa.id = ap.facility_id
			WHERE fa.organization_id = o.id
			  AND ($2::bool OR fa.id = ANY($3))
		) a ON true
		WHERE o.id = $1`,
		id, readSet.Unrestricted, readSet.IDs,
	).Scan(&ov.ID, &ov.Name, &ov.Region, &ov.IsActive,
		&ov.FacilityCount, &ov.ServiceUnitCount, &ov.ActiveQueueCount, &ov.AppointmentCount)

	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Organisasi tidak ditemukan.")
			h.logOrgAccess(r, actor, "organization.get", "not_found", id, id)
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil ringkasan organisasi.")
		h.logOrgAccess(r, actor, "organization.get", "error", err.Error(), id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": ov})
	h.logOrgAccess(r, actor, "organization.get", "ok", id, id)
}

// OrganizationsRouter dispatches organization admin requests by method and path
// suffix, mirroring FacilitiesRouter. It is runtime-complete and unit-testable
// without net/http route registration.
func (h *AdminHandler) OrganizationsRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/api/v1/admin/organizations" {
			h.ListOrganizations(w, r)
			return
		}
		h.GetOrganizationOverview(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Metode tidak diizinkan.")
	}
}

// itoa avoids importing strconv in this file for a single small integer.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}