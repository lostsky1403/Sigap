package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The public facility catalog wire contract.
//
// This file is the guard rail for the Phase 3B2.1 additive change that put
// `type` on the public wire. It is written as an allowlist test rather than a
// "contains type" test on purpose: the danger of exposing one more field on a
// public endpoint is that the *next* field follows, and each addition looks
// individually reasonable. Pinning the complete key set means any future
// addition has to edit this file and be argued for explicitly, which is the
// point.
//
// The absence assertions are the load-bearing ones. `type` is a public
// classification, so exposing it is safe. `total_beds` and `available_beds`
// look equally harmless and are not: they are operational data an operator
// maintains by hand, and a stale bed count on a public page reads as a live
// claim about a real hospital.

// facilityTypeEnumValues is the exact set the facility_type column accepts,
// read from 0001_init.sql. Used to assert the wire value is a real enum member
// rather than a free-text string the handler happened to emit.
var facilityTypeEnumValues = []string{"puskesmas", "rumah_sakit"}

// adminOnlyFacilityFields are fields the admin API returns that the public
// catalog must never carry. Sourced from facilityResponse in admin.go.
var adminOnlyFacilityFields = []string{
	"address",
	"kecamatan",
	"kabupaten_kota",
	"provinsi",
	"phone",
	"total_beds",
	"available_beds",
	"created_at",
	"updated_at",
}

// allowedPublicFacilityFields is the complete, intended public key set.
var allowedPublicFacilityFields = []string{"id", "name", "short_code", "type", "is_active"}

func setupCatalogTest(t *testing.T) (*pgxpool.Pool, *CatalogHandler) {
	t.Helper()
	dbURL := os.Getenv("SIGAP_DATABASE_URL")
	if dbURL == "" {
		t.Skip("SIGAP_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool, NewCatalogHandler(pool)
}

// seedCatalogFacility inserts one facility with a known type and isActive, and
// removes it afterwards so the public catalog a citizen sees is not polluted
// by test rows.
func seedCatalogFacility(t *testing.T, pool *pgxpool.Pool, name, facilityType string, isActive bool) string {
	t.Helper()
	ctx := context.Background()
	id := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO facilities (id, name, type, address, kecamatan, kabupaten_kota, provinsi,
			phone, total_beds, available_beds, short_code, is_active)
		 VALUES ($1, $2, $3, 'Jl. Uji No. 1', 'Kec. Uji', 'Kab. Uji', 'Prov. Uji',
			'021-9999999', 42, 17, 'UJI', $4)`,
		id, name, facilityType, isActive)
	if err != nil {
		t.Fatalf("seed facility: %v", err)
	}
	t.Cleanup(func() {
		// ON DELETE RESTRICT on children means a stray dependent row would fail
		// cleanup and leave the fixture behind; the test ids are unique so a
		// hard delete is safe.
		_, _ = pool.Exec(context.Background(), `DELETE FROM facilities WHERE id = $1`, id)
	})
	return id
}

// callPublicFacilities issues the public request with no credentials at all.
func callPublicFacilities(t *testing.T, h *CatalogHandler) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/facilities", nil)
	// No Authorization header, no session, no identity in context. If this
	// endpoint ever grows an auth dependency, this test fails rather than the
	// public catalog quietly becoming private.
	rec := httptest.NewRecorder()
	h.ListPublicFacilities(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return rec.Code, envelope
}

func publicFacilityRows(t *testing.T, envelope map[string]any) []map[string]any {
	t.Helper()
	raw, ok := envelope["data"]
	if !ok {
		t.Fatalf("envelope has no data key: %v", envelope)
	}
	// The frontend normalizes `data: null` to []. The backend may legitimately
	// answer null when a query matches zero rows, so the test accepts both
	// shapes rather than demanding one and breaking the other.
	if raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("data is not a list: %T", raw)
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("data item is not an object: %T", item)
		}
		rows = append(rows, row)
	}
	return rows
}

// A. The response carries exactly id, name, short_code, type, is_active.
func TestListPublicFacilities_ExactFieldSet(t *testing.T) {
	pool, h := setupCatalogTest(t)
	seedCatalogFacility(t, pool, "Faskes Kontrak Field", "puskesmas", true)

	_, envelope := callPublicFacilities(t, h)
	rows := publicFacilityRows(t, envelope)
	if len(rows) == 0 {
		t.Fatal("expected at least one facility")
	}

	for _, row := range rows {
		got := make([]string, 0, len(row))
		for key := range row {
			got = append(got, key)
		}
		sort.Strings(got)
		want := append([]string(nil), allowedPublicFacilityFields...)
		sort.Strings(want)

		// Compare the whole key set, not a subset. An extra key fails here
		// even though every allowed key is present.
		if len(got) != len(want) {
			t.Fatalf("field set drifted.\n got: %v\nwant: %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("field set drifted.\n got: %v\nwant: %v", got, want)
			}
		}
	}
}

// B. `type` is a real facility_type enum value on every row.
func TestListPublicFacilities_TypeIsEnumValue(t *testing.T) {
	pool, h := setupCatalogTest(t)
	seedCatalogFacility(t, pool, "Faskes Enum A", "rumah_sakit", true)
	seedCatalogFacility(t, pool, "Faskes Enum B", "puskesmas", true)

	_, envelope := callPublicFacilities(t, h)
	rows := publicFacilityRows(t, envelope)
	if len(rows) == 0 {
		t.Fatal("expected seeded facilities")
	}

	valid := make(map[string]bool, len(facilityTypeEnumValues))
	for _, v := range facilityTypeEnumValues {
		valid[v] = true
	}
	for _, row := range rows {
		raw, ok := row["type"]
		if !ok {
			t.Fatal("row is missing the type field")
		}
		value, ok := raw.(string)
		if !ok {
			t.Fatalf("type is not a string: %T", raw)
		}
		if !valid[value] {
			t.Errorf("type %q is not a facility_type enum value %v", value, facilityTypeEnumValues)
		}
	}
}

// The wire value is the database enum verbatim, not a translated display
// string. The frontend owns human labels; if the server started emitting
// "Puskesmas" instead of "puskesmas", the client-side filter would silently
// match nothing.
func TestListPublicFacilities_TypeIsWireValueNotLabel(t *testing.T) {
	pool, h := setupCatalogTest(t)
	id := seedCatalogFacility(t, pool, "Faskes Nilai Wire", "puskesmas", true)

	_, envelope := callPublicFacilities(t, h)
	for _, row := range publicFacilityRows(t, envelope) {
		if row["id"] != id {
			continue
		}
		if got := row["type"]; got != "puskesmas" {
			t.Fatalf("expected the raw enum value 'puskesmas', got %v", got)
		}
	}
}

// C. Only active facilities are returned.
func TestListPublicFacilities_ExcludesInactive(t *testing.T) {
	pool, h := setupCatalogTest(t)
	activeID := seedCatalogFacility(t, pool, "Faskes Aktif", "puskesmas", true)
	inactiveID := seedCatalogFacility(t, pool, "Faskes Nonaktif", "rumah_sakit", false)

	_, envelope := callPublicFacilities(t, h)
	var sawInactive bool
	for _, row := range publicFacilityRows(t, envelope) {
		switch row["id"] {
		case activeID:
			if row["is_active"] != true {
				t.Error("the active facility reported is_active false")
			}
		case inactiveID:
			sawInactive = true
		}
	}
	if sawInactive {
		t.Error("an inactive facility was returned by the public catalog")
	}
}

// D. Sensitive and admin-only fields remain absent.
func TestListPublicFacilities_NoAdminOnlyFields(t *testing.T) {
	pool, h := setupCatalogTest(t)
	seedCatalogFacility(t, pool, "Faskes Tanpa Rahasia", "rumah_sakit", true)

	_, envelope := callPublicFacilities(t, h)
	rows := publicFacilityRows(t, envelope)
	if len(rows) == 0 {
		t.Fatal("expected at least one facility")
	}

	for _, row := range rows {
		for _, forbidden := range adminOnlyFacilityFields {
			if _, present := row[forbidden]; present {
				t.Errorf("public row leaked admin-only field %q", forbidden)
			}
		}
	}
}

// D (structural). The exact-key-set test above already fails on any extra
// field; this one names the specific ones so a regression message says what
// leaked rather than just "field set drifted".
func TestListPublicFacilities_ExplicitlyNoBedCountsOrAddress(t *testing.T) {
	pool, h := setupCatalogTest(t)
	seedCatalogFacility(t, pool, "Faskes Bebas Informasi", "rumah_sakit", true)

	_, envelope := callPublicFacilities(t, h)
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// A substring check on the encoded body catches a field that is serialized
	// under a name the allowlist test did not anticipate.
	body := string(raw)
	for _, forbidden := range []string{
		"total_beds", "available_beds", "address", "phone",
		"kecamatan", "kabupaten_kota", "provinsi",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("public response body contains %q", forbidden)
		}
	}
}

// E. The endpoint is reachable with no authentication whatsoever.
func TestListPublicFacilities_NoAuthRequired(t *testing.T) {
	pool, h := setupCatalogTest(t)
	seedCatalogFacility(t, pool, "Faskes Publik", "puskesmas", true)

	// callPublicFacilities sends a bare request with no Authorization header
	// and no identity in context. Reaching 200 here is the assertion.
	status, envelope := callPublicFacilities(t, h)
	if status != http.StatusOK {
		t.Fatalf("public catalog returned %d without credentials", status)
	}
	if envelope["success"] != true {
		t.Errorf("expected success:true, got %v", envelope["success"])
	}
}

// F. Zero-row behaviour stays compatible with the frontend normalization.
func TestListPublicFacilities_EmptyCatalogIsNullOrEmpty(t *testing.T) {
	pool, h := setupCatalogTest(t)
	ctx := context.Background()

	// Isolate the query: deactivate everything, then restore. Using a
	// transaction-scoped flag would not work because the handler uses its own
	// pool, so this brackets the whole table and puts it back afterwards.
	rows, err := pool.Query(ctx, `SELECT id, is_active FROM facilities`)
	if err != nil {
		t.Fatalf("read facilities: %v", err)
	}
	type prior struct {
		id  string
		act bool
	}
	var restore []prior
	for rows.Next() {
		var p prior
		if err := rows.Scan(&p.id, &p.act); err != nil {
			t.Fatalf("scan: %v", err)
		}
		restore = append(restore, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	t.Cleanup(func() {
		for _, p := range restore {
			_, _ = pool.Exec(context.Background(),
				`UPDATE facilities SET is_active = $2 WHERE id = $1`, p.id, p.act)
		}
	})

	if _, err := pool.Exec(ctx, `UPDATE facilities SET is_active = false`); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/facilities", nil)
	rec := httptest.NewRecorder()
	h.ListPublicFacilities(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an empty catalog, got %d", rec.Code)
	}

	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Go encodes a nil slice as `data: null`. The web client normalizes both
	// null and [] to an empty array, so the only real requirement is a 200 with
	// a data key. Asserting a specific shape here would break the other
	// legitimate encoding for no benefit.
	if _, ok := envelope["data"]; !ok {
		t.Fatal("empty response has no data key")
	}
	if envelope["success"] != true {
		t.Errorf("expected success:true, got %v", envelope["success"])
	}
	// And the decoded helper must tolerate it, proving the two shapes converge.
	if got := publicFacilityRows(t, envelope); len(got) != 0 {
		t.Errorf("expected zero rows for an empty catalog, got %d", len(got))
	}
}
