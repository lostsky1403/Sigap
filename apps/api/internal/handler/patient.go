package handler

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/limiter"
	"github.com/sigap/sigap/apps/api/internal/router"
)

const maxPatientCodeLen = 64

var safeCodeRe = regexp.MustCompile(`^[A-Za-z0-9\-]+$`)

// PatientHandler handles public patient status lookup endpoints.
// Unlike admin handlers, these endpoints require no authentication
// and expose only non-PII status information.
type PatientHandler struct {
	pool *pgxpool.Pool
	rl   *limiter.RateLimiter
}

// NewPatientHandler creates a handler backed by a pgx pool and rate limiter.
func NewPatientHandler(pool *pgxpool.Pool, rl *limiter.RateLimiter) *PatientHandler {
	return &PatientHandler{pool: pool, rl: rl}
}

// PatientStatusResponse is the public-facing status payload.
// Deliberately excludes any PII fields (patient_phone, patient_display_name,
// patient_id, recipient_contact, etc.).
//
// SECURITY: appointment_time is no longer included in the response.
// Exposure of appointment timing enables correlation attacks (visiting
// patterns, facility attendance) and is PHI-adjacent data.
type PatientStatusResponse struct {
	FoundBy         string `json:"found_by"`
	FacilityName    string `json:"facility_name"`
	AppointmentStatus string `json:"appointment_status"`
	CheckinStatus   string `json:"checkin_status"`
	QueueNumber     *int   `json:"queue_number,omitempty"`
	QueueStatus     *string `json:"queue_status,omitempty"`
}

// PatientStatusLookup handles GET /api/v1/patient/status?code=...
//
// SECURITY FIX (vuln-0003): This endpoint is now gated for the
// formatted_number lookup strategy. Only checkin_code lookup is available
// — these are randomly generated 6-char codes that are not sequential
// and therefore not enumerable.
//
// The endpoint still allows unauthenticated access because the checkin_code
// is a random token the patient already possesses. However:
//   - appointment_time is no longer returned (PHI-adjacent timing data).
//   - The formatted_number lookup has been removed entirely, closing the
//     enumeration vector (RSK-0001, RSK-0002, ...).
//   - Rate limiting remains per-IP at 30 req/min.
func (h *PatientHandler) PatientStatusLookup(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, "Parameter 'code' diperlukan.")
		return
	}

	if len(code) > maxPatientCodeLen || !safeCodeRe.MatchString(code) {
		writeError(w, http.StatusBadRequest, "Kode tidak valid.")
		return
	}

	// Rate limit by client IP.
	if h.rl != nil && !h.rl.Allow(extractIP(r)) {
		writeError(w, http.StatusTooManyRequests, "Terlalu banyak permintaan. Coba lagi nanti.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var resp PatientStatusResponse
	var aptStatus *string
	var qNum *int
	var qStatus *string

	// Only lookup by checkin_code — random, non-enumerable token.
	// The formatted_number lookup strategy has been removed to prevent
	// sequential enumeration of patient appointment data.
	err := h.pool.QueryRow(ctx,
		`SELECT f.name, a.status,
		        qt.queue_number, qt.status
		 FROM appointments a
		 JOIN facilities f ON f.id = a.facility_id
		 LEFT JOIN queue_tickets qt ON qt.id = a.queue_ticket_id
		 WHERE LOWER(a.checkin_code) = LOWER($1)`, code,
	).Scan(&resp.FacilityName, &aptStatus, &qNum, &qStatus)

	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Kode tidak ditemukan.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mencari status pasien.")
		return
	}

	resp.FoundBy = "checkin_code"
	if aptStatus != nil {
		resp.AppointmentStatus = *aptStatus
		resp.CheckinStatus = mapCheckinStatus(*aptStatus)
	}
	resp.QueueNumber = qNum
	resp.QueueStatus = qStatus

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    resp,
	})
}

// extractIP returns the sanitized client IP from TrustedProxy middleware
// context; falls back to RemoteAddr if the middleware has not run.
func extractIP(r *http.Request) string {
	return router.ClientIPFromContext(r)
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
