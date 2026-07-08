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
			"id":         id,
			"status":     req.Status,
			"updated_at": time.Now().UTC(),
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

// --- Service unit response types ---

type serviceUnitResponse struct {
	ID          string     `json:"id"`
	FacilityID  string     `json:"facility_id"`
	Name        string     `json:"name"`
	Code        *string    `json:"code,omitempty"`
	Description *string    `json:"description,omitempty"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

// CreateServiceUnitRequest is the JSON body for service unit creation.
type CreateServiceUnitRequest struct {
	Name        string `json:"name"`
	FacilityID  string `json:"facility_id"`
	Code        string `json:"code,omitempty"`
	Description string `json:"description,omitempty"`
}

// UpdateServiceUnitRequest is the JSON body for service unit updates.
type UpdateServiceUnitRequest struct {
	Name        *string `json:"name,omitempty"`
	FacilityID  *string `json:"facility_id,omitempty"`
	Code        *string `json:"code,omitempty"`
	Description *string `json:"description,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

// --- Service unit handlers ---

// ListServiceUnits handles GET /api/v1/admin/service-units.
// Requires the schedule.read permission.
func (h *AdminHandler) ListServiceUnits(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logServiceUnitAccess(r, actor, "service_unit.list", "error", "nil pool")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := h.pool.Query(ctx,
		`SELECT id, facility_id, name, code, description, is_active
		 FROM service_units ORDER BY name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data unit layanan.")
		h.logServiceUnitAccess(r, actor, "service_unit.list", "error", err.Error())
		return
	}
	defer rows.Close()

	var results []serviceUnitResponse
	for rows.Next() {
		var u serviceUnitResponse
		if err := rows.Scan(&u.ID, &u.FacilityID, &u.Name, &u.Code, &u.Description, &u.IsActive); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data unit layanan.")
			h.logServiceUnitAccess(r, actor, "service_unit.list", "error", err.Error())
			return
		}
		results = append(results, u)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    results,
	})
	h.logServiceUnitAccess(r, actor, "service_unit.list", "ok", fmt.Sprintf("count=%d", len(results)))
}

// GetServiceUnit handles GET /api/v1/admin/service-units/{id}.
// Requires the schedule.read permission.
func (h *AdminHandler) GetServiceUnit(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logServiceUnitAccess(r, actor, "service_unit.get", "error", "nil pool")
		return
	}
	id := extractServiceUnitID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID unit layanan tidak valid.")
		h.logServiceUnitAccess(r, actor, "service_unit.get", "error", "missing id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var u serviceUnitResponse
	err := h.pool.QueryRow(ctx,
		`SELECT id, facility_id, name, code, description, is_active, created_at, updated_at
		 FROM service_units WHERE id = $1`, id,
	).Scan(&u.ID, &u.FacilityID, &u.Name, &u.Code, &u.Description, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Unit layanan tidak ditemukan.")
			h.logServiceUnitAccess(r, actor, "service_unit.get", "not_found", id)
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data unit layanan.")
		h.logServiceUnitAccess(r, actor, "service_unit.get", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    u,
	})
	h.logServiceUnitAccess(r, actor, "service_unit.get", "ok", id)
}

// CreateServiceUnit handles POST /api/v1/admin/service-units.
// Requires the schedule.manage permission.
func (h *AdminHandler) CreateServiceUnit(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logServiceUnitAccess(r, actor, "service_unit.created", "error", "nil pool")
		return
	}

	var req CreateServiceUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		h.logServiceUnitAccess(r, actor, "service_unit.created", "error", "invalid json")
		return
	}
	if err := validateCreateServiceUnit(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		h.logServiceUnitAccess(r, actor, "service_unit.created", "validation_error", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO service_units (facility_id, name, code, description)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id`,
		req.FacilityID, req.Name, req.Code, req.Description,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat unit layanan.")
		h.logServiceUnitAccess(r, actor, "service_unit.created", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"data":    map[string]string{"id": id},
	})
	h.logServiceUnitAccess(r, actor, "service_unit.created", "ok", id)
}

// UpdateServiceUnit handles PATCH /api/v1/admin/service-units/{id}.
// Requires the schedule.manage permission.
func (h *AdminHandler) UpdateServiceUnit(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logServiceUnitAccess(r, actor, "service_unit.updated", "error", "nil pool")
		return
	}
	id := extractServiceUnitID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID unit layanan tidak valid.")
		h.logServiceUnitAccess(r, actor, "service_unit.updated", "error", "missing id")
		return
	}

	var req UpdateServiceUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		h.logServiceUnitAccess(r, actor, "service_unit.updated", "error", "invalid json")
		return
	}
	if err := validateUpdateServiceUnit(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		h.logServiceUnitAccess(r, actor, "service_unit.updated", "validation_error", err.Error())
		return
	}

	var setClauses []string
	var args []any
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.FacilityID != nil {
		setClauses = append(setClauses, fmt.Sprintf("facility_id = $%d", argIdx))
		args = append(args, *req.FacilityID)
		argIdx++
	}
	if req.Code != nil {
		setClauses = append(setClauses, fmt.Sprintf("code = $%d", argIdx))
		args = append(args, *req.Code)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}

	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "Tidak ada field yang diupdate.")
		h.logServiceUnitAccess(r, actor, "service_unit.updated", "error", "no fields")
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf("UPDATE service_units SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	res, err := h.pool.Exec(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengupdate unit layanan.")
		h.logServiceUnitAccess(r, actor, "service_unit.updated", "error", err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Unit layanan tidak ditemukan.")
		h.logServiceUnitAccess(r, actor, "service_unit.updated", "not_found", id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    map[string]string{"id": id},
	})
	h.logServiceUnitAccess(r, actor, "service_unit.updated", "ok", id)
}

// ServiceUnitsRouter dispatches service unit admin requests by method and path suffix.
func (h *AdminHandler) ServiceUnitsRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/api/v1/admin/service-units" {
			h.ListServiceUnits(w, r)
			return
		}
		h.GetServiceUnit(w, r)
	case http.MethodPost:
		h.CreateServiceUnit(w, r)
	case http.MethodPatch:
		h.UpdateServiceUnit(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// extractServiceUnitID parses the service unit UUID from paths like:
//   /api/v1/admin/service-units/{id}
func extractServiceUnitID(path string) string {
	prefix := "/api/v1/admin/service-units/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	remainder = strings.TrimSpace(remainder)
	if _, err := uuid.Parse(remainder); err != nil {
		return ""
	}
	return remainder
}

// logServiceUnitAccess records an audit event for service unit admin endpoint access.
func (h *AdminHandler) logServiceUnitAccess(r *http.Request, actor identity.Actor, action, status, detail string) {
	if h.audit == nil {
		return
	}
	h.audit.LogEvent(r.Context(), audit.Event{
		Action:       action,
		ResourceType: "service_unit",
		ActorType:    string(actor.Type),
		ActorUserID:  actor.UserID,
		RequestID:    identity.RequestIDFromContext(r.Context()),
		Metadata: audit.SanitizeMetadata(map[string]any{
			"status": status,
			"detail": detail,
		}),
	})
}

func validateCreateServiceUnit(req CreateServiceUnitRequest) error {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) < 1 || len(name) > 100 {
		return fmt.Errorf("Nama unit layanan wajib diisi (1–100 karakter).")
	}
	if _, err := uuid.Parse(req.FacilityID); err != nil {
		return fmt.Errorf("Facility ID tidak valid.")
	}
	if req.Code != "" && len(req.Code) > 20 {
		return fmt.Errorf("Kode unit layanan maksimal 20 karakter.")
	}
	return nil
}

func validateUpdateServiceUnit(req UpdateServiceUnitRequest) error {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 100 {
			return fmt.Errorf("Nama unit layanan tidak boleh kosong dan maksimal 100 karakter.")
		}
	}
	if req.FacilityID != nil {
		if _, err := uuid.Parse(*req.FacilityID); err != nil {
			return fmt.Errorf("Facility ID tidak valid.")
		}
	}
	if req.Code != nil && *req.Code != "" && len(*req.Code) > 20 {
		return fmt.Errorf("Kode unit layanan maksimal 20 karakter.")
	}
	return nil
}

// --- Schedule response types ---

type scheduleResponse struct {
	ID              string     `json:"id"`
	FacilityID      string     `json:"facility_id"`
	PractitionerID  string     `json:"practitioner_id,omitempty"`
	ServiceUnitID   string     `json:"service_unit_id"`
	ScheduleDate    string     `json:"schedule_date"`
	StartTime       string     `json:"start_time"`
	EndTime         string     `json:"end_time"`
	SlotMinutes     int        `json:"slot_minutes"`
	CapacityPerSlot int        `json:"capacity_per_slot"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// CreateScheduleRequest is the JSON body for practitioner schedule creation.
type CreateScheduleRequest struct {
	FacilityID      string `json:"facility_id"`
	PractitionerID  string `json:"practitioner_id,omitempty"`
	ServiceUnitID   string `json:"service_unit_id"`
	ScheduleDate    string `json:"schedule_date"` // YYYY-MM-DD
	StartTime       string `json:"start_time"`    // HH:MM or HH:MM:SS
	EndTime         string `json:"end_time"`      // HH:MM or HH:MM:SS
	SlotMinutes     int    `json:"slot_minutes"`
	CapacityPerSlot int    `json:"capacity_per_slot"`
}

// UpdateScheduleRequest is the JSON body for practitioner schedule updates.
type UpdateScheduleRequest struct {
	FacilityID      *string `json:"facility_id,omitempty"`
	PractitionerID  *string `json:"practitioner_id,omitempty"`
	ServiceUnitID   *string `json:"service_unit_id,omitempty"`
	ScheduleDate    *string `json:"schedule_date,omitempty"`
	StartTime       *string `json:"start_time,omitempty"`
	EndTime         *string `json:"end_time,omitempty"`
	SlotMinutes     *int    `json:"slot_minutes,omitempty"`
	CapacityPerSlot *int    `json:"capacity_per_slot,omitempty"`
	IsActive        *bool   `json:"is_active,omitempty"`
}

// --- Schedule handlers ---

// ListSchedules handles GET /api/v1/admin/schedules.
// Requires the schedule.read permission.
func (h *AdminHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logScheduleAccess(r, actor, "schedule.list", "error", "nil pool")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := h.pool.Query(ctx,
		`SELECT id, facility_id, practitioner_id, service_unit_id,
		    schedule_date::text, start_time::text, end_time::text,
		    slot_minutes, capacity_per_slot, is_active,
		    created_at, updated_at
		 FROM practitioner_schedules
		 ORDER BY schedule_date DESC, start_time ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data jadwal.")
		h.logScheduleAccess(r, actor, "schedule.list", "error", err.Error())
		return
	}
	defer rows.Close()

	var results []scheduleResponse
	for rows.Next() {
		var s scheduleResponse
		if err := rows.Scan(&s.ID, &s.FacilityID, &s.PractitionerID, &s.ServiceUnitID,
			&s.ScheduleDate, &s.StartTime, &s.EndTime, &s.SlotMinutes,
			&s.CapacityPerSlot, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data jadwal.")
			h.logScheduleAccess(r, actor, "schedule.list", "error", err.Error())
			return
		}
		results = append(results, s)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    results,
	})
	h.logScheduleAccess(r, actor, "schedule.list", "ok", fmt.Sprintf("count=%d", len(results)))
}

// GetSchedule handles GET /api/v1/admin/schedules/{id}.
// Requires the schedule.read permission.
func (h *AdminHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logScheduleAccess(r, actor, "schedule.get", "error", "nil pool")
		return
	}
	id := extractScheduleID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID jadwal tidak valid.")
		h.logScheduleAccess(r, actor, "schedule.get", "error", "missing id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var s scheduleResponse
	err := h.pool.QueryRow(ctx,
		`SELECT id, facility_id, practitioner_id, service_unit_id,
		    schedule_date::text, start_time::text, end_time::text,
		    slot_minutes, capacity_per_slot, is_active,
		    created_at, updated_at
		 FROM practitioner_schedules WHERE id = $1`, id,
	).Scan(&s.ID, &s.FacilityID, &s.PractitionerID, &s.ServiceUnitID,
		&s.ScheduleDate, &s.StartTime, &s.EndTime, &s.SlotMinutes,
		&s.CapacityPerSlot, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Jadwal tidak ditemukan.")
			h.logScheduleAccess(r, actor, "schedule.get", "not_found", id)
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data jadwal.")
		h.logScheduleAccess(r, actor, "schedule.get", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    s,
	})
	h.logScheduleAccess(r, actor, "schedule.get", "ok", id)
}

// CreateSchedule handles POST /api/v1/admin/schedules.
// Requires the schedule.manage permission.
func (h *AdminHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logScheduleAccess(r, actor, "schedule.created", "error", "nil pool")
		return
	}

	var req CreateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		h.logScheduleAccess(r, actor, "schedule.created", "error", "invalid json")
		return
	}
	if err := validateCreateSchedule(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		h.logScheduleAccess(r, actor, "schedule.created", "validation_error", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO practitioner_schedules (facility_id, practitioner_id, service_unit_id,
		    schedule_date, start_time, end_time, slot_minutes, capacity_per_slot)
		 VALUES ($1, NULLIF($2, ''), $3, $4::date, $5::time, $6::time, $7, $8)
		 RETURNING id`,
		req.FacilityID, req.PractitionerID, req.ServiceUnitID,
		req.ScheduleDate, req.StartTime, req.EndTime, req.SlotMinutes, req.CapacityPerSlot,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat jadwal.")
		h.logScheduleAccess(r, actor, "schedule.created", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"data":    map[string]string{"id": id},
	})
	h.logScheduleAccess(r, actor, "schedule.created", "ok", id)
}

// UpdateSchedule handles PATCH /api/v1/admin/schedules/{id}.
// Requires the schedule.manage permission.
func (h *AdminHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logScheduleAccess(r, actor, "schedule.updated", "error", "nil pool")
		return
	}
	id := extractScheduleID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID jadwal tidak valid.")
		h.logScheduleAccess(r, actor, "schedule.updated", "error", "missing id")
		return
	}

	var req UpdateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		h.logScheduleAccess(r, actor, "schedule.updated", "error", "invalid json")
		return
	}
	if err := validateUpdateSchedule(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		h.logScheduleAccess(r, actor, "schedule.updated", "validation_error", err.Error())
		return
	}

	var setClauses []string
	var args []any
	argIdx := 1

	if req.FacilityID != nil {
		setClauses = append(setClauses, fmt.Sprintf("facility_id = $%d", argIdx))
		args = append(args, *req.FacilityID)
		argIdx++
	}
	if req.PractitionerID != nil {
		if *req.PractitionerID == "" {
			setClauses = append(setClauses, fmt.Sprintf("practitioner_id = NULL"))
		} else {
			setClauses = append(setClauses, fmt.Sprintf("practitioner_id = $%d", argIdx))
			args = append(args, *req.PractitionerID)
			argIdx++
		}
	}
	if req.ServiceUnitID != nil {
		setClauses = append(setClauses, fmt.Sprintf("service_unit_id = $%d", argIdx))
		args = append(args, *req.ServiceUnitID)
		argIdx++
	}
	if req.ScheduleDate != nil {
		setClauses = append(setClauses, fmt.Sprintf("schedule_date = $%d::date", argIdx))
		args = append(args, *req.ScheduleDate)
		argIdx++
	}
	if req.StartTime != nil {
		setClauses = append(setClauses, fmt.Sprintf("start_time = $%d::time", argIdx))
		args = append(args, *req.StartTime)
		argIdx++
	}
	if req.EndTime != nil {
		setClauses = append(setClauses, fmt.Sprintf("end_time = $%d::time", argIdx))
		args = append(args, *req.EndTime)
		argIdx++
	}
	if req.SlotMinutes != nil {
		setClauses = append(setClauses, fmt.Sprintf("slot_minutes = $%d", argIdx))
		args = append(args, *req.SlotMinutes)
		argIdx++
	}
	if req.CapacityPerSlot != nil {
		setClauses = append(setClauses, fmt.Sprintf("capacity_per_slot = $%d", argIdx))
		args = append(args, *req.CapacityPerSlot)
		argIdx++
	}
	if req.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}

	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "Tidak ada field yang diupdate.")
		h.logScheduleAccess(r, actor, "schedule.updated", "error", "no fields")
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf("UPDATE practitioner_schedules SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	res, err := h.pool.Exec(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengupdate jadwal.")
		h.logScheduleAccess(r, actor, "schedule.updated", "error", err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Jadwal tidak ditemukan.")
		h.logScheduleAccess(r, actor, "schedule.updated", "not_found", id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    map[string]string{"id": id},
	})
	h.logScheduleAccess(r, actor, "schedule.updated", "ok", id)
}

// SchedulesRouter dispatches practitioner schedule admin requests by method and path suffix.
func (h *AdminHandler) SchedulesRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/api/v1/admin/schedules" {
			h.ListSchedules(w, r)
			return
		}
		h.GetSchedule(w, r)
	case http.MethodPost:
		h.CreateSchedule(w, r)
	case http.MethodPatch:
		h.UpdateSchedule(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// extractScheduleID parses the schedule UUID from paths like:
//   /api/v1/admin/schedules/{id}
func extractScheduleID(path string) string {
	prefix := "/api/v1/admin/schedules/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	remainder = strings.TrimSpace(remainder)
	if _, err := uuid.Parse(remainder); err != nil {
		return ""
	}
	return remainder
}

// logScheduleAccess records an audit event for schedule admin endpoint access.
func (h *AdminHandler) logScheduleAccess(r *http.Request, actor identity.Actor, action, status, detail string) {
	if h.audit == nil {
		return
	}
	h.audit.LogEvent(r.Context(), audit.Event{
		Action:       action,
		ResourceType: "schedule",
		ActorType:    string(actor.Type),
		ActorUserID:  actor.UserID,
		RequestID:    identity.RequestIDFromContext(r.Context()),
		Metadata: audit.SanitizeMetadata(map[string]any{
			"status": status,
			"detail": detail,
		}),
	})
}

func validateCreateSchedule(req CreateScheduleRequest) error {
	if _, err := uuid.Parse(req.FacilityID); err != nil {
		return fmt.Errorf("Facility ID tidak valid.")
	}
	if req.PractitionerID != "" {
		if _, err := uuid.Parse(req.PractitionerID); err != nil {
			return fmt.Errorf("Practitioner ID tidak valid.")
		}
	}
	if _, err := uuid.Parse(req.ServiceUnitID); err != nil {
		return fmt.Errorf("Service Unit ID tidak valid.")
	}
	if req.ScheduleDate == "" {
		return fmt.Errorf("Tanggal jadwal wajib diisi (YYYY-MM-DD).")
	}
	if _, err := time.Parse("2006-01-02", req.ScheduleDate); err != nil {
		return fmt.Errorf("Format tanggal jadwal tidak valid (YYYY-MM-DD).")
	}
	if req.StartTime == "" || req.EndTime == "" {
		return fmt.Errorf("Jam mulai dan jam selesai wajib diisi (HH:MM).")
	}
	var startDuration, endDuration time.Duration
	var startErr, endErr error
	for _, layout := range []string{"15:04", "15:04:05"} {
		var st, et time.Time
		st, startErr = time.Parse(layout, req.StartTime)
		et, endErr = time.Parse(layout, req.EndTime)
		if startErr == nil && endErr == nil {
			// strict round-trip check: reject "8:00" when "08:00" expected
			if st.Format(layout) != req.StartTime || et.Format(layout) != req.EndTime {
				startErr = fmt.Errorf("invalid time format")
				endErr = fmt.Errorf("invalid time format")
				continue
			}
			startDuration = time.Duration(st.Hour())*time.Hour + time.Duration(st.Minute())*time.Minute
			endDuration = time.Duration(et.Hour())*time.Hour + time.Duration(et.Minute())*time.Minute
			break
		}
	}
	if startErr != nil || endErr != nil {
		return fmt.Errorf("Format jam tidak valid (pilih HH:MM atau HH:MM:SS).")
	}
	if endDuration <= startDuration {
		return fmt.Errorf("Jam selesai harus setelah jam mulai.")
	}
	if req.SlotMinutes <= 0 {
		return fmt.Errorf("Durasi slot harus lebih dari 0 menit.")
	}
	if req.SlotMinutes < 5 || req.SlotMinutes > 180 {
		return fmt.Errorf("Durasi slot harus antara 5–180 menit.")
	}
	totalMinutes := int(endDuration - startDuration) / int(time.Minute)
	if totalMinutes%req.SlotMinutes != 0 {
		return fmt.Errorf("Durasi slot harus membagi habis rentang waktu (%d menit).", totalMinutes)
	}
	if req.CapacityPerSlot <= 0 {
		return fmt.Errorf("Kapasitas per slot harus lebih dari 0.")
	}
	if req.CapacityPerSlot > 100 {
		return fmt.Errorf("Kapasitas per slot maksimal 100.")
	}
	return nil
}

func validateUpdateSchedule(req UpdateScheduleRequest) error {
	if req.FacilityID != nil {
		if _, err := uuid.Parse(*req.FacilityID); err != nil {
			return fmt.Errorf("Facility ID tidak valid.")
		}
	}
	if req.PractitionerID != nil {
		if *req.PractitionerID != "" {
			if _, err := uuid.Parse(*req.PractitionerID); err != nil {
				return fmt.Errorf("Practitioner ID tidak valid.")
			}
		}
	}
	if req.ServiceUnitID != nil {
		if _, err := uuid.Parse(*req.ServiceUnitID); err != nil {
			return fmt.Errorf("Service Unit ID tidak valid.")
		}
	}
	if req.ScheduleDate != nil {
		if *req.ScheduleDate == "" {
			return fmt.Errorf("Tanggal jadwal tidak boleh kosong.")
		}
		if _, err := time.Parse("2006-01-02", *req.ScheduleDate); err != nil {
			return fmt.Errorf("Format tanggal jadwal tidak valid (YYYY-MM-DD).")
		}
	}
	if req.StartTime != nil {
		if *req.StartTime == "" {
			return fmt.Errorf("Jam mulai tidak boleh kosong.")
		}
		var ok bool
		for _, layout := range []string{"15:04", "15:04:05"} {
			if t, err := time.Parse(layout, *req.StartTime); err == nil {
				if t.Format(layout) == *req.StartTime {
					ok = true
					break
				}
			}
		}
		if !ok {
			return fmt.Errorf("Format jam mulai tidak valid (pilih HH:MM atau HH:MM:SS).")
		}
	}
	if req.EndTime != nil {
		if *req.EndTime == "" {
			return fmt.Errorf("Jam selesai tidak boleh kosong.")
		}
		var ok bool
		for _, layout := range []string{"15:04", "15:04:05"} {
			if t, err := time.Parse(layout, *req.EndTime); err == nil {
				if t.Format(layout) == *req.EndTime {
					ok = true
					break
				}
			}
		}
		if !ok {
			return fmt.Errorf("Format jam selesai tidak valid (pilih HH:MM atau HH:MM:SS).")
		}
	}
	if req.StartTime != nil && req.EndTime != nil {
		var startDuration, endDuration time.Duration
		for _, layout := range []string{"15:04", "15:04:05"} {
			st, serr := time.Parse(layout, *req.StartTime)
			et, eerr := time.Parse(layout, *req.EndTime)
			if serr == nil && eerr == nil {
				startDuration = time.Duration(st.Hour())*time.Hour + time.Duration(st.Minute())*time.Minute
				endDuration = time.Duration(et.Hour())*time.Hour + time.Duration(et.Minute())*time.Minute
				break
			}
		}
		if endDuration <= startDuration {
			return fmt.Errorf("Jam selesai harus setelah jam mulai.")
		}
	}
	if req.SlotMinutes != nil {
		if *req.SlotMinutes <= 0 {
			return fmt.Errorf("Durasi slot harus lebih dari 0 menit.")
		}
		if *req.SlotMinutes < 5 || *req.SlotMinutes > 180 {
			return fmt.Errorf("Durasi slot harus antara 5–180 menit.")
		}
	}
	if req.CapacityPerSlot != nil {
		if *req.CapacityPerSlot <= 0 {
			return fmt.Errorf("Kapasitas per slot harus lebih dari 0.")
		}
		if *req.CapacityPerSlot > 100 {
			return fmt.Errorf("Kapasitas per slot maksimal 100.")
		}
	}
	return nil
}

// --- Appointment response types ---

type appointmentResponse struct {
	ID                     string     `json:"id"`
	FacilityID             string     `json:"facility_id"`
	ServiceUnitID          string     `json:"service_unit_id"`
	PractitionerID         *string    `json:"practitioner_id,omitempty"`
	PractitionerScheduleID *string    `json:"practitioner_schedule_id,omitempty"`
	AppointmentTime        *time.Time `json:"appointment_time,omitempty"`
	Status                 string     `json:"status"`
	PatientDisplayName     string     `json:"patient_display_name"`
	CheckinCode            string     `json:"checkin_code,omitempty"`
	QueueTicketID          *string    `json:"queue_ticket_id,omitempty"`
	Notes                  *string    `json:"notes,omitempty"`
	CreatedAt              *time.Time `json:"created_at,omitempty"`
	UpdatedAt              *time.Time `json:"updated_at,omitempty"`
}

// UpdateAppointmentStatusRequest is the JSON body for updating an appointment status.
type UpdateAppointmentStatusRequest struct {
	Status string `json:"status"`
}

// validAppointmentTransitions defines the allowed status changes.
var validAppointmentTransitions = map[string][]string{
	"scheduled":   {"checked_in", "cancelled", "no_show"},
	"checked_in":  {"queued", "cancelled", "no_show"},
	"queued":      {"completed", "cancelled", "no_show"},
	"completed":   {},
	"cancelled":   {},
	"no_show":     {},
}

func isValidAppointmentTransition(from, to string) bool {
	allowed, ok := validAppointmentTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// --- Appointment admin handlers ---

// ListAppointments handles GET /api/v1/admin/appointments.
// Requires the appointment.read permission.
func (h *AdminHandler) ListAppointments(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logAppointmentAccess(r, actor, "appointment.list", "error", "nil pool")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := h.pool.Query(ctx,
		`SELECT id, facility_id, service_unit_id, practitioner_id, practitioner_schedule_id,
		    appointment_time, status, patient_display_name, checkin_code, queue_ticket_id,
		    created_at, updated_at
		 FROM appointments
		 ORDER BY appointment_time DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data janji temu.")
		h.logAppointmentAccess(r, actor, "appointment.list", "error", err.Error())
		return
	}
	defer rows.Close()

	var results []appointmentResponse
	for rows.Next() {
		var a appointmentResponse
		if err := rows.Scan(&a.ID, &a.FacilityID, &a.ServiceUnitID, &a.PractitionerID,
			&a.PractitionerScheduleID, &a.AppointmentTime, &a.Status, &a.PatientDisplayName,
			&a.CheckinCode, &a.QueueTicketID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data janji temu.")
			h.logAppointmentAccess(r, actor, "appointment.list", "error", err.Error())
			return
		}
		results = append(results, a)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    results,
	})
	h.logAppointmentAccess(r, actor, "appointment.list", "ok", fmt.Sprintf("count=%d", len(results)))
}

// UpdateAppointmentStatus handles PATCH /api/v1/admin/appointments/{id}/status.
// Requires the appointment.manage permission.
func (h *AdminHandler) UpdateAppointmentStatus(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "error", "nil pool")
		return
	}
	id := extractAppointmentID(r.URL.Path)
	if id == "" {
		writeError(w, http.StatusBadRequest, "ID janji temu tidak valid.")
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "error", "missing id")
		return
	}

	var req UpdateAppointmentStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "error", "invalid json")
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		writeError(w, http.StatusBadRequest, "Status wajib diisi.")
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "error", "empty status")
		return
	}
	var validStatuses = map[string]bool{
		"scheduled": true, "checked_in": true, "queued": true,
		"completed": true, "cancelled": true, "no_show": true,
	}
	if !validStatuses[status] {
		writeError(w, http.StatusBadRequest, "Status janji temu tidak valid.")
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "error", "invalid status")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var current string
	if err := h.pool.QueryRow(ctx, `SELECT status FROM appointments WHERE id = $1`, id).Scan(&current); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Janji temu tidak ditemukan.")
			h.logAppointmentAccess(r, actor, "appointment.status_updated", "not_found", id)
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membaca status janji temu.")
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "error", err.Error())
		return
	}
	if !isValidAppointmentTransition(current, status) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Transisi status '%s' → '%s' tidak diizinkan.", current, status))
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "error", fmt.Sprintf("invalid transition %s->%s", current, status))
		return
	}

	setClause := "status = $1"
	if status == "checked_in" {
		setClause += ", checkin_at = NOW()"
	}
	if status == "completed" {
		setClause += ", completed_at = NOW()"
	}
	if status == "cancelled" {
		setClause += ", cancelled_at = NOW()"
	}

	res, err := h.pool.Exec(ctx,
		fmt.Sprintf("UPDATE appointments SET %s, updated_at = NOW() WHERE id = $2", setClause),
		status, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengupdate status janji temu.")
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "error", err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "Janji temu tidak ditemukan.")
		h.logAppointmentAccess(r, actor, "appointment.status_updated", "not_found", id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{
			"id":         id,
			"status":     status,
			"updated_at": time.Now().UTC(),
		},
	})
	h.logAppointmentAccess(r, actor, "appointment.status_updated", "ok", fmt.Sprintf("%s->%s", current, status))
}

// AppointmentsRouter dispatches appointment admin requests by method and path suffix.
func (h *AdminHandler) AppointmentsRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListAppointments(w, r)
	case http.MethodPatch:
		h.UpdateAppointmentStatus(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// extractAppointmentID parses the appointment UUID from paths like:
//   /api/v1/admin/appointments/{id}/status
func extractAppointmentID(path string) string {
	prefix := "/api/v1/admin/appointments/"
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

// logAppointmentAccess records an audit event for appointment admin endpoint access.
func (h *AdminHandler) logAppointmentAccess(r *http.Request, actor identity.Actor, action, status, detail string) {
	if h.audit == nil {
		return
	}
	h.audit.LogEvent(r.Context(), audit.Event{
		Action:       action,
		ResourceType: "appointment",
		ActorType:    string(actor.Type),
		ActorUserID:  actor.UserID,
		RequestID:    identity.RequestIDFromContext(r.Context()),
		Metadata: audit.SanitizeMetadata(map[string]any{
			"status": status,
			"detail": detail,
		}),
	})
}
