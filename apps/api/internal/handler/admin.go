package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// --- Facility response types ---

type facilityResponse struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	Address        string     `json:"address"`
	Kecamatan      string     `json:"kecamatan"`
	KabupatenKota  string     `json:"kabupaten_kota"`
	Provinsi       string     `json:"provinsi"`
	Phone          string     `json:"phone"`
	TotalBeds      int        `json:"total_beds"`
	AvailableBeds  int        `json:"available_beds"`
	IsActive       bool       `json:"is_active"`
	ShortCode      string     `json:"short_code"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

// CreateFacilityRequest is the JSON body for facility creation.
type CreateFacilityRequest struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Address       string `json:"address"`
	Kecamatan     string `json:"kecamatan"`
	KabupatenKota string `json:"kabupaten_kota"`
	Provinsi      string `json:"provinsi"`
	Phone         string `json:"phone"`
	TotalBeds     int    `json:"total_beds"`
	AvailableBeds int    `json:"available_beds"`
	ShortCode     string `json:"short_code"`
}

// UpdateFacilityRequest is the JSON body for facility updates.
type UpdateFacilityRequest struct {
	Name          *string `json:"name,omitempty"`
	Type          *string `json:"type,omitempty"`
	Address       *string `json:"address,omitempty"`
	Kecamatan     *string `json:"kecamatan,omitempty"`
	KabupatenKota *string `json:"kabupaten_kota,omitempty"`
	Provinsi      *string `json:"provinsi,omitempty"`
	Phone         *string `json:"phone,omitempty"`
	TotalBeds     *int    `json:"total_beds,omitempty"`
	AvailableBeds *int    `json:"available_beds,omitempty"`
	ShortCode     *string `json:"short_code,omitempty"`
}

// --- Facility handlers ---

// ListFacilities handles GET /api/v1/admin/facilities.
// Requires the facility.read permission (enforced by RequirePermission middleware).
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

	var results []facilityResponse
	for rows.Next() {
		var f facilityResponse
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

// GetFacility handles GET /api/v1/admin/facilities/{id}.
// Requires the facility.read permission.
func (h *AdminHandler) GetFacility(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id := extractFacilityID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID fasilitas tidak valid.")
		h.logAccess(r, actor, "facility.get", "error", "missing id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var f facilityResponse
	err := h.pool.QueryRow(ctx,
		`SELECT id, name, type, address, kecamatan, kabupaten_kota, provinsi,
			phone, total_beds, available_beds, is_active, short_code,
			created_at, updated_at
		 FROM facilities WHERE id = $1`, id,
	).Scan(&f.ID, &f.Name, &f.Type, &f.Address, &f.Kecamatan,
		&f.KabupatenKota, &f.Provinsi, &f.Phone, &f.TotalBeds,
		&f.AvailableBeds, &f.IsActive, &f.ShortCode, &f.CreatedAt, &f.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Fasilitas tidak ditemukan.")
			h.logAccess(r, actor, "facility.get", "not_found", id)
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data fasilitas.")
		h.logAccess(r, actor, "facility.get", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    f,
	})
	h.logAccess(r, actor, "facility.get", "ok", id)
}

// CreateFacility handles POST /api/v1/admin/facilities.
// Requires facility.manage permission.
func (h *AdminHandler) CreateFacility(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())

	var req CreateFacilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		h.logAccess(r, actor, "facility.created", "error", "invalid json")
		return
	}
	if err := validateCreateFacility(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		h.logAccess(r, actor, "facility.created", "validation_error", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO facilities (name, type, address, kecamatan, kabupaten_kota, provinsi,
			phone, total_beds, available_beds, short_code)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id`,
		req.Name, req.Type, req.Address, req.Kecamatan, req.KabupatenKota, req.Provinsi,
		req.Phone, req.TotalBeds, req.AvailableBeds, req.ShortCode,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat fasilitas.")
		h.logAccess(r, actor, "facility.created", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"data":    map[string]string{"id": id},
	})
	h.logAccess(r, actor, "facility.created", "ok", id)
}

// UpdateFacility handles PATCH /api/v1/admin/facilities/{id}.
// Requires facility.manage permission.
func (h *AdminHandler) UpdateFacility(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id := extractFacilityID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID fasilitas tidak valid.")
		h.logAccess(r, actor, "facility.updated", "error", "missing id")
		return
	}

	var req UpdateFacilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		h.logAccess(r, actor, "facility.updated", "error", "invalid json")
		return
	}
	if err := validateUpdateFacility(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		h.logAccess(r, actor, "facility.updated", "validation_error", err.Error())
		return
	}

	// Build dynamic SET clauses so nil fields are untouched.
	var setClauses []string
	var args []any
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Type != nil {
		setClauses = append(setClauses, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, *req.Type)
		argIdx++
	}
	if req.Address != nil {
		setClauses = append(setClauses, fmt.Sprintf("address = $%d", argIdx))
		args = append(args, *req.Address)
		argIdx++
	}
	if req.Kecamatan != nil {
		setClauses = append(setClauses, fmt.Sprintf("kecamatan = $%d", argIdx))
		args = append(args, *req.Kecamatan)
		argIdx++
	}
	if req.KabupatenKota != nil {
		setClauses = append(setClauses, fmt.Sprintf("kabupaten_kota = $%d", argIdx))
		args = append(args, *req.KabupatenKota)
		argIdx++
	}
	if req.Provinsi != nil {
		setClauses = append(setClauses, fmt.Sprintf("provinsi = $%d", argIdx))
		args = append(args, *req.Provinsi)
		argIdx++
	}
	if req.Phone != nil {
		setClauses = append(setClauses, fmt.Sprintf("phone = $%d", argIdx))
		args = append(args, *req.Phone)
		argIdx++
	}
	if req.TotalBeds != nil {
		setClauses = append(setClauses, fmt.Sprintf("total_beds = $%d", argIdx))
		args = append(args, *req.TotalBeds)
		argIdx++
	}
	if req.AvailableBeds != nil {
		setClauses = append(setClauses, fmt.Sprintf("available_beds = $%d", argIdx))
		args = append(args, *req.AvailableBeds)
		argIdx++
	}
	if req.ShortCode != nil {
		setClauses = append(setClauses, fmt.Sprintf("short_code = $%d", argIdx))
		args = append(args, *req.ShortCode)
		argIdx++
	}

	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "Tidak ada field yang diupdate.")
		h.logAccess(r, actor, "facility.updated", "error", "no fields")
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf("UPDATE facilities SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	res, err := h.pool.Exec(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengupdate fasilitas.")
		h.logAccess(r, actor, "facility.updated", "error", err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Fasilitas tidak ditemukan.")
		h.logAccess(r, actor, "facility.updated", "not_found", id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    map[string]string{"id": id},
	})
	h.logAccess(r, actor, "facility.updated", "ok", id)
}

// DeactivateFacility handles PATCH /api/v1/admin/facilities/{id}/deactivate.
// Requires facility.manage permission. Soft-delete: sets is_active=false.
func (h *AdminHandler) DeactivateFacility(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id := extractFacilityID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID fasilitas tidak valid.")
		h.logAccess(r, actor, "facility.deactivated", "error", "missing id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	res, err := h.pool.Exec(ctx,
		`UPDATE facilities SET is_active = false, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menonaktifkan fasilitas.")
		h.logAccess(r, actor, "facility.deactivated", "error", err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Fasilitas tidak ditemukan.")
		h.logAccess(r, actor, "facility.deactivated", "not_found", id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    map[string]string{"id": id, "is_active": "false"},
	})
	h.logAccess(r, actor, "facility.deactivated", "ok", id)
}

// FacilitiesRouter dispatches facility admin requests by method and path suffix.
func (h *AdminHandler) FacilitiesRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/api/v1/admin/facilities" {
			h.ListFacilities(w, r)
			return
		}
		h.GetFacility(w, r)
	case http.MethodPost:
		h.CreateFacility(w, r)
	case http.MethodPatch:
		if strings.HasSuffix(r.URL.Path, "/deactivate") {
			h.DeactivateFacility(w, r)
			return
		}
		h.UpdateFacility(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- Validation helpers ---

var validFacilityTypes = map[string]bool{
	"rumah_sakit": true,
	"puskesmas":   true,
}

func validateCreateFacility(req CreateFacilityRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("Nama fasilitas wajib diisi.")
	}
	if !validFacilityTypes[req.Type] {
		return fmt.Errorf("Tipe fasilitas tidak valid (pilih: rumah_sakit, puskesmas).")
	}
	if req.Address == "" || req.Kecamatan == "" || req.KabupatenKota == "" || req.Provinsi == "" {
		return fmt.Errorf("Alamat lengkap (jalan, kecamatan, kabupaten/kota, provinsi) wajib diisi.")
	}
	if req.TotalBeds < 0 {
		return fmt.Errorf("Total tempat tidur tidak boleh negatif.")
	}
	if req.AvailableBeds < 0 {
		return fmt.Errorf("Tempat tidur tersedia tidak boleh negatif.")
	}
	if req.AvailableBeds > req.TotalBeds {
		return fmt.Errorf("Tempat tidur tersedia tidak boleh melebihi total.")
	}
	if strings.TrimSpace(req.Phone) == "" {
		return fmt.Errorf("Nomor telepon wajib diisi.")
	}
	if strings.ContainsAny(req.Phone, "<>\"'&") {
		return fmt.Errorf("Nomor telepon mengandung karakter tidak valid.")
	}
	if strings.TrimSpace(req.ShortCode) == "" {
		return fmt.Errorf("Kode singkat wajib diisi.")
	}
	return nil
}

func validateUpdateFacility(req UpdateFacilityRequest) error {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return fmt.Errorf("Nama fasilitas tidak boleh kosong.")
	}
	if req.Type != nil && !validFacilityTypes[*req.Type] {
		return fmt.Errorf("Tipe fasilitas tidak valid (pilih: rumah_sakit, puskesmas).")
	}
	if req.TotalBeds != nil && *req.TotalBeds < 0 {
		return fmt.Errorf("Total tempat tidur tidak boleh negatif.")
	}
	if req.AvailableBeds != nil && *req.AvailableBeds < 0 {
		return fmt.Errorf("Tempat tidur tersedia tidak boleh negatif.")
	}
	if req.TotalBeds != nil && req.AvailableBeds != nil && *req.AvailableBeds > *req.TotalBeds {
		return fmt.Errorf("Tempat tidur tersedia tidak boleh melebihi total.")
	}
	if req.Phone != nil && strings.ContainsAny(*req.Phone, "<>\"'&") {
		return fmt.Errorf("Nomor telepon mengandung karakter tidak valid.")
	}
	return nil
}

// extractFacilityID parses the facility UUID from paths like:
//   /api/v1/admin/facilities/{id}
//   /api/v1/admin/facilities/{id}/deactivate
func extractFacilityID(path string) string {
	prefix := "/api/v1/admin/facilities/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	remainder = strings.TrimSuffix(remainder, "/deactivate")
	remainder = strings.TrimSpace(remainder)
	if _, err := uuid.Parse(remainder); err != nil {
		return ""
	}
	return remainder
}

// --- Audit helpers ---

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

// --- Queue admin types ---

type queueTicketResponse struct {
	ID              string     `json:"id"`
	FacilityID      string     `json:"facility_id"`
	QueueNumber     int        `json:"queue_number"`
	FormattedNumber string     `json:"formatted_number"`
	Status          string     `json:"status"`
	RegisteredAt    time.Time  `json:"registered_at"`
	CalledAt        *time.Time `json:"called_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

// --- Queue admin handlers ---

// ListQueueTickets handles GET /api/v1/admin/queues?facility_id=...
// Requires queue.read permission.
func (h *AdminHandler) ListQueueTickets(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	facilityID := r.URL.Query().Get("facility_id")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var rows pgx.Rows
	var err error
	if facilityID != "" {
		if _, err := uuid.Parse(facilityID); err != nil {
			writeError(w, http.StatusBadRequest, "facility_id tidak valid.")
			return
		}
		rows, err = h.pool.Query(ctx,
			`SELECT id, facility_id, queue_number, formatted_number, status,
				registered_at, called_at, completed_at
			 FROM queue_tickets WHERE facility_id = $1 ORDER BY registered_at DESC`,
			facilityID)
	} else {
		rows, err = h.pool.Query(ctx,
			`SELECT id, facility_id, queue_number, formatted_number, status,
				registered_at, called_at, completed_at
			 FROM queue_tickets ORDER BY registered_at DESC LIMIT 200`)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data antrean.")
		h.logQueueAccess(r, actor, "queue.list", "error", err.Error())
		return
	}
	defer rows.Close()

	var results []queueTicketResponse
	for rows.Next() {
		var t queueTicketResponse
		if err := rows.Scan(&t.ID, &t.FacilityID, &t.QueueNumber, &t.FormattedNumber,
			&t.Status, &t.RegisteredAt, &t.CalledAt, &t.CompletedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data antrean.")
			h.logQueueAccess(r, actor, "queue.list", "error", err.Error())
			return
		}
		results = append(results, t)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    results,
	})
	h.logQueueAccess(r, actor, "queue.list", "ok", fmt.Sprintf("count=%d, facility=%s", len(results), facilityID))
}

// GetQueueTicket handles GET /api/v1/admin/queues/{id}.
// Requires queue.read permission.
func (h *AdminHandler) GetQueueTicket(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id := extractQueueTicketID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID antrean tidak valid.")
		h.logQueueAccess(r, actor, "queue.read", "error", "missing id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var t queueTicketResponse
	err := h.pool.QueryRow(ctx,
		`SELECT id, facility_id, queue_number, formatted_number, status,
			registered_at, called_at, completed_at
		 FROM queue_tickets WHERE id = $1`, id,
	).Scan(&t.ID, &t.FacilityID, &t.QueueNumber, &t.FormattedNumber,
		&t.Status, &t.RegisteredAt, &t.CalledAt, &t.CompletedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Antrean tidak ditemukan.")
			h.logQueueAccess(r, actor, "queue.read", "not_found", id)
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data antrean.")
		h.logQueueAccess(r, actor, "queue.read", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    t,
	})
	h.logQueueAccess(r, actor, "queue.read", "ok", id)
}

// UpdateQueueStatus handles PATCH /api/v1/admin/queues/{id}/status.
// Requires queue.manage permission.
func (h *AdminHandler) UpdateQueueStatus(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	id := extractQueueTicketID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID antrean tidak valid.")
		h.logQueueAccess(r, actor, "queue.status_updated", "error", "missing id")
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		h.logQueueAccess(r, actor, "queue.status_updated", "error", "invalid json")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var currentStatus string
	err := h.pool.QueryRow(ctx, `SELECT status FROM queue_tickets WHERE id = $1`, id).Scan(&currentStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Antrean tidak ditemukan.")
			h.logQueueAccess(r, actor, "queue.status_updated", "not_found", id)
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil status antrean.")
		h.logQueueAccess(r, actor, "queue.status_updated", "error", err.Error())
		return
	}

	if !isValidQueueTransition(currentStatus, req.Status) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Transisi status tidak valid: %s → %s.", currentStatus, req.Status))
		h.logQueueAccess(r, actor, "queue.status_updated", "validation_error",
			fmt.Sprintf("%s → %s", currentStatus, req.Status))
		return
	}

	var setClauses []string
	var args []any
	argIdx := 1

	setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
	args = append(args, req.Status)
	argIdx++

	if req.Status == "called" {
		setClauses = append(setClauses, fmt.Sprintf("called_at = $%d", argIdx))
		args = append(args, time.Now().UTC())
		argIdx++
	}
	if req.Status == "completed" || req.Status == "cancelled" {
		setClauses = append(setClauses, fmt.Sprintf("completed_at = $%d", argIdx))
		args = append(args, time.Now().UTC())
		argIdx++
	}

	query := fmt.Sprintf(`UPDATE queue_tickets SET %s WHERE id = $%d`, strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)

	_, err = h.pool.Exec(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status antrean.")
		h.logQueueAccess(r, actor, "queue.status_updated", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{
			"id":     id,
			"status": req.Status,
		},
	})
	h.logQueueAccess(r, actor, "queue.status_updated", "ok",
		fmt.Sprintf("%s → %s", currentStatus, req.Status))
}

// QueuesRouter dispatches queue admin requests.
func (h *AdminHandler) QueuesRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/api/v1/admin/queues" {
			h.ListQueueTickets(w, r)
			return
		}
		h.GetQueueTicket(w, r)
	case http.MethodPatch:
		h.UpdateQueueStatus(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// isValidQueueTransition validates state-machine transitions.
var validQueueTransitions = map[string][]string{
	"waiting":    {"called", "cancelled"},
	"called":     {"in_service", "cancelled", "skipped"},
	"in_service": {"completed"},
	"completed":  {},
	"cancelled":  {},
	"skipped":    {},
}

func isValidQueueTransition(from, to string) bool {
	allowed, ok := validQueueTransitions[from]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// extractQueueTicketID parses the queue ticket UUID from paths like:
//   /api/v1/admin/queues/{id}
//   /api/v1/admin/queues/{id}/status
func extractQueueTicketID(path string) string {
	prefix := "/api/v1/admin/queues/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	remainder = strings.TrimSuffix(remainder, "/status")
	remainder = strings.TrimSpace(remainder)
	if _, err := uuid.Parse(remainder); err != nil {
		return ""
	}
	return remainder
}

// logQueueAccess records an audit event for queue admin endpoint access.
func (h *AdminHandler) logQueueAccess(r *http.Request, actor identity.Actor, action, status, detail string) {
	if h.audit == nil {
		return
	}
	h.audit.LogEvent(r.Context(), audit.Event{
		Action:       action,
		ResourceType: "queue",
		ActorType:    string(actor.Type),
		ActorUserID:  actor.UserID,
		RequestID:    identity.RequestIDFromContext(r.Context()),
		Metadata: audit.SanitizeMetadata(map[string]any{
			"status": status,
			"detail": detail,
		}),
	})
}
