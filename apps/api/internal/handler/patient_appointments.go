package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/sigap/sigap/apps/api/internal/identity"
)

// listMyAppointmentsSQL resolves a citizen's own appointments by their
// internal patient identity (appointments.patient_account_id). The
// check-in code is NEVER part of the WHERE clause: identity, not possession of
// a bearer code, is what scopes this list. The rows are ordered newest-first
// so the most relevant visit is what a citizen reads first.
const listMyAppointmentsSQL = `
	SELECT a.id, a.checkin_code, f.name, su.name, a.appointment_time, a.status
	FROM appointments a
	JOIN facilities f ON f.id = a.facility_id
	LEFT JOIN service_units su ON su.id = a.service_unit_id
	WHERE a.patient_account_id = $1
	ORDER BY a.appointment_time DESC`

// PatientAppointment is one appointment the signed-in citizen owns.
//
// `service_unit_name` is omitempty because the join is a LEFT JOIN: an
// appointment whose service unit was deleted (or never resolved) still has a
// facility and a time, and must still be listable without inventing a unit.
type PatientAppointment struct {
	ID              string    `json:"id"`
	CheckinCode     string    `json:"checkin_code"`
	FacilityName    string    `json:"facility_name"`
	ServiceUnitName string    `json:"service_unit_name,omitempty"`
	AppointmentTime time.Time `json:"appointment_time"`
	Status          string    `json:"appointment_status"`
}

// ListMyAppointments handles GET /api/v1/patient/appointments.
//
// Unlike the code-keyed status lookup, this endpoint is authenticated by
// ACCOUNT, not by possession of a check-in code. The actor must resolve to a
// real citizen account (identity.Actor.PatientAccountID() non-empty); a
// missing, dev, system, or staff actor fails closed with 401 because staff
// identities have their own appointment surface and a citizen's appointments
// are that citizen's alone.
//
// The query is scoped server-side to patient_account_id = that id. The client
// sends no identifier and no code; the rows returned are exactly the ones the
// authenticated account owns.
func (h *PatientHandler) ListMyAppointments(w http.ResponseWriter, r *http.Request) {
	accountID := identity.ActorFromContext(r.Context()).PatientAccountID()
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "Silakan masuk untuk melihat janji temu Anda.")
		return
	}

	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := h.pool.Query(ctx, listMyAppointmentsSQL, accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat janji temu Anda.")
		return
	}
	defer rows.Close()

	appointments := []PatientAppointment{}
	for rows.Next() {
		var appt PatientAppointment
		var serviceUnitName *string
		if err := rows.Scan(
			&appt.ID,
			&appt.CheckinCode,
			&appt.FacilityName,
			&serviceUnitName,
			&appt.AppointmentTime,
			&appt.Status,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal memuat janji temu Anda.")
			return
		}
		if serviceUnitName != nil {
			appt.ServiceUnitName = *serviceUnitName
		}
		appointments = append(appointments, appt)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat janji temu Anda.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    appointments,
	})
}