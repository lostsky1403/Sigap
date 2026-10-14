package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sigap/sigap/apps/api/internal/auth"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// Schedule mutation options (Phase 3B5.0)
//
// WHY THIS ENDPOINT EXISTS, and why it returns DOMAIN DATA rather than
// permissions.
//
// Phase 3B5 needs a ScheduleEditor, and a form that offers a facility the
// operator cannot act on is a lie the operator discovers only after submitting.
// The obvious fix — ask the browser what it may do — is the one thing this
// codebase must never do: the client holds `hasSession` and nothing else, and a
// browser that held a permission list would be an authorization authority that
// anyone can edit.
//
// So the server answers a DOMAIN question instead of a security question:
//
//     "Which facilities, and which service units within them, may I offer as
//      choices when building a schedule?"
//
// That answer is a list of ids and human-readable names. It carries no role, no
// permission name, no grant, no scope flag, and no user identity. A caller
// cannot use it to learn anything about the authorization model; it can only
// learn which options to render, which is exactly what a form needs.
//
// ADVISORY ONLY. Every mutation endpoint re-derives its own authorization from
// the stored resource's facility. An actor whose grant is revoked between this
// call and a later PATCH still gets 403/404 from the mutation itself. Nothing
// here is a grant.
// ---------------------------------------------------------------------------

// scheduleOptionServiceUnit is one selectable service unit.
//
// FacilityID is the PARENT facility, returned only to preserve the nesting
// relationship the editor renders. It is always a facility that already appears
// in the enclosing facility list, so it discloses nothing new.
type scheduleOptionServiceUnit struct {
	ID         string `json:"id"`
	FacilityID string `json:"facility_id"`
	Name       string `json:"name"`
}

// scheduleOptionFacility is one selectable facility and its service units.
type scheduleOptionFacility struct {
	ID           string                      `json:"id"`
	Name         string                      `json:"name"`
	ServiceUnits []scheduleOptionServiceUnit `json:"service_units"`
}

// scheduleOptionsResponse is the response body.
//
// An empty (non-nil) Facilities slice is meaningful and deliberate: it means
// "you hold schedule.manage at no facility", which the UI renders as a
// capability refusal. It is NOT an authorization error, and returning 403 here
// would be wrong — the caller IS authenticated, and answering 403 would
// reintroduce the "403 is ambiguous" problem this design exists to avoid.
type scheduleOptionsResponse struct {
	Facilities []scheduleOptionFacility `json:"facilities"`
}

// ListScheduleOptions handles GET /api/v1/admin/schedules/options.
//
// AUTHORIZATION MODEL, precisely:
//
//   - The route registry gates the request COARSELY on `schedule.read`. This is
//     the same gate the schedule LIST uses, so an operator who cannot read
//     schedules cannot discover their options either. It is deliberately NOT
//     `schedule.manage`: gating on manage here would 403 the mixed actor from
//     §5/§12 — one who manages only facility B but reads both — and would
//     answer a question about permissions with a status code.
//   - The FACILITY-LEVEL filter is the real control, and it is computed from
//     DB-resolved provenance by `AuthorizedFacilityIDsForPermission` with
//     `schedule.manage`. That helper intersects the actor's facility scope with
//     the facilities at which `schedule.manage` is actually granted, so an actor
//     who is a schedule reader at A and a schedule manager at B sees B only.
//
// A flat `actor.HasPermission("schedule.manage")` would return the whole scope
// and reintroduce exactly the Cartesian widening this codebase spent Phase 3B0
// removing. It is not used here.
func (h *AdminHandler) ListScheduleOptions(w http.ResponseWriter, r *http.Request) {
	actor := identity.ActorFromContext(r.Context())
	if h.pool == nil {
		writeError(w, http.StatusInternalServerError, "Database connection unavailable.")
		h.logScheduleAccess(r, actor, "schedule.options", "error", "nil pool")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// MANAGE provenance, not read provenance. This single argument is the whole
	// security property of this endpoint.
	manageSet, _ := h.listFacilityReadSet(r, actor, "schedule.manage")
	if manageSet.Err != nil || !manageSet.HasFacilities() {
		// Fail closed with an empty list, never with 403. A scope resolution
		// failure must not become a 500 that looks like a server fault, and it
		// must not become a 403 that the client would have to interpret.
		detail := "no facilities authorized for schedule.manage"
		status := "ok"
		if manageSet.Err != nil {
			detail = "scope resolution failed"
			status = "error"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"data":    scheduleOptionsResponse{Facilities: []scheduleOptionFacility{}},
		})
		h.logScheduleAccess(r, actor, "schedule.options", status, detail)
		return
	}

	facilities, err := h.loadScheduleOptionFacilities(ctx, manageSet)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil pilihan jadwal.")
		h.logScheduleAccess(r, actor, "schedule.options", "error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    scheduleOptionsResponse{Facilities: facilities},
	})
	h.logScheduleAccess(r, actor, "schedule.options", "ok", fmt.Sprintf("count=%d", len(facilities)))
}

// loadScheduleOptionFacilities reads the manageable facilities and their service
// units in a single query, so the two lists cannot be observed at different
// times and produce a service unit whose parent is missing.
func (h *AdminHandler) loadScheduleOptionFacilities(
	ctx context.Context,
	manageSet auth.FacilityReadSet,
) ([]scheduleOptionFacility, error) {
	const q = `
		SELECT f.id::text, f.name, su.id::text, su.name
		FROM facilities f
		LEFT JOIN service_units su
		       ON su.facility_id = f.id
		      AND su.is_active = true
		WHERE f.is_active = true
		  AND ($1::boolean OR f.id = ANY($2))
		ORDER BY f.name ASC, su.name ASC`

	// A facility with no active service unit still belongs in the list: the
	// editor must be able to show the facility and explain that it has no
	// service yet, rather than silently omitting it. `su.is_active` therefore
	// lives in the JOIN's ON clause, NOT in WHERE — a WHERE predicate on the
	// null-able side of an outer join discards the unmatched rows and silently
	// degrades the LEFT JOIN to an INNER JOIN, which would drop exactly those
	// facilities. The facility row survives with NULL unit columns, and the
	// rebuild below turns it into an entry with an empty service_units list.
	rows, err := h.pool.Query(ctx, q, manageSet.Unrestricted, manageSet.IDs)
	if err != nil {
		return nil, fmt.Errorf("query schedule options: %w", err)
	}
	defer rows.Close()

	facilities := make([]scheduleOptionFacility, 0, len(manageSet.IDs))
	index := make(map[string]int, len(manageSet.IDs))

	for rows.Next() {
		var facilityID, facilityName string
		var unitID, unitName *string
		if err := rows.Scan(&facilityID, &facilityName, &unitID, &unitName); err != nil {
			return nil, fmt.Errorf("scan schedule options: %w", err)
		}

		pos, ok := index[facilityID]
		if !ok {
			pos = len(facilities)
			index[facilityID] = pos
			facilities = append(facilities, scheduleOptionFacility{
				ID:           facilityID,
				Name:         facilityName,
				ServiceUnits: []scheduleOptionServiceUnit{},
			})
		}
		// A NULL service unit row means the facility has none active.
		if unitID == nil || unitName == nil {
			continue
		}
		facilities[pos].ServiceUnits = append(facilities[pos].ServiceUnits, scheduleOptionServiceUnit{
			ID:         *unitID,
			FacilityID: facilityID,
			Name:       *unitName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedule options: %w", err)
	}

	// Facilities only ever enter this slice from a row that passed the
	// is_active filter above, so there is nothing left to prune here.
	return facilities, nil
}
