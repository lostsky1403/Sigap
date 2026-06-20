package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// BookingHandler handles public appointment booking and check-in.
type BookingHandler struct {
	pool     *pgxpool.Pool
	audit    *audit.Service
	limiter  *limiter.RateLimiter
	queueSvc service.QueueService
}

// NewBookingHandler creates a handler with db pool and optional rate limiter.
func NewBookingHandler(pool *pgxpool.Pool, rl *limiter.RateLimiter) *BookingHandler {
	return &BookingHandler{pool: pool, limiter: rl}
}

// WithAudit attaches an audit service.
func (h *BookingHandler) WithAudit(a *audit.Service) *BookingHandler {
	h.audit = a
	return h
}

// BookAppointmentRequest is the public booking payload.
type BookAppointmentRequest struct {
	FacilityID             string `json:"facility_id"`
	ServiceUnitID          string `json:"service_unit_id"`
	PractitionerScheduleID string `json:"practitioner_schedule_id,omitempty"`
	PatientDisplayName     string `json:"patient_display_name"`
	PatientPhone           string `json:"patient_phone"`
	AppointmentTime        string `json:"appointment_time"` // RFC3339
	Notes                  string `json:"notes,omitempty"`
}

// BookAppointmentResponse is the public booking success payload.
type BookAppointmentResponse struct {
	ID              string `json:"id"`
	CheckinCode     string `json:"checkin_code"`
	Status          string `json:"status"`
	AppointmentTime string `json:"appointment_time"`
}

// BookAppointment handles POST /api/v1/appointments (no auth required).
func (h *BookingHandler) BookAppointment(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		return
	}

	var req BookAppointmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		return
	}

	// Validation
	if err := validateBookAppointment(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Normalize phone
	phone := normalizePhone(req.PatientPhone)

	// Rate limit: max 3 bookings per phone per day
	today := time.Now().UTC().Format("2006-01-02")
	if h.limiter != nil && !h.limiter.Allow("phone:"+phone+":"+today) {
		writeError(w, http.StatusTooManyRequests, "Nomor HP ini sudah melebihi batas pemesanan per hari. Silakan coba lagi besok.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Verify facility and service unit exist
	var facilityExists, serviceUnitExists bool
	err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM facilities WHERE id = $1), EXISTS(SELECT 1 FROM service_units WHERE id = $2)`,
		req.FacilityID, req.ServiceUnitID).Scan(&facilityExists, &serviceUnitExists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memverifikasi fasilitas.")
		return
	}
	if !facilityExists {
		writeError(w, http.StatusBadRequest, "Fasilitas tidak ditemukan.")
		return
	}
	if !serviceUnitExists {
		writeError(w, http.StatusBadRequest, "Unit layanan tidak ditemukan.")
		return
	}

	// Parse appointment time
	aptTime, err := time.Parse(time.RFC3339, req.AppointmentTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Format waktu janji temu tidak valid (RFC3339).")
		return
	}
	if aptTime.Before(time.Now().UTC()) || aptTime.Equal(time.Now().UTC()) {
		writeError(w, http.StatusBadRequest, "Waktu janji temu harus di masa depan.")
		return
	}

	// Capacity check if a schedule is chosen
	if req.PractitionerScheduleID != "" {
		var capacityPerSlot int
		var slotMinutes int
		var startTime, endTime string
		err := h.pool.QueryRow(ctx,
			`SELECT capacity_per_slot, COALESCE(slot_minutes,0), start_time::text, end_time::text
			 FROM practitioner_schedules WHERE id = $1`,
			req.PractitionerScheduleID).Scan(&capacityPerSlot, &slotMinutes, &startTime, &endTime)
		if err != nil {
			if err == pgx.ErrNoRows {
				writeError(w, http.StatusBadRequest, "Jadwal praktisi tidak ditemukan.")
				return
			}
			writeError(w, http.StatusInternalServerError, "Gagal memverifikasi jadwal.")
			return
		}

		var slotFrom time.Time
		slotFrom, _ = time.Parse(time.RFC3339, req.AppointmentTime)
		slotTo := slotFrom.Add(time.Duration(slotMinutes) * time.Minute)

		var count int
		err = h.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM appointments
			 WHERE practitioner_schedule_id = $1
			   AND appointment_time >= $2
			   AND appointment_time < $3
			   AND status NOT IN ('cancelled', 'no_show')`,
			req.PractitionerScheduleID, slotFrom, slotTo).Scan(&count)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memeriksa ketersediaan slot.")
			return
		}
		if count >= capacityPerSlot {
			writeError(w, http.StatusConflict, "Slot jadwal sudah penuh. Silakan pilih jadwal lain.")
			return
		}
	}

	// Generate check-in code
	code, err := generateCheckinCode(6)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat kode check-in.")
		return
	}

	// Insert appointment
	var id string
	err = h.pool.QueryRow(ctx,
		`INSERT INTO appointments
		 (facility_id, service_unit_id, practitioner_schedule_id, patient_display_name,
		  patient_phone, appointment_time, status, checkin_code, notes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'scheduled', $7, $8, NOW(), NOW())
		 RETURNING id`,
		req.FacilityID, req.ServiceUnitID, strPtr(req.PractitionerScheduleID),
		req.PatientDisplayName, phone, aptTime, code, strPtr(req.Notes)).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan janji temu.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": BookAppointmentResponse{
			ID:              id,
			CheckinCode:     code,
			Status:          "scheduled",
			AppointmentTime: aptTime.Format(time.RFC3339),
		},
	})

	// Audit — sanitize: no raw phone
	h.logBookingEvent(r, "appointment.created", "ok",
		fmt.Sprintf("facility=%s service_unit=%s schedule=%s", req.FacilityID, req.ServiceUnitID, req.PractitionerScheduleID))
}

// validateBookAppointment performs structural validation.
func validateBookAppointment(req BookAppointmentRequest) error {
	if _, err := uuid.Parse(req.FacilityID); err != nil {
		return fmt.Errorf("ID fasilitas tidak valid.")
	}
	if _, err := uuid.Parse(req.ServiceUnitID); err != nil {
		return fmt.Errorf("ID unit layanan tidak valid.")
	}
	if req.PractitionerScheduleID != "" {
		if _, err := uuid.Parse(req.PractitionerScheduleID); err != nil {
			return fmt.Errorf("ID jadwal praktisi tidak valid.")
		}
	}
	if strings.TrimSpace(req.PatientDisplayName) == "" {
		return fmt.Errorf("Nama pasien wajib diisi.")
	}
	if len(req.PatientDisplayName) > 100 {
		return fmt.Errorf("Nama pasien maksimal 100 karakter.")
	}
	phone := normalizePhone(req.PatientPhone)
	if phone == "" {
		return fmt.Errorf("Nomor telepon wajib diisi (10-15 digit).")
	}
	if len(phone) < 10 || len(phone) > 15 {
		return fmt.Errorf("Nomor telepon harus 10-15 digit angka.")
	}
	if req.AppointmentTime == "" {
		return fmt.Errorf("Waktu janji temu wajib diisi (RFC3339).")
	}
	return nil
}

// normalizePhone strips non-digit characters.
func normalizePhone(s string) string {
	re := regexp.MustCompile(`\D`)
	return re.ReplaceAllString(s, "")
}

// generateCheckinCode creates a random alphanumeric code of given length.
func generateCheckinCode(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

// strPtr returns a pointer to a string, or nil if empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// WithQueueService attaches the queue engine service for check-in.
func (h *BookingHandler) WithQueueService(s service.QueueService) *BookingHandler {
	h.queueSvc = s
	return h
}

// CheckIn handles POST /api/v1/appointments/{id}/check-in (public).
// Validates check-in code, calls gRPC GenerateQueueNumber, updates appointment to queued.
func (h *BookingHandler) CheckIn(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		return
	}

	// Extract appointment ID from path
	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		writeError(w, http.StatusBadRequest, "URL tidak valid.")
		return
	}
	id := parts[2]
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, http.StatusBadRequest, "ID janji temu tidak valid.")
		return
	}

	var req struct {
		CheckinCode string `json:"checkin_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		return
	}
	if strings.TrimSpace(req.CheckinCode) == "" {
		writeError(w, http.StatusBadRequest, "Kode check-in wajib diisi.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Fetch appointment and validate code and status
	var facilityID, serviceUnitID, patientName, patientPhone string
	var practitionerScheduleID, queueTicketID *string
	var status string
	err := h.pool.QueryRow(ctx,
		`SELECT facility_id, service_unit_id, practitioner_schedule_id,
		        patient_display_name, patient_phone, status, queue_ticket_id
		 FROM appointments WHERE id = $1`, id,
	).Scan(&facilityID, &serviceUnitID, &practitionerScheduleID,
		&patientName, &patientPhone, &status, &queueTicketID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Janji temu tidak ditemukan.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membaca janji temu.")
		return
	}
	if status != "scheduled" {
		writeError(w, http.StatusConflict, "Janji temu tidak dapat check-in dengan status: "+status)
		return
	}

	// Verify check-in code
	var dbCode string
	err = h.pool.QueryRow(ctx,
		`SELECT checkin_code FROM appointments WHERE id = $1`, id).Scan(&dbCode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memverifikasi kode check-in.")
		return
	}
	if !strings.EqualFold(dbCode, req.CheckinCode) {
		writeError(w, http.StatusUnauthorized, "Kode check-in tidak cocok.")
		return
	}

	// Transition to checked_in
	_, err = h.pool.Exec(ctx,
		`UPDATE appointments SET status = 'checked_in', checkin_at = NOW(), updated_at = NOW() WHERE id = $1`,
		id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status check-in.")
		return
	}

	// Call queue engine to generate ticket
	if h.queueSvc == nil {
		writeError(w, http.StatusInternalServerError, "Layanan antrean tidak tersedia.")
		return
	}
	res, err := h.queueSvc.Generate(ctx, service.GenerateInput{
		FacilityID: facilityID,
		Patient: service.PatientInput{
			FullName: patientName,
			Phone:    patientPhone,
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil nomor antrean.")
		return
	}

	// Transition to queued with queue_ticket_id
	_, err = h.pool.Exec(ctx,
		`UPDATE appointments SET status = 'queued', queue_ticket_id = $1, updated_at = NOW() WHERE id = $2`,
		res.TicketID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyelesaikan check-in.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{
			"queue_ticket_id":      res.TicketID,
			"formatted_number":     res.FormattedNumber,
			"status":               "queued",
			"estimated_wait_minutes": res.EstimatedWaitMinutes,
			"processing_time":      res.ProcessingTime,
		},
	})

	h.logBookingEvent(r, "appointment.checked_in", "ok",
		fmt.Sprintf("facility=%s appointment=%s ticket=%s", facilityID, id, res.TicketID))
}

// PublicAppointmentsRouter dispatches public appointment endpoints.
// POST /api/v1/appointments               → BookAppointment
// POST /api/v1/appointments/{id}/check-in → CheckIn
func (h *BookingHandler) PublicAppointmentsRouter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		path := r.URL.Path
		if path == "/api/v1/appointments" {
			h.BookAppointment(w, r)
			return
		}
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "appointments" && parts[4] == "check-in" {
			h.CheckIn(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *BookingHandler) logBookingEvent(r *http.Request, action, status, detail string) {
	if h.audit == nil {
		return
	}
	h.audit.LogEvent(r.Context(), audit.Event{
		Action:       action,
		ResourceType: "appointment",
		ActorType:    "anonymous",
		ActorUserID:  "",
		RequestID:    identity.RequestIDFromContext(r.Context()),
		Metadata: audit.SanitizeMetadata(map[string]any{
			"status": status,
			"detail": detail,
		}),
	})
}
