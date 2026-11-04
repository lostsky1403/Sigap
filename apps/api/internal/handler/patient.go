package handler

import (
	"context"
	"errors"
	"fmt"
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
	// ServiceUnitName is public service-catalog data: the unit a citizen
	// booked at (e.g. "Poli Umum"). It is already exposed, unauthenticated,
	// through GET /api/v1/public/service-units, so surfacing it here adds no
	// new disclosure surface. It is NOT appointment timing and carries no PII.
	ServiceUnitName   string `json:"service_unit_name,omitempty"`
	AppointmentStatus string `json:"appointment_status"`
	CheckinStatus     string `json:"checkin_status"`
	// QueueNumber is the integer position within the facility's day.
	// QueueFormattedNumber is the ticket string printed on the slip and shown
	// on the waiting-room screen ("PHB-0004"), which is what a citizen can
	// actually match against the display. The page renders the formatted one;
	// the integer alone would tell them to look for "4" on a board that shows
	// "PHB-0004".
	QueueNumber          *int    `json:"queue_number,omitempty"`
	QueueFormattedNumber *string `json:"queue_formatted_number,omitempty"`
	QueueStatus          *string `json:"queue_status,omitempty"`
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
	// Whitespace anywhere, not just at the ends. A code read aloud or copied
	// from an SMS arrives as "5ZE IOO" often enough that rejecting it with
	// "Kode tidak valid." blames the citizen for a formatting difference the
	// lookup can absorb for free.
	code := strings.Join(strings.Fields(r.URL.Query().Get("code")), "")
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

	// The exact match runs first and decides on its own. Folding is a fallback
	// for a code the citizen mistyped, never a replacement for a correct one.
	const lookupSQL = `
		SELECT f.name, a.status, su.name, qt.queue_number, qt.formatted_number, qt.status
		FROM appointments a
		JOIN facilities f ON f.id = a.facility_id
		LEFT JOIN service_units su ON su.id = a.service_unit_id
		LEFT JOIN queue_tickets qt ON qt.id = a.queue_ticket_id
		WHERE %s`

	resp, err := h.lookupBy(ctx, fmt.Sprintf(lookupSQL, `upper(a.checkin_code) = upper($1)`), code)
	if err == errAmbiguousCode {
		// Two rows share one check-in code. That should be impossible — the
		// generator is random and the column is meant to be unique — but it can
		// happen from a manual seed or a restored backup. Answering 500 would
		// report a server fault to a citizen holding a perfectly valid code, and
		// answering with either row would disclose one patient's visit to the
		// holder of another's code. So it fails closed as "not found", exactly
		// like the ambiguous fold below.
		writeError(w, http.StatusNotFound, "Kode tidak ditemukan.")
		return
	}
	if err == pgx.ErrNoRows {
		// No exact hit. Retry with both sides folded, and accept the result
		// ONLY when it is unique: two codes differing by a confusable pair both
		// canonicalise to the same value, and picking either would disclose one
		// patient's visit status to the holder of the other's code.
		resp, err = h.lookupBy(
			ctx,
			fmt.Sprintf(lookupSQL, `translate(upper(a.checkin_code), '01', 'OI') = translate(upper($1), '01', 'OI') LIMIT 2`),
			code,
		)
		if err == errAmbiguousCode {
			// Deliberately indistinguishable from "not found": telling the
			// caller that a code is ambiguous confirms that some code near it
			// exists, which is the enumeration signal the endpoint refuses to
			// give.
			writeError(w, http.StatusNotFound, "Kode tidak ditemukan.")
			return
		}
	}

	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Kode tidak ditemukan.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal mencari status pasien.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    resp,
	})
}

// errAmbiguousCode means more than one appointment canonicalises to the same
// code, so no single answer would be correct.
var errAmbiguousCode = errors.New("ambiguous check-in code")

// lookupBy runs one lookup and maps the row onto the public response shape.
func (h *PatientHandler) lookupBy(ctx context.Context, query, code string) (PatientStatusResponse, error) {
	var resp PatientStatusResponse
	var aptStatus *string
	var suName *string
	var qNum *int
	var qFormatted *string
	var qStatus *string

	rows, err := h.pool.Query(ctx, query, code)
	if err != nil {
		return resp, err
	}
	defer rows.Close()

	matched := 0
	for rows.Next() {
		matched++
		if matched > 1 {
			// The query that can return more than one row is the folded one.
			return resp, errAmbiguousCode
		}
		if err := rows.Scan(&resp.FacilityName, &aptStatus, &suName, &qNum, &qFormatted, &qStatus); err != nil {
			return resp, err
		}
	}
	if err := rows.Err(); err != nil {
		return resp, err
	}
	if matched == 0 {
		return resp, pgx.ErrNoRows
	}

	resp.FoundBy = "checkin_code"
	if aptStatus != nil {
		resp.AppointmentStatus = *aptStatus
		resp.CheckinStatus = mapCheckinStatus(*aptStatus)
	}
	if suName != nil {
		resp.ServiceUnitName = *suName
	}
	resp.QueueNumber = qNum
	resp.QueueFormattedNumber = qFormatted
	resp.QueueStatus = qStatus

	return resp, nil
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
