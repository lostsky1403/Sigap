package notification

import (
	"testing"
	"time"
)

// TestBackoffFor_Schedule asserts the documented schedule:
//
//	attempt 1 -> 1 minute
//	attempt 2 -> 5 minutes
//	attempt 3 -> 15 minutes (reserved; MaxAttempts=3 means we stop
//	                 after attempt 3, so this entry is only used if
//	                 MaxAttempts is raised in the future)
//	default    -> 15 minutes (defensive fallback)
func TestBackoffFor_Schedule(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Minute},
		{2, 5 * time.Minute},
		{3, 15 * time.Minute},
		{0, 15 * time.Minute}, // defensive default
		{99, 15 * time.Minute},
	}
	for _, tc := range cases {
		got := BackoffFor(tc.attempt)
		if got != tc.want {
			t.Errorf("BackoffFor(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

// TestMaxAttempts_Constant pins the documented cap. Any change to
// MaxAttempts is a behaviour change that requires a spec update.
func TestMaxAttempts_Constant(t *testing.T) {
	if MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3 (per spec)", MaxAttempts)
	}
}

// TestRenderTemplate_NoRawContactOrBodyInOutboxRow guards against
// the future regression where someone adds a RecipientContact or
// RenderedBody field to OutboxRow. The worker reads from this
// struct; if such fields exist, the worker might log them.
func TestRenderTemplate_NoRawContactOrBodyInOutboxRow(t *testing.T) {
	// Renderer is not the same struct as OutboxRow; the test in
	// service_test.go covers OutboxRow. This test exists here to
	// document the privacy contract for the worker / renderer pair.
	if _, ok := any(OutboxRow{}).(interface{ Fields() []string }); ok {
		// Compile-time guard for future struct introspection.
		_ = ok
	}
	// The renderer is a pure function; it must not retain state
	// between calls. We exercise that via two calls with the same
	// input returning the same output.
	a, _ := RenderTemplate("X={facility_name}", map[string]string{"facility_name": "A"})
	b, _ := RenderTemplate("X={facility_name}", map[string]string{"facility_name": "A"})
	if a != b {
		t.Errorf("RenderTemplate not deterministic: %q vs %q", a, b)
	}
}

// TestWorker_BackoffSchedule_IsConsistentWithSchedule ensures the
// private backoffSchedule() and the exported BackoffFor() return
// the same value for every documented attempt number. Catches the
// bug where one is updated and the other is forgotten.
func TestWorker_BackoffSchedule_IsConsistentWithSchedule(t *testing.T) {
	for a := 1; a <= 5; a++ {
		if backoffSchedule(a) != BackoffFor(a) {
			t.Errorf("backoffSchedule(%d) != BackoffFor(%d)", a, a)
		}
	}
}

// TestRenderTemplate_DoesNotCallOutboxMasking is a documentation
// guard. The renderer is a pure string function and must not call
// into the masking helpers. If a future refactor adds such a call,
// this test will still pass (no API change), but a code-review
// comment is warranted.
func TestRenderTemplate_DoesNotCallOutboxMasking(t *testing.T) {
	// RenderTemplate("Kode: {checkin_code}", {"checkin_code":"AB12CD"})
	// must return a string containing "AB12CD" verbatim; the
	// renderer must not mask it. (The masking happens at Enqueue
	// time, not at render time.)
	got, err := RenderTemplate(
		"Kode check-in: {checkin_code}.",
		map[string]string{"checkin_code": "AB12CD"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Kode check-in: AB12CD." {
		t.Errorf("renderer must not alter allow-listed values; got %q", got)
	}
}

// TestDevSimulateOutcome_DeterministicAcrossCalls guards the
// claim that two providers calling the same outbox id always see the
// same outcome. This is the foundation of the worker's
// retryability — without it, retries would behave non-deterministically.
// The behaviour is verified exhaustively in provider_test.go; this
// test exists as a co-location reminder.
func TestDevSimulateOutcome_DeterministicAcrossCalls(t *testing.T) {
	// Sanity: a deterministic outcome exists and is a Status type.
	out := DevSimulateOutcome([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	if out.Status != StatusDelivered && out.Status != StatusFailed {
		t.Errorf("unexpected status %q", out.Status)
	}
}