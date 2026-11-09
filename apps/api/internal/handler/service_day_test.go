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