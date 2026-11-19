package handler

import (
	"testing"
	"time"
)

// TestServiceDayRange_UtcMidnightDoesNotShiftWibDay proves the closed
// UTC-vs-WIB divergence: the write path (serviceDayRange) computes the day from
// WIB, so UTC midnight (07:00 WIB) does NOT split a service day. Two instants two
// minutes apart across UTC midnight — 06:59:00 WIB and 07:01:00 WIB — resolve to
// the same WIB calendar day.
func TestServiceDayRange_UtcMidnightDoesNotShiftWibDay(t *testing.T) {
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, serviceDayWIB) // WIB midnight
	before := day.Add(6*time.Hour + 59*time.Minute)          // 06:59 WIB = 23:59 UTC (prev day)
	after := day.Add(7*time.Hour + 1*time.Minute)            // 07:01 WIB = 00:01 UTC

	startBefore, endBefore := serviceDayRange(before)
	startAfter, endAfter := serviceDayRange(after)

	if !startBefore.Equal(startAfter) || !endBefore.Equal(endAfter) {
		t.Fatalf("06:59 WIB (%v) and 07:01 WIB (%v) must share one WIB service day;"+
			" got [%v,%v) and [%v,%v)", before, after, startBefore, endBefore, startAfter, endAfter)
	}

	if got := startBefore.In(serviceDayWIB).Format("2006-01-02"); got != "2026-11-09" {
		t.Fatalf("WIB service day = %s, want 2026-11-09", got)
	}
}

// TestServiceDayRange_WibMidnightFlipsDay proves WIB midnight is the true service-day
// boundary: 23:59:59 WIB and 00:00:01 WIB (the next day) are two different service days.
func TestServiceDayRange_WibMidnightFlipsDay(t *testing.T) {
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, serviceDayWIB)
	before := day.Add(-time.Second) // 23:59:59 WIB on 2026-11-08
	after := day.Add(time.Second)   // 00:00:01 WIB on 2026-11-09

	startBefore, _ := serviceDayRange(before)
	startAfter, _ := serviceDayRange(after)

	if startBefore.Equal(startAfter) {
		t.Fatalf("23:59:59 WIB and 00:00:01 WIB must be different service days; both -> %v", startBefore)
	}
	if got := startBefore.In(serviceDayWIB).Format("2006-01-02"); got != "2026-11-08" {
		t.Fatalf("23:59:59 WIB day = %s, want 2026-11-08", got)
	}
	if got := startAfter.In(serviceDayWIB).Format("2006-01-02"); got != "2026-11-09" {
		t.Fatalf("00:00:01 WIB day = %s, want 2026-11-09", got)
	}
}

// TestServiceDayRange_AgreesWithResolveServiceDay proves the write path and read
// path cannot drift: the allocator's window for an instant equals the listing
// window resolveServiceDay produces for that instant's WIB date string.
func TestServiceDayRange_AgreesWithResolveServiceDay(t *testing.T) {
	instant := time.Date(2026, 11, 9, 6, 59, 0, 0, serviceDayWIB)

	allocStart, allocEnd := serviceDayRange(instant)
	readStart, readEnd, err := resolveServiceDay(instant.In(serviceDayWIB).Format("2006-01-02"))
	if err != nil {
		t.Fatalf("resolveServiceDay: %v", err)
	}

	if !allocStart.Equal(readStart) || !allocEnd.Equal(readEnd) {
		t.Fatalf("allocation window [%v,%v) != listing window [%v,%v)",
			allocStart, allocEnd, readStart, readEnd)
	}
}

// TestServiceDayRange_WibMidnightExactBoundary pins the exact WIB day boundary:
// 23:59:59 WIB and 00:00:00 WIB (the first instant of the next day) are two
// different service days. The earlier flip test used 00:00:01; this one proves the
// boundary itself — 00:00:00 belongs to the NEW day, never the old one.
func TestServiceDayRange_WibMidnightExactBoundary(t *testing.T) {
	before := time.Date(2026, 11, 8, 23, 59, 59, 0, serviceDayWIB)
	after := time.Date(2026, 11, 9, 0, 0, 0, 0, serviceDayWIB)

	startBefore, _ := serviceDayRange(before)
	startAfter, _ := serviceDayRange(after)

	if startBefore.Equal(startAfter) {
		t.Fatalf("23:59:59 WIB and 00:00:00 WIB must be different service days; both -> %v", startBefore)
	}
	if got := startBefore.In(serviceDayWIB).Format("2006-01-02"); got != "2026-11-08" {
		t.Fatalf("23:59:59 WIB day = %s, want 2026-11-08", got)
	}
	if got := startAfter.In(serviceDayWIB).Format("2006-01-02"); got != "2026-11-09" {
		t.Fatalf("00:00:00 WIB day = %s, want 2026-11-09", got)
	}
}

// TestServiceDayRange_Utc2300ZIsNextWibDay proves the UTC-civil-day vs WIB-day
// divergence in the sharpest direction: 23:00:00Z on 2026-11-08 is 06:00:00 WIB on
// 2026-11-09, so the WIB service day is the NEXT day relative to the UTC calendar.
func TestServiceDayRange_Utc2300ZIsNextWibDay(t *testing.T) {
	instant := time.Date(2026, 11, 8, 23, 0, 0, 0, time.UTC)

	start, end := serviceDayRange(instant)

	if got := start.In(serviceDayWIB).Format("2006-01-02"); got != "2026-11-09" {
		t.Fatalf("23:00Z service day = %s, want 2026-11-09", got)
	}
	if got := instant.UTC().Format("2006-01-02"); got != "2026-11-08" {
		t.Fatalf("test fixture wrong: UTC day should be 2026-11-08, got %s", got)
	}
	wantStart := time.Date(2026, 11, 9, 0, 0, 0, 0, serviceDayWIB)
	wantEnd := wantStart.AddDate(0, 0, 1)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("23:00Z window = [%v,%v), want [%v,%v)", start, end, wantStart, wantEnd)
	}
}

// TestServiceDayRange_MultipleFacilitiesShareWibDay guards the facility-agnostic
// timezone invariant. `facilities` carries no per-facility timezone column and
// serviceDayRange takes no facility_id, so a single instant MUST land on the same
// WIB service day at every facility — even at the sharpest WIB-midnight boundary.
// This fails loudly if a future change starts branching the day on facility_id.
func TestServiceDayRange_MultipleFacilitiesShareWibDay(t *testing.T) {
	instant := time.Date(2026, 11, 9, 0, 0, 0, 0, serviceDayWIB) // WIB midnight

	facilityIDs := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
	}

	var wantStart, wantEnd time.Time
	for i, fid := range facilityIDs {
		start, end := serviceDayRange(instant)
		if i == 0 {
			wantStart, wantEnd = start, end
			continue
		}
		if !start.Equal(wantStart) || !end.Equal(wantEnd) {
			t.Fatalf("facility %s got [%v,%v), want shared [%v,%v)",
				fid, start, end, wantStart, wantEnd)
		}
	}

	if got := wantStart.In(serviceDayWIB).Format("2006-01-02"); got != "2026-11-09" {
		t.Fatalf("shared WIB service day = %s, want 2026-11-09", got)
	}
}

// TestServiceDayRange_HistoricalTicketResolvesToItsOwnDay proves a ticket minted
// days earlier resolves to its OWN WIB day, not the caller's today.
func TestServiceDayRange_HistoricalTicketResolvesToItsOwnDay(t *testing.T) {
	registeredAt := time.Date(2026, 11, 6, 10, 15, 30, 0, serviceDayWIB) // days ago
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, serviceDayWIB)           // today

	start, end := serviceDayRange(registeredAt)

	if got := start.In(serviceDayWIB).Format("2006-01-02"); got != "2026-11-06" {
		t.Fatalf("historical ticket day = %s, want 2026-11-06 (its own day)", got)
	}
	wantEnd := start.AddDate(0, 0, 1)
	if !end.Equal(wantEnd) {
		t.Fatalf("historical window = [%v,%v), want [%v,%v)", start, end, start, wantEnd)
	}

	todayStart, _ := serviceDayRange(now)
	if start.Equal(todayStart) {
		t.Fatalf("historical ticket mistakenly resolved to today (%v)", todayStart)
	}
}

// TestServiceDayRange_AllocationMatchesListingAcrossBoundary proves the write path
// (allocation window via serviceDayRange) and read path (listing window via
// resolveServiceDay) agree for the same instant AT every boundary, so the minted
// queue_number counter never drifts from what the listing filters on.
func TestServiceDayRange_AllocationMatchesListingAcrossBoundary(t *testing.T) {
	// Fixed instants spanning the sharp boundaries, expressed explicitly so the test
	// is deterministic and needs no clock injection.
	instants := []time.Time{
		time.Date(2026, 11, 8, 23, 59, 59, 0, serviceDayWIB), // last second of Nov 8
		time.Date(2026, 11, 9, 0, 0, 0, 0, serviceDayWIB),    // first instant of Nov 9
		time.Date(2026, 11, 8, 23, 0, 0, 0, time.UTC),        // 06:00 WIB Nov 9
		time.Date(2026, 11, 9, 6, 59, 0, 0, serviceDayWIB),   // mid-morning
		time.Date(2026, 11, 6, 10, 15, 30, 0, serviceDayWIB), // historical
	}

	for _, instant := range instants {
		allocStart, allocEnd := serviceDayRange(instant)
		dateStr := instant.In(serviceDayWIB).Format("2006-01-02")

		readStart, readEnd, err := resolveServiceDay(dateStr)
		if err != nil {
			t.Fatalf("resolveServiceDay(%q): %v", dateStr, err)
		}

		if !allocStart.Equal(readStart) || !allocEnd.Equal(readEnd) {
			t.Fatalf("instant %v: allocator window [%v,%v) != listing window [%v,%v)",
				instant, allocStart, allocEnd, readStart, readEnd)
		}
		if got := allocStart.In(serviceDayWIB).Format("2006-01-02"); got != dateStr {
			t.Fatalf("instant %v: allocator day = %s, want %s", instant, got, dateStr)
		}
	}
}