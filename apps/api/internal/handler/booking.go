package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/audit"
	"github.com/sigap/sigap/apps/api/internal/identity"
	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/notification"
	"github.com/sigap/sigap/apps/api/internal/router"
	"github.com/sigap/sigap/apps/api/internal/service"
)

// BookingHandler handles public appointment booking and check-in.
type BookingHandler struct {
	pool           *pgxpool.Pool
	audit          *audit.Service
	limiter        *limiter.RateLimiter
	checkinLimiter *limiter.RateLimiter // AUDIT-302: per-IP+appointment brute-force protection
	cancelLimiter  *limiter.RateLimiter // shared with patient status lookup: 30/min/IP
	queueSvc       service.QueueService
	notify         *notification.Service
}

// NewBookingHandler creates a handler with db pool and optional rate limiter.
func NewBookingHandler(pool *pgxpool.Pool, rl *limiter.RateLimiter) *BookingHandler {
	return &BookingHandler{pool: pool, limiter: rl}
}

// WithCheckinLimiter attaches a rate limiter for check-in brute-force protection.
// AUDIT-302: 5 attempts per5 minutes per (IP, appointment) pair.
func (h *BookingHandler) WithCheckinLimiter(rl *limiter.RateLimiter) *BookingHandler {
	h.checkinLimiter = rl
	return h
}

// WithCancelLimiter attaches the SAME per-IP limiter the public status lookup
// uses (30 req/min). Sharing the instance, rather than creating a second
// limiter, means a code cannot be probed at 60 combined req/min by spreading
// attempts across the status and cancel surfaces — the two endpoints burn the
// same budget.
func (h *BookingHandler) WithCancelLimiter(rl *limiter.RateLimiter) *BookingHandler {
	h.cancelLimiter = rl
	return h
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

	// Claim the appointment for the authenticated citizen account when one is
	// present; anonymous (unauthenticated) booking leaves patient_account_id NULL
	// and stays on the code-only model — no booking is forced into auth.
	actorAccountID := identity.ActorFromContext(ctx).PatientAccountID()
	var patientAccountID *string
	if actorAccountID != "" {
		patientAccountID = &actorAccountID
	}

	// Insert appointment
	var id string
	err = h.pool.QueryRow(ctx,
		`INSERT INTO appointments
		 (facility_id, service_unit_id, practitioner_schedule_id, patient_display_name,
		  patient_phone, appointment_time, status, checkin_code, notes, patient_account_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'scheduled', $7, $8, $9, NOW(), NOW())
		 RETURNING id`,
		req.FacilityID, req.ServiceUnitID, strPtr(req.PractitionerScheduleID),
		req.PatientDisplayName, phone, aptTime, code, strPtr(req.Notes), patientAccountID).Scan(&id)
	if err != nil {
		slog.Error("booking insert failed",
			"facility_id", req.FacilityID,
			"service_unit_id", req.ServiceUnitID,
			"practitioner_schedule_id", req.PractitionerScheduleID,
			"appointment_time", req.AppointmentTime,
			"err", err.Error())
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

	// Notification outbox trigger (fire-and-forget). We pass the
	// booking-id and the patient contact to the enqueue helper; the
	// helper consumes the contact transiently, computes mask + hash,
	// and discards the raw value before any persistence or return.
	h.fireBookingConfirmation(id, code, req.PatientDisplayName, phone, req.FacilityID)
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

// checkinCodeAlphabet is the symbol set a check-in code is drawn from.
//
// It deliberately omits I, O, 0 and 1. A check-in code is read off a
// confirmation screen or an SMS and typed into a phone, often at a facility
// entrance, and "O" versus "0" and "I" versus "1" are indistinguishable in
// most sans faces. A citizen who transcribes the letter faithfully but the
// screen renders a digit (or vice versa) gets "Kode tidak ditemukan" — the
// code they hold is correct, they copied what they saw, and the system tells
// them it does not exist. That is unfixable by the user and is exactly the
// failure this alphabet prevents.
//
// 32 symbols over 6 positions is 32^6 = 1.07e9 combinations, which remains far
// outside anything the endpoint's per-IP limit (30 requests/minute) can walk.
const checkinCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// generateCheckinCode creates a random code of the given length.
func generateCheckinCode(length int) (string, error) {
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(checkinCodeAlphabet))))
		if err != nil {
			return "", err
		}
		result[i] = checkinCodeAlphabet[n.Int64()]
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

// WithNotificationService attaches the notification outbox service.
// After booking and check-in, the handler fires a fire-and-forget
// goroutine that enqueues the corresponding confirmation; the service
// is allowed to be nil (in which case the goroutine is skipped).
func (h *BookingHandler) WithNotificationService(s *notification.Service) *BookingHandler {
	h.notify = s
	return h
}

// fireBookingConfirmation enqueues a booking-confirmation notification
// on a fire-and-forget goroutine. The HTTP response is never blocked
// by enqueue failures; any panic in the goroutine is recovered and
// logged without PII.
//
// patientName and patientPhone are transient: the notification
// service consumes the contact, computes mask + hash, and discards the
// raw value. patientName is NOT passed to the notification service
// (templates use {appointment_id} / {facility_name} placeholders
// only — never raw PII placeholders).
func (h *BookingHandler) fireBookingConfirmation(appointmentID, checkinCode, patientName, patientPhone, facilityID string) {
	if h.notify == nil {
		return
	}
	apptUUID, err := uuid.Parse(appointmentID)
	if err != nil {
		return // ignore — appointmentID must always be a UUID, this is just defensive
	}
	var facUUIDPtr *uuid.UUID
	if facilityID != "" {
		if f, err := uuid.Parse(facilityID); err == nil {
			facUUIDPtr = &f
		}
	}
	_ = patientName // currently unused; reserved for future templates that need display-name placeholders

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("notification: panic in booking-enqueue goroutine",
					"appointment_id", apptUUID.String(), "err", fmt.Sprintf("%v", r))
			}
		}()
		// Use a background context because r.Context() is cancelled
		// when the HTTP response completes. The DB pool enforces its
		// own timeouts.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := h.notify.Enqueue(ctx, notification.EnqueueInput{
			FacilityID:          facUUIDPtr,
			Channel:             notification.ChannelDev,
			TemplateKey:         "appointment.booked.confirmation",
			Subject:             "Konfirmasi Janji Temu Sigap",
			BodyTemplate:        "Janji temu Anda berhasil dicatat. Kode check-in: {checkin_code}.",
			TemplateVars:        map[string]string{"checkin_code": checkinCode},
			RecipientType:       notification.RecipientPatient,
			RecipientContact:    patientPhone,
			RelatedResourceType: "appointment",
			RelatedResourceID:   apptUUID.String(),
		})
		if err != nil {
			slog.Warn("notification: booking enqueue failed",
				"appointment_id", apptUUID.String(), "err", err.Error())
		}
	}()
}

// fireCheckInConfirmation enqueues a check-in-confirmation notification
// on a fire-and-forget goroutine. Same fire-and-forget contract as
// fireBookingConfirmation.
func (h *BookingHandler) fireCheckInConfirmation(appointmentID, patientPhone, facilityID, formattedQueueNumber string) {
	if h.notify == nil {
		return
	}
	apptUUID, err := uuid.Parse(appointmentID)
	if err != nil {
		return
	}
	var facUUIDPtr *uuid.UUID
	if facilityID != "" {
		if f, err := uuid.Parse(facilityID); err == nil {
			facUUIDPtr = &f
		}
	}
	queueNumber := formattedQueueNumber
	if queueNumber == "" {
		queueNumber = "(sedang diproses)"
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("notification: panic in check-in-enqueue goroutine",
					"appointment_id", apptUUID.String(), "err", fmt.Sprintf("%v", r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := h.notify.Enqueue(ctx, notification.EnqueueInput{
			FacilityID:          facUUIDPtr,
			Channel:             notification.ChannelDev,
			TemplateKey:         "appointment.checked_in.confirmation",
			Subject:             "Status Check-in Sigap",
			BodyTemplate:        "Check-in Anda berhasil. Nomor antrean: {queue_number}.",
			TemplateVars:        map[string]string{"queue_number": queueNumber},
			RecipientType:       notification.RecipientPatient,
			RecipientContact:    patientPhone,
			RelatedResourceType: "appointment",
			RelatedResourceID:   apptUUID.String(),
		})
		if err != nil {
			slog.Warn("notification: check-in enqueue failed",
				"appointment_id", apptUUID.String(), "err", err.Error())
		}
	}()
}

// CheckIn handles POST /api/v1/appointments/{id}/check-in (public).
//
// AUDIT-1004 fix: the appointment status transition is now atomic — a single
// UPDATE with WHERE id=$1 AND checkin_code=$2 AND status='scheduled' ensures
// that concurrent attempts cannot both succeed.  The separate SELECT+UPDATE
// TOCTOU window is eliminated.
//
// AUDIT-302 fix: per-IP + per-appointment rate limiting is applied before
// any DB work.  The composite key prevents brute-force code guessing while
// avoiding cross-user lockout behind shared NAT.
//
// Error classification (information-oracle resistant):
//   - 404: appointment does not exist
//   - 409: appointment exists but is not in 'scheduled' state
//   - 401: code does not match (existing scheduled appointment)
//   - 409: atomic transition failed (lost race to concurrent request)
func (h *BookingHandler) CheckIn(w http.ResponseWriter, r *http.Request) {
	// Extract appointment ID from path: POST /api/v1/appointments/{id}/check-in
	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		writeError(w, http.StatusBadRequest, "URL tidak valid.")
		return
	}
	id := parts[3]
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

	// AUDIT-302: rate-limit check-in attempts per IP + per appointment.
	// Checked early (before DB) to reject brute-force cheaply.
	// Key: "checkin:<client_ip>:<appointment_id>" — limits brute-force
	// code guessing without locking out other users or appointments.
	if h.checkinLimiter != nil {
		clientIP := router.ClientIPFromContext(r)
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		key := "checkin:" + clientIP + ":" + id
		if !h.checkinLimiter.Allow(key) {
			writeError(w, http.StatusTooManyRequests, "Terlalu banyak percobaan. Coba lagi nanti.")
			return
		}
	}

	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// AUDIT-1004: single atomic UPDATE that validates code + status in one
	// database round-trip.  If the row does not match all three conditions
	// (id, checkin_code, status='scheduled') the UPDATE touches zero rows
	// and we classify the failure.
	var facilityID, serviceUnitID, patientName, patientPhone string
	var practitionerScheduleID *string
	var codeVerified bool

	err := h.pool.QueryRow(ctx,
		`UPDATE appointments
		 SET status = 'checked_in', checkin_at = NOW(), updated_at = NOW()
		 WHERE id = $1
		   AND LOWER(checkin_code) = LOWER($2)
		   AND status = 'scheduled'
		 RETURNING facility_id, service_unit_id, practitioner_schedule_id,
		           patient_display_name, patient_phone`,
		id, req.CheckinCode,
	).Scan(&facilityID, &serviceUnitID, &practitionerScheduleID,
		&patientName, &patientPhone)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Atomic update returned zero rows.  Classify: wrong code or
			// already-transitioned — but do NOT reveal which.
			var exists bool
			err2 := h.pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM appointments WHERE id = $1)`, id,
			).Scan(&exists)
			if err2 != nil {
				// A database failure is not evidence the appointment is absent;
				// answering 404 here would hide an outage behind a user error.
				slog.Error("check-in existence probe failed",
					"appointment_id", id, "err", err2.Error())
				writeError(w, http.StatusInternalServerError, "Gagal memverifikasi check-in.")
				return
			}
			if !exists {
				writeError(w, http.StatusNotFound, "Janji temu tidak ditemukan.")
				return
			}
			// Appointment exists — wrong code or already checked in.
			// Check code match to determine response (non-oracle: same
			// HTTP status, different message for UX clarity).
			var dbCode string
			var dbStatus string
			err3 := h.pool.QueryRow(ctx,
				`SELECT checkin_code, status FROM appointments WHERE id = $1`, id,
			).Scan(&dbCode, &dbStatus)
			if err3 != nil {
				slog.Error("check-in code verification failed",
					"appointment_id", id, "err", err3.Error())
				writeError(w, http.StatusInternalServerError, "Gagal memverifikasi check-in.")
				return
			}
			if !strings.EqualFold(dbCode, req.CheckinCode) {
				writeError(w, http.StatusUnauthorized, "Kode check-in tidak cocok.")
				return
			}
			// Code matched but status was wrong — already checked in.
			writeError(w, http.StatusConflict, "Janji temu tidak dapat check-in dengan status: "+dbStatus)
			return
		}
		slog.Error("atomic check-in transition failed",
			"appointment_id", id, "err", err.Error())
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui status check-in.")
		return
	}
	codeVerified = true
	_ = codeVerified // defensive; always true after successful RETURNING scan

	// --- Queue ticket generation ---
	// The Rust engine owns queue numbering authority.  We call it via gRPC
	// after the atomic transition.  If it fails, we roll back to 'scheduled'
	// so the appointment can be retried.  This means a brief window exists
	// where the appointment is 'checked_in' with no queue_ticket_id — the
	// readyz probe reflects DB health, and the 500 response signals the
	// caller to retry.  True distributed atomicity is not achievable here
	// without two-phase commit; the compensating rollback is the strongest
	// safe behavior.
	if h.queueSvc == nil {
		// Roll back to scheduled so the appointment is not stranded.
		slog.Error("queue service not configured", "appointment_id", id)
		if _, rbErr := h.rollbackCheckIn(ctx, id); rbErr != nil {
			slog.Error("rollback to scheduled failed",
				"appointment_id", id, "err", rbErr.Error())
		}
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
		slog.Error("queue generation failed",
			"appointment_id", id,
			"facility_id", facilityID,
			"err", err.Error())
		// Roll back to scheduled so the appointment can be retried.
		rows, rbErr := h.rollbackCheckIn(ctx, id)
		if rbErr != nil {
			slog.Error("rollback to scheduled failed",
				"appointment_id", id, "err", rbErr.Error())
		} else if rows == 0 {
			slog.Warn("rollback guard: appointment no longer in checked_in state",
				"appointment_id", id)
			writeError(w, http.StatusConflict, "Janji temu sudah diproses oleh permintaan lain.")
			return
		}
		// Dev fallback: retry with FakeQueueService when SIGAP_ENGINE_FALLBACK=dev.
		if os.Getenv("SIGAP_ENGINE_FALLBACK") == "dev" {
			slog.Warn("SIGAP_ENGINE_FALLBACK=dev: retrying queue generation with FakeQueueService",
				"appointment_id", id)

			// AUDIT-1004: re-acquire ownership BEFORE creating a ticket.  The
			// rollback above released the appointment back to 'scheduled', so a
			// concurrent request may have claimed it in the meantime.  Only the
			// request that successfully flips scheduled→checked_in may insert a
			// queue ticket; otherwise two requests that both entered this fallback
			// path would each insert a ticket (duplicate tickets) even though only
			// one returns 200.
			ct, err := h.pool.Exec(ctx,
				`UPDATE appointments SET status = 'checked_in', checkin_at = NOW(), updated_at = NOW() WHERE id = $1 AND status = 'scheduled' AND queue_ticket_id IS NULL`,
				id)
			if err != nil {
				slog.Error("fallback re-acquire failed",
					"appointment_id", id, "err", err.Error())
				writeError(w, http.StatusInternalServerError, "Gagal memperbarui status check-in.")
				return
			}
			if ct.RowsAffected() == 0 {
				slog.Warn("fallback guard: appointment no longer in scheduled state",
					"appointment_id", id)
				writeError(w, http.StatusConflict, "Janji temu sudah diproses oleh permintaan lain.")
				return
			}

			fake := service.NewFakeQueueService()
			res, err = fake.Generate(ctx, service.GenerateInput{
				FacilityID: facilityID,
				Patient: service.PatientInput{
					FullName: patientName,
					Phone:    patientPhone,
				},
			})
			if err != nil {
				slog.Error("fallback queue generation failed",
					"appointment_id", id, "err", err.Error())
				// We hold checked_in but produced no ticket — release so a retry works.
				if _, rbErr := h.rollbackCheckIn(ctx, id); rbErr != nil {
					slog.Error("rollback to scheduled failed",
						"appointment_id", id, "err", rbErr.Error())
				}
				writeError(w, http.StatusInternalServerError, "Gagal mengambil nomor antrean.")
				return
			}

			// No engine issued this ticket, so this process mints the id and
			// stores the row itself.
			res.TicketID = uuid.New().String()
			if err := h.recordQueueTicket(ctx, time.Now(), res.TicketID, facilityID, patientName, patientPhone, res.FormattedNumber); err != nil {
				slog.Error("fallback ticket insert failed",
					"appointment_id", id, "queue_ticket_id", res.TicketID, "err", err.Error())
				if _, rbErr := h.rollbackCheckIn(ctx, id); rbErr != nil {
					slog.Error("rollback to scheduled failed",
						"appointment_id", id, "err", rbErr.Error())
				}
				writeError(w, http.StatusInternalServerError, "Gagal membuat tiket antrean.")
				return
			}
		} else {
			writeError(w, http.StatusInternalServerError, "Gagal mengambil nomor antrean.")
			return
		}
	}

	// The engine commits queue_tickets in its own transaction, against whatever
	// DATABASE_URL it was started with. When that is not this database the row
	// is invisible here and the foreign key on appointments.queue_ticket_id
	// rejects the finalize below, leaving the citizen checked_in with no ticket.
	//
	// This is a configuration fault, and it is reported as one. An earlier
	// attempt inserted a local copy of the engine-issued ticket so the finalize
	// would succeed — that was worse than the bug: the waiting-room display
	// reads the engine's database, so the number the citizen received would
	// never be called, and the local insert would silently fork the numbering
	// sequence into duplicate numbers on the board. A failed check-in the
	// citizen can retry is honest; a ticket that looks real and is not is not.
	if err := h.verifyQueueTicketVisible(ctx, res.TicketID); err != nil {
		slog.Error("queue ticket is not visible in this database; check that the queue engine and the API share one DATABASE_URL",
			"appointment_id", id, "queue_ticket_id", res.TicketID, "err", err.Error())
		if _, rbErr := h.rollbackCheckIn(ctx, id); rbErr != nil {
			slog.Error("rollback to scheduled failed",
				"appointment_id", id, "err", rbErr.Error())
		}
		writeError(w, http.StatusServiceUnavailable, "Layanan antrean tidak sinkron. Coba lagi sebentar lagi.")
		return
	}

	// Transition to queued with queue_ticket_id
	// Guarded: only the request that claimed this appointment can finalize it
	ct, err := h.pool.Exec(ctx,
		`UPDATE appointments SET status = 'queued', queue_ticket_id = $1, updated_at = NOW() WHERE id = $2 AND status = 'checked_in' AND queue_ticket_id IS NULL`,
		res.TicketID, id)
	if err != nil {
		slog.Error("check-in finalization failed",
			"appointment_id", id, "queue_ticket_id", res.TicketID, "err", err.Error())
		// A failed finalize must not leave the appointment checked_in without a
		// ticket: that state has no exit, because a retry no longer matches the
		// status='scheduled' guard and is answered 409 forever. Releasing it back
		// to scheduled lets the citizen retry with the same code; the ticket
		// row already recorded stays behind as an unused number, which is
		// cheaper than an appointment nobody can check in.
		if _, rbErr := h.rollbackCheckIn(ctx, id); rbErr != nil {
			slog.Error("rollback to scheduled failed",
				"appointment_id", id, "err", rbErr.Error())
		}
		writeError(w, http.StatusInternalServerError, "Gagal menyelesaikan check-in.")
		return
	}
	if ct.RowsAffected() == 0 {
		slog.Warn("finalization guard: appointment no longer in checked_in state",
			"appointment_id", id)
		writeError(w, http.StatusConflict, "Janji temu sudah diproses oleh permintaan lain.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{
			"appointment_id":         id,
			"queue_ticket_id":        res.TicketID,
			"formatted_number":       res.FormattedNumber,
			"status":                 "queued",
			"estimated_wait_minutes": res.EstimatedWaitMinutes,
			"processing_time":        res.ProcessingTime,
		},
	})

	h.logBookingEvent(r, "appointment.checked_in", "ok",
		fmt.Sprintf("facility=%s appointment=%s ticket=%s", facilityID, id, res.TicketID))

	// Notification outbox trigger (fire-and-forget).
	h.fireCheckInConfirmation(id, patientPhone, facilityID, res.FormattedNumber)
}

// CancelAppointment handles POST /api/v1/appointments/cancel (public).
//
// A citizen cancels their own appointment by check-in code, mirroring the
// status lookup: no auth, code-only access, per-IP rate limiting, and the same
// confusable-fold fallback so a mistyped "O"/"0" or "I"/"1" still resolves.
//
// Information-oracle resistance: an ambiguous fold fails closed as "not found"
// (indistinguishable from an unknown code), and the error classification never
// distinguishes "wrong code" from "already checked in".
func (h *BookingHandler) CancelAppointment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CheckinCode string `json:"checkin_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Format permintaan tidak valid.")
		return
	}

	// Whitespace anywhere, not just at the ends: a code copied from an SMS or
	// read aloud often arrives with stray spaces, exactly as the status lookup
	// tolerates.
	code := strings.Join(strings.Fields(req.CheckinCode), "")
	if code == "" {
		writeError(w, http.StatusBadRequest, "Kode check-in wajib diisi.")
		return
	}
	if len(code) > maxPatientCodeLen || !safeCodeRe.MatchString(code) {
		writeError(w, http.StatusBadRequest, "Kode tidak valid.")
		return
	}

	// Rate limit with the SAME limiter instance the status lookup uses
	// (30 req/min per IP). WHY shared, not a second instance: a code must not
	// be probeable at 60 combined req/min by spreading attempts across the
	// status and cancel surfaces.
	if h.cancelLimiter != nil && !h.cancelLimiter.Allow(extractIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak permintaan. Coba lagi nanti.")
		return
	}

	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Exact match first, folded as a fallback — the same two-query strategy as
	// the status lookup. SELECTing id/facility_id/status and the owner account
	// id only; no patient data.
	const resolveSQL = `
		SELECT a.id, a.facility_id, a.status, a.patient_account_id
		FROM appointments a
		WHERE %s`

	id, facilityID, status, ownerAccountID, err := h.resolveByCheckinCode(
		ctx, fmt.Sprintf(resolveSQL, `upper(a.checkin_code) = upper($1)`), code)
	if err == errAmbiguousCode {
		writeError(w, http.StatusNotFound, "Kode tidak ditemukan.")
		return
	}
	if err == pgx.ErrNoRows {
		id, facilityID, status, ownerAccountID, err = h.resolveByCheckinCode(
			ctx,
			fmt.Sprintf(resolveSQL, `translate(upper(a.checkin_code), '01', 'OI') = translate(upper($1), '01', 'OI') LIMIT 2`),
			code,
		)
		if err == errAmbiguousCode {
			writeError(w, http.StatusNotFound, "Kode tidak ditemukan.")
			return
		}
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Kode tidak ditemukan.")
			return
		}
		slog.Error("cancel resolve failed", "err", err.Error())
		writeError(w, http.StatusInternalServerError, "Gagal memproses pembatalan.")
		return
	}

	// Server-side ownership re-check on cancel.
	//
	// An appointment claimed by a citizen account (patient_account_id NOT NULL)
	// may only be cancelled BY that account. Anonymous bookings (NULL) keep the
	// code-only model — holding the check-in code is still sufficient — and are
	// deliberately NOT forced into authentication. This is what stops a signed-in
	// citizen (or anyone holding a leaked code) from cancelling a DIFFERENT
	// citizen's owned appointment.
	actorAccountID := identity.ActorFromContext(r.Context()).PatientAccountID()
	if ownerAccountID != nil && (actorAccountID == "" || actorAccountID != *ownerAccountID) {
		writeError(w, http.StatusForbidden, "Anda tidak berhak membatalkan janji temu ini.")
		return
	}

	switch status {
	case "cancelled":
		// Idempotent: a double-tap or a retry after a dropped response reaches
		// an already-cancelled appointment and should not be reported as a loss.
		writeJSON(w, http.StatusOK, map[string]any{"status": status})
		return
	case "checked_in", "queued", "completed", "no_show":
		// The citizen is past the point they still control the appointment;
		// staff must handle these. WHY 409 rather than a silent no-op: saying
		// "cancelled" here would be false — the appointment is still live.
		writeError(w, http.StatusConflict, "Janji temu sudah tidak dapat dibatalkan.")
		return
	case "scheduled":
		// Proceed to the guarded transition below.
	default:
		// Any status the codebase does not model is treated as
		// non-cancellable-by-citizen rather than risk cancelling something
		// staff owns.
		writeError(w, http.StatusConflict, "Janji temu sudah tidak dapat dibatalkan.")
		return
	}

	// Guarded transition: WHERE id=$1 AND status='scheduled' so a concurrent
	// request that already moved this appointment cannot be overwritten.
	ct, err := h.pool.Exec(ctx,
		`UPDATE appointments SET status = 'cancelled', cancelled_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND status = 'scheduled'`, id)
	if err != nil {
		slog.Error("cancel transition failed", "appointment_id", id, "err", err.Error())
		writeError(w, http.StatusInternalServerError, "Gagal membatalkan janji temu.")
		return
	}
	if ct.RowsAffected() == 0 {
		// Lost a race: the appointment left 'scheduled' between resolve and
		// update. Report the conflict rather than claiming success.
		writeError(w, http.StatusConflict, "Janji temu sudah tidak dapat dibatalkan.")
		return
	}

	h.logBookingEvent(r, "appointment.cancelled", "ok",
		fmt.Sprintf("facility=%s appointment=%s", facilityID, id))

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{
			"appointment_id": id,
			"status":         "cancelled",
		},
	})
}

// resolveByCheckinCode runs one lookup and returns the resolved appointment's
// id, facility id, current status, and owner account id (NULL for anonymous
// bookings), following lookupBy's ambiguity contract (more than one canonical
// row → errAmbiguousCode).
func (h *BookingHandler) resolveByCheckinCode(ctx context.Context, query, code string) (id, facilityID, status string, ownerAccountID *string, err error) {
	rows, err := h.pool.Query(ctx, query, code)
	if err != nil {
		return "", "", "", nil, err
	}
	defer rows.Close()

	matched := 0
	for rows.Next() {
		matched++
		if matched > 1 {
			return "", "", "", nil, errAmbiguousCode
		}
		if err := rows.Scan(&id, &facilityID, &status, &ownerAccountID); err != nil {
			return "", "", "", nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", "", nil, err
	}
	if matched == 0 {
		return "", "", "", nil, pgx.ErrNoRows
	}
	return id, facilityID, status, ownerAccountID, nil
}

// PublicAppointmentsRouter dispatches public appointment endpoints.
// POST /api/v1/appointments               → BookAppointment
// POST /api/v1/appointments/cancel        → CancelAppointment
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
		if len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "appointments" && parts[3] == "cancel" {
			h.CancelAppointment(w, r)
			return
		}
		if len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "appointments" && parts[4] == "check-in" {
			h.CheckIn(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// rollbackCheckIn releases an appointment this request claimed but could not
// finalize. The guard keeps the release idempotent and stops it from undoing a
// concurrent request that already completed the transition.
//
// The release runs on a context detached from the request: the failure that
// brought us here is often a timeout, and a rollback inheriting the expired
// deadline would leave exactly the stranded row it exists to prevent.
func (h *BookingHandler) rollbackCheckIn(ctx context.Context, id string) (int64, error) {
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	ct, err := h.pool.Exec(rbCtx,
		`UPDATE appointments SET status = 'scheduled', checkin_at = NULL, updated_at = NOW() WHERE id = $1 AND status = 'checked_in' AND queue_ticket_id IS NULL`, id)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// verifyQueueTicketVisible confirms the ticket the queue service issued actually
// exists in THIS database, before the appointment is allowed to reference it.
//
// A non-UUID id is rejected here rather than by the uuid column, so a
// misbehaving queue service surfaces as a named error instead of a driver-level
// syntax failure. A UUID that is absent means the engine committed somewhere
// else — see the comment at the call site for why that is reported rather than
// repaired.
func (h *BookingHandler) verifyQueueTicketVisible(ctx context.Context, ticketID string) error {
	if _, err := uuid.Parse(ticketID); err != nil {
		return fmt.Errorf("queue service returned a non-UUID ticket id %q: %w", ticketID, err)
	}

	var exists bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM queue_tickets WHERE id = $1)`, ticketID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("look up queue ticket: %w", err)
	}
	if !exists {
		return fmt.Errorf("queue ticket %s was issued but is not in this database", ticketID)
	}
	return nil
}

// recordQueueTicket inserts a queue ticket this process minted itself.
//
// It exists for exactly one caller: the SIGAP_ENGINE_FALLBACK=dev path, where
// the in-memory FakeQueueService answers without touching any database, so the
// API must persist the ticket or the appointment would reference nothing. It is
// NOT used to paper over a real engine that committed elsewhere — that case is
// reported by verifyQueueTicketVisible.
func (h *BookingHandler) recordQueueTicket(ctx context.Context, now time.Time, ticketID, facilityID, patientName, patientPhone, formattedNumber string) error {
	dayStart, dayEnd := serviceDayRange(now)
	_, err := h.pool.Exec(ctx,
		`WITH p AS (
		    INSERT INTO patients (id, full_name, phone, date_of_birth)
		    VALUES ($1::uuid, $2, $3, '2000-01-01')
		    ON CONFLICT (phone) DO UPDATE SET full_name = EXCLUDED.full_name
		    RETURNING id
		)
		INSERT INTO queue_tickets (id, facility_id, patient_id, queue_number, formatted_number, status, registered_at)
		SELECT $4::uuid, $5::uuid, p.id,
		       (SELECT COALESCE(MAX(qt.queue_number), 0) + 1 FROM queue_tickets qt
		         WHERE qt.facility_id = $5::uuid AND qt.registered_at >= $7 AND qt.registered_at < $8),
		       $6, 'waiting', $9
		FROM p
		ON CONFLICT (id) DO NOTHING`,
		uuid.New().String(), patientName, patientPhone, ticketID, facilityID, formattedNumber, dayStart, dayEnd, now)
	return err
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
