package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// AdminHandler handles administration endpoints gated by the RBAC permission
// system. All operations are logged via the audit service with privacy-safe
// metadata (no patient data, no PII).
type AdminHandler struct {
	pool  *pgxpool.Pool
	audit *audit.Service
}

// NewAdminHandler creates an admin handler backed by a pgx pool.
func NewAdminHandler(pool *pgxpool.Pool) *AdminHandler {
	return &AdminHandler{pool: pool}
}

// WithAudit attaches an optional audit service for access logging.
func (h *AdminHandler) WithAudit(a *audit.Service) *AdminHandler {
	h.audit = a
	return h
}

// ListFacilities handles GET /api/v1/admin/facilities.
// Requires the facility.manage permission (enforced by RequirePermission middleware).
func (h *AdminHandler) ListFacilities(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := h.pool.Query(ctx,
		`SELECT id, name, type, kecamatan, kabupaten_kota, provinsi,
			phone, total_beds, available_beds, is_active, short_code
		 FROM facilities ORDER BY name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data fasilitas.")
		h.logAccess(r, actor, "facility.list", "error", err.Error())
		return
	}
	defer rows.Close()

	type facility struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Type           string `json:"type"`
		Kecamatan      string `json:"kecamatan"`
		KabupatenKota  string `json:"kabupaten_kota"`
		Provinsi       string `json:"provinsi"`
		Phone          string `json:"phone"`
		TotalBeds      int    `json:"total_beds"`
		AvailableBeds  int    `json:"available_beds"`
		IsActive       bool   `json:"is_active"`
		ShortCode      string `json:"short_code"`
	}

	var results []facility
	for rows.Next() {
		var f facility
		if err := rows.Scan(&f.ID, &f.Name, &f.Type, &f.Kecamatan, &f.KabupatenKota,
			&f.Provinsi, &f.Phone, &f.TotalBeds, &f.AvailableBeds, &f.IsActive, &f.ShortCode); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data fasilitas.")
			h.logAccess(r, actor, "facility.list", "error", err.Error())
			return
		}
		results = append(results, f)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    results,
	})

	h.logAccess(r, actor, "facility.list", "ok", fmt.Sprintf("count=%d", len(results)))
}

// logAccess records an audit event for admin endpoint access with privacy-safe metadata.
func (h *AdminHandler) logAccess(r *http.Request, actor identity.Actor, action, status, detail string) {
	if h.audit == nil {
		return
	}
	h.audit.LogEvent(r.Context(), audit.Event{
		Action:       action,
		ResourceType: "facility",
		ActorType:    string(actor.Type),
		ActorUserID:  actor.UserID,
		RequestID:    identity.RequestIDFromContext(r.Context()),
		Metadata: audit.SanitizeMetadata(map[string]any{
			"status": status,
			"detail": detail,
		}),
	})
}
