package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/limiter"
)

// PatientHandler handles public patient status lookup endpoints.
// Unlike admin handlers, these endpoints require no authentication
// and expose only non-PII status information.
type PatientHandler struct {
	pool *pgxpool.Pool
}

// NewPatientHandler creates a handler backed by a pgx pool.
func NewPatientHandler(pool *pgxpool.Pool, _ *limiter.RateLimiter) *PatientHandler {
	return &PatientHandler{pool: pool}
}

// PatientStatusResponse is the public-facing status payload.
// Deliberately excludes any PII fields (patient_phone, patient_display_name,
// patient_id, recipient_contact, etc.).
type PatientStatusResponse struct {
	FoundBy              string  `json:"found_by"`
	FacilityName         string  `json:"facility_name"`
	AppointmentStatus    string  `json:"appointment_status"`
	AppointmentTime      string  `json:"appointment_time"`
	CheckinStatus        string  `json:"checkin_status"`
	QueueNumber          *int    `json:"queue_number,omitempty"`
	QueueStatus          *string `json:"queue_status,omitempty"`
	QueueFormattedNumber *string `json:"queue_formatted_number,omitempty"`
}

// PatientStatusLookup handles GET /api/v1/patient/status?code=...
// Public endpoint (no auth required). Looks up a patient's appointment and/or
// queue status by check-in code or formatted queue number.
//
// Lookup strategy:
//  1. Try matching by appointments.checkin_code (case-insensitive).
//  2. If not found, try matching by queue_tickets.formatted_number (case-insensitive).
//  3. If neither matches, return 404.
//
// NEVER returns patient_phone, patient_display_name, patient_id, or any PII.
func (h *PatientHandler) PatientStatusLookup(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, "Parameter 'code' diperlukan.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var resp PatientStatusResponse
	var aptStatus *string
	var aptTime *time.Time
	var qNum *int
	var qStatus *string
	var qFmtNum *string

	// Attempt 1: lookup by checkin_code on appointments table.
	err := h.pool.QueryRow(ctx,
		`SELECT f.name, a.status, a.appointment_time,
		        qt.queue_number, qt.status, qt.formatted_number
		 FROM appointments a
		 JOIN facilities f ON f.id = a.facility_id
		 LEFT JOIN queue_tickets qt ON qt.id = a.queue_ticket_id
		 WHERE LOWER(a.checkin_code) = LOWER($1)`, code,
	).Scan(&resp.FacilityName, &aptStatus, &aptTime,
		&qNum, &qStatus, &qFmtNum)

	if err != nil {
		if err != pgx.ErrNoRows {
			writeError(w, http.StatusInternalServerError, "Gagal mencari status pasien.")
			return
		}
		// Attempt 2: lookup by formatted_number on queue_tickets table.
		err = h.pool.QueryRow(ctx,
			`SELECT f.name, a.status, a.appointment_time,
			        qt.queue_number, qt.status, qt.formatted_number
			 FROM queue_tickets qt
			 JOIN facilities f ON f.id = qt.facility_id
			 LEFT JOIN appointments a ON a.queue_ticket_id = qt.id
			 WHERE LOWER(qt.formatted_number) = LOWER($1)`, code,
		).Scan(&resp.FacilityName, &aptStatus, &aptTime,
			&qNum, &qStatus, &qFmtNum)

		if err != nil {
			if err == pgx.ErrNoRows {
				writeError(w, http.StatusNotFound, "Kode tidak ditemukan.")
				return
			}
			writeError(w, http.StatusInternalServerError, "Gagal mencari status pasien.")
			return
		}
		resp.FoundBy = "formatted_number"
	} else {
		resp.FoundBy = "checkin_code"
	}

	if aptStatus != nil {
		resp.AppointmentStatus = *aptStatus
		resp.CheckinStatus = mapCheckinStatus(*aptStatus)
	}
	if aptTime != nil {
		resp.AppointmentTime = aptTime.Format(time.RFC3339)
	}
	resp.QueueNumber = qNum
	resp.QueueStatus = qStatus
	resp.QueueFormattedNumber = qFmtNum

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    resp,
	})
}

// mapCheckinStatus translates internal appointment status values into
// user-friendly Indonesian checkin status labels.
func mapCheckinStatus(status string) string {
	switch status {
	case "scheduled":
		return "not_checked_in"
	case "checked_in":
		return "checked_in"
	case "queued":
		return "in_queue"
	case "completed":
		return "selesai"
	case "cancelled":
		return "dibatalkan"
	case "no_show":
		return "tidak_hadir"
	default:
		return status
	}
}
