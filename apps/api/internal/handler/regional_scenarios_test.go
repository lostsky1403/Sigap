package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// Regional scenarios at the API surface that are NOT already exercised by
// org_tenancy_test.go (admin scope / tenancy isolation) or service_day_test.go
// (pure WIB service-day internals).
//
//   Scenario A — citizen cross-org discovery + booking through the PUBLIC
//                catalog (ListPublicFacilities) and the public booking path.
//   Scenario F — multi-facility WIB-midnight queue allocation, DB-backed:
//                the per-facility, per-service-day counter resets at WIB
//                midnight independently at each facility, and the read path
//                (ListQueueTickets) agrees with the allocator.
//
// Reuses the disposable test_scope harness (newScopeTestPool) and the org
// seed helpers from org_tenancy_test.go.
// ---------------------------------------------------------------------------

// queueNumbersWithin returns the queue_numbers at a facility within the WIB
// service day containing `at`, ascending. It is the allocator-side assertion:
// the same half-open window the write path uses to assign numbers.
func queueNumbersWithin(t *testing.T, ctx context.Context, pool *pgxpool.Pool, facilityID string, at time.Time) []int {
	t.Helper()
	start, end := serviceDayRange(at)
	rows, err := pool.Query(ctx,
		`SELECT queue_number FROM queue_tickets
		 WHERE facility_id = $1 AND registered_at >= $2 AND registered_at < $3
		 ORDER BY queue_number`, facilityID, start, end)
	if err != nil {
		t.Fatalf("query queue numbers: %v", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan queue number: %v", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate queue numbers: %v", err)
	}
	return out
}

type listedQueueTicket struct {
	ID              string `json:"id"`
	FacilityID      string `json:"facility_id"`
	QueueNumber     int    `json:"queue_number"`
	FormattedNumber string `json:"formatted_number"`
}

// listQueueFor exercises the read path (ListQueueTickets) and decodes the rows.
func listQueueFor(t *testing.T, h *AdminHandler, actor identity.Actor, query string) []listedQueueTicket {
	t.Helper()
	rec := dispatchQueueRequest(h, http.MethodGet, "/api/v1/admin/queues"+query, "", actor)
	if rec.Code != http.StatusOK {
		t.Fatalf("list queues %q: got %d want 200: %s", query, rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []listedQueueTicket `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode queue list %q: %v", query, err)
	}
	return resp.Data
}

// queueNumbersByFacility groups a listing's rows into ascending per-facility
// sequences, for order-insensitive comparison against the allocator's output.
func queueNumbersByFacility(rows []listedQueueTicket) map[string][]int {
	out := map[string][]int{}
	for _, r := range rows {
		out[r.FacilityID] = append(out[r.FacilityID], r.QueueNumber)
	}
	for k := range out {
		sort.Ints(out[k])
	}
	return out
}

// TestRegionalScenarios_CitizenCrossOrgDiscoveryAndBooking proves a citizen can
// discover facilities across two organizations through the PUBLIC catalog and
// then book at one, without the catalog leaking org-internal records.
func TestRegionalScenarios_CitizenCrossOrgDiscoveryAndBooking(t *testing.T) {
	pool, cleanup := newScopeTestPool(t)
	defer cleanup()
	ctx := context.Background()

	orgA := uuid.NewString()
	orgB := uuid.NewString()
	facA := uuid.NewString()
	facB := uuid.NewString()
	suA := uuid.NewString()

	seedOrganization(ctx, pool, orgA, "Citizen Org A (synthetic)", "Jawa Barat")
	seedOrganization(ctx, pool, orgB, "Citizen Org B (synthetic)", "DKI Jakarta")
	seedOrgFacility(ctx, pool, facA, "Citizen Facility A", orgA)
	seedOrgFacility(ctx, pool, facB, "Citizen Facility B", orgB)
	seedOrgServiceUnit(ctx, pool, suA, facA, "Poli Umum A", "PUA")

	// --- Discovery: the PUBLIC catalog surfaces BOTH orgs' facilities and never
	// leaks org-internal records (organization_id or operational fields).
	ch := NewCatalogHandler(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/facilities", nil)
	rec := httptest.NewRecorder()
	ch.ListPublicFacilities(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public facility list status=%d want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, facA) || !strings.Contains(body, facB) {
		t.Fatalf("public catalog must surface both orgs' facilities; got %s", body)
	}
	for _, forbidden := range []string{"organization_id", "address", "phone", "total_beds", "available_beds"} {
		if strings.Contains(body, `"`+forbidden+`"`) {
			t.Fatalf("public catalog leaked org-internal field %q: %s", forbidden, body)
		}
	}

	// --- Booking: a citizen discovers facA and books at that chosen facility.
	bh := NewBookingHandler(pool, nil)
	aptTime := time.Now().UTC().Add(24 * time.Hour)
	booking := map[string]any{
		"facility_id":          facA,
		"service_unit_id":      suA,
		"patient_display_name": "Budi Warga",
		"patient_phone":        "081234567890",
		"appointment_time":     aptTime.Format(time.RFC3339),
	}
	raw, _ := json.Marshal(booking)
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", bytes.NewReader(raw))
	reqB.Header.Set("Content-Type", "application/json")
	recB := httptest.NewRecorder()
	bh.BookAppointment(recB, reqB)
	if recB.Code != http.StatusOK {
		t.Fatalf("book appointment status=%d want 200: %s", recB.Code, recB.Body.String())
	}

	var booked struct {
		Data struct {
			ID          string `json:"id"`
			CheckinCode string `json:"checkin_code"`
			Status      string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recB.Body.Bytes(), &booked); err != nil {
		t.Fatalf("decode booking: %v", err)
	}
	if booked.Data.ID == "" || booked.Data.CheckinCode == "" || booked.Data.Status != "scheduled" {
		t.Fatalf("unexpected booking payload: %s", recB.Body.String())
	}

	// The appointment must be persisted against the CHOSEN facility, not a
	// sibling org's facility, and in the scheduled state.
	var gotFacility, gotStatus string
	if err := pool.QueryRow(ctx,
		`SELECT facility_id::text, status::text FROM appointments WHERE id = $1`, booked.Data.ID,
	).Scan(&gotFacility, &gotStatus); err != nil {
		t.Fatalf("read persisted appointment: %v", err)
	}
	if gotFacility != facA || gotStatus != "scheduled" {
		t.Fatalf("booked appointment facility=%q status=%q want facA/scheduled", gotFacility, gotStatus)
	}
}

// TestRegionalScenarios_MultiFacilityWibMidnightQueueSequence proves the
// DB-backed allocator keeps a per-facility, per-WIB-service-day counter that
// (a) is independent across facilities, (b) resets at WIB midnight, and (c) is
// consistent with what ListQueueTickets returns for each facility and date.
func TestRegionalScenarios_MultiFacilityWibMidnightQueueSequence(t *testing.T) {
	pool, cleanup, _, _, facA, _, facB := orgTenancyFixture(t)
	defer cleanup()
	ctx := context.Background()

	// The allocator under test is the REAL check-in fallback path, not a
	// re-implementation of its SQL. Injecting `now` keeps the WIB boundary
	// deterministic while exercising the exact INSERT production runs.
	alloc := NewAdminHandler(pool)

	// Fixed WIB instants at the sharp service-day boundary.
	nov8LastSec := time.Date(2026, 11, 8, 23, 59, 59, 0, serviceDayWIB) // 2026-11-08T23:59:59+07:00
	nov9First := time.Date(2026, 11, 9, 0, 0, 0, 0, serviceDayWIB)      // 2026-11-09T00:00:00+07:00

	mint := func(facilityID string, at time.Time, formattedNumber string) string {
		t.Helper()
		ticketID := uuid.NewString()
		phone := "0811" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
		if err := alloc.recordAdminQueueTicket(ctx, at, ticketID, facilityID, "WIB Boundary Patient", phone, formattedNumber); err != nil {
			t.Fatalf("recordAdminQueueTicket(%s @ %s): %v", facilityID, at.Format(time.RFC3339), err)
		}
		return ticketID
	}

	// facA: two tickets within Nov 8 WIB => [1,2]; one on Nov 9 => reset [1].
	boundaryTicketID := mint(facA, nov8LastSec, "MFA-0001")
	mint(facA, nov8LastSec, "MFA-0002") // still 2026-11-08 => seq 2
	mint(facA, nov9First, "MFA-0003")   // new WIB day => counter resets to 1
	// facB: independent counter (NOT 3) and its own midnight reset.
	mint(facB, nov8LastSec, "MFB-0001")
	mint(facB, nov9First, "MFB-0002")

	assertInts := func(got []int, want ...int) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("queue numbers got %v want %v", got, want)
		}
	}

	// Allocator side: each facility's own WIB-day sequence, reset at midnight,
	// never mixing across facilities.
	assertInts(queueNumbersWithin(t, ctx, pool, facA, nov8LastSec), 1, 2)
	assertInts(queueNumbersWithin(t, ctx, pool, facA, nov9First), 1)
	assertInts(queueNumbersWithin(t, ctx, pool, facB, nov8LastSec), 1)
	assertInts(queueNumbersWithin(t, ctx, pool, facB, nov9First), 1)

	// Read side: ListQueueTickets must agree with the allocator, per facility
	// and per date.
	globalAdmin := uuid.NewString()
	if err := seedAppUser(ctx, pool, globalAdmin, "midnight-global"); err != nil {
		t.Fatal(err)
	}
	if err := seedGlobalSuperAdminRole(ctx, pool, globalAdmin); err != nil {
		t.Fatal(err)
	}
	opA := uuid.NewString()
	if err := seedAppUser(ctx, pool, opA, "midnight-op-a"); err != nil {
		t.Fatal(err)
	}
	seedNamedRoleAtFacility(t, pool, opA, "operator", facA)

	h := scopedHandler(pool)

	globalActor := dbScopedActor(t, pool, globalAdmin)
	nov8Rows := listQueueFor(t, h, globalActor, "?date=2026-11-08")
	nov9Rows := listQueueFor(t, h, globalActor, "?date=2026-11-09")

	gotNov8 := queueNumbersByFacility(nov8Rows)
	wantNov8 := map[string][]int{facA: {1, 2}, facB: {1}}
	if !reflect.DeepEqual(gotNov8, wantNov8) {
		t.Fatalf("global list date=2026-11-08: got %v want %v", gotNov8, wantNov8)
	}
	gotNov9 := queueNumbersByFacility(nov9Rows)
	wantNov9 := map[string][]int{facA: {1}, facB: {1}}
	if !reflect.DeepEqual(gotNov9, wantNov9) {
		t.Fatalf("global list date=2026-11-09: got %v want %v", gotNov9, wantNov9)
	}

	// Boundary agreement: the allocator's service day and the listing's date
	// scope must be the SAME WIB day. A ticket minted at 23:59:59 WIB Nov 8 is
	// a Nov-8 ticket — returned by ?date=2026-11-08 and NOT by ?date=2026-11-09.
	hasID := func(rows []listedQueueTicket, id string) bool {
		for _, r := range rows {
			if r.ID == id {
				return true
			}
		}
		return false
	}
	if !hasID(nov8Rows, boundaryTicketID) {
		t.Fatalf("boundary ticket %s (23:59:59 WIB Nov 8) missing from ?date=2026-11-08", boundaryTicketID)
	}
	if hasID(nov9Rows, boundaryTicketID) {
		t.Fatalf("boundary ticket %s (23:59:59 WIB Nov 8) must NOT appear under ?date=2026-11-09", boundaryTicketID)
	}

	// Per-facility listing consistency: an operator scoped to facA reads only
	// facA's tickets for that date, and the numbers match the allocator.
	opAActor := dbScopedActor(t, pool, opA)
	gotOpANov8 := queueNumbersByFacility(listQueueFor(t, h, opAActor, "?date=2026-11-08"))
	wantOpANov8 := map[string][]int{facA: {1, 2}}
	if !reflect.DeepEqual(gotOpANov8, wantOpANov8) {
		t.Fatalf("operator-A list date=2026-11-08: got %v want %v", gotOpANov8, wantOpANov8)
	}
}

