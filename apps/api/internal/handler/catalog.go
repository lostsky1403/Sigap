package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CatalogHandler serves public, unauthenticated catalog data for the
// patient booking flow.  Responses contain only non-sensitive fields
// (id, name, short_code, type) — no addresses, phone numbers, or bed counts.
// The facility type is a public classification, not operational data, so it
// is safe to expose; the exact field set is pinned by catalog_test.go.
type CatalogHandler struct {
	pool *pgxpool.Pool
}

// NewCatalogHandler creates a catalog handler backed by a pgx pool.
func NewCatalogHandler(pool *pgxpool.Pool) *CatalogHandler {
	return &CatalogHandler{pool: pool}
}

// publicFacility is the minimal facility shape for the booking dropdown.
//
// `type` is the existing facilities.type column, whose values are the
// facility_type enum: 'rumah_sakit' and 'puskesmas'. It is exposed
// verbatim rather than translated here, because the wire value is a
// database enum and the human label belongs in the presentation layer.
// Translating server-side would give the same Indonesian label to an
// English client and would mean the filter had to un-translate it.
//
// The field set stays minimal and public. Deliberately absent, and
// asserted absent in catalog_test.go: address, kecamatan, kabupaten_kota,
// provinsi, phone, total_beds, available_beds, and the timestamps. Bed
// counts in particular are operational data an operator maintains, and a
// stale public bed count reads as a live operational claim.
type publicFacility struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortCode string `json:"short_code"`
	Type      string `json:"type"`
	IsActive  bool   `json:"is_active"`
}

// publicServiceUnit is the minimal service-unit shape for the booking dropdown.
type publicServiceUnit struct {
	ID         string `json:"id"`
	FacilityID string `json:"facility_id"`
	Name       string `json:"name"`
	Code       string `json:"code"`
	IsActive   bool   `json:"is_active"`
}

// ListPublicFacilities returns active facilities (id, name, short_code, type,
// is_active). No authentication required.
func (h *CatalogHandler) ListPublicFacilities(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := h.pool.Query(ctx,
		`SELECT id, name, short_code, type, is_active
		 FROM facilities
		 WHERE is_active = true
		 ORDER BY name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data fasilitas.")
		return
	}
	defer rows.Close()

	var results []publicFacility
	for rows.Next() {
		var f publicFacility
		// The scan order must match the SELECT order exactly. `type` is a
		// facility_type enum, which pgx scans into a string.
		if err := rows.Scan(&f.ID, &f.Name, &f.ShortCode, &f.Type, &f.IsActive); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data fasilitas.")
			return
		}
		results = append(results, f)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    results,
	})
}

// ListPublicServiceUnits returns active service units for a facility.
// No authentication required.  Accepts optional ?facility_id= filter.
func (h *CatalogHandler) ListPublicServiceUnits(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	facilityID := r.URL.Query().Get("facility_id")

	var rows pgx.Rows
	var err error

	if facilityID != "" {
		rows, err = h.pool.Query(ctx,
			`SELECT id, facility_id, name, code, is_active
			 FROM service_units
			 WHERE is_active = true AND facility_id = $1
			 ORDER BY name`, facilityID)
	} else {
		rows, err = h.pool.Query(ctx,
			`SELECT id, facility_id, name, code, is_active
			 FROM service_units
			 WHERE is_active = true
			 ORDER BY name`)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengambil data unit layanan.")
		return
	}
	defer rows.Close()

	var results []publicServiceUnit
	for rows.Next() {
		var u publicServiceUnit
		if err := rows.Scan(&u.ID, &u.FacilityID, &u.Name, &u.Code, &u.IsActive); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data unit layanan.")
			return
		}
		results = append(results, u)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    results,
	})
}
