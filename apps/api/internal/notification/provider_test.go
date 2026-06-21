package notification

import (
	"testing"

	"github.com/google/uuid"
)

func TestDevSimulateOutcome_Deterministic(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	a := DevSimulateOutcome(id)
	b := DevSimulateOutcome(id)
	if a.Status != b.Status {
		t.Errorf("DevSimulateOutcome not deterministic for status: %v vs %v", a.Status, b.Status)
	}
	if a.Excerpt != b.Excerpt {
		t.Errorf("DevSimulateOutcome not deterministic for excerpt: %q vs %q", a.Excerpt, b.Excerpt)
	}
	if (a.ErrorCode == nil) != (b.ErrorCode == nil) {
		t.Errorf("DevSimulateOutcome not deterministic for error-code presence")
	}
	if a.ErrorCode != nil && *a.ErrorCode != *b.ErrorCode {
		t.Errorf("DevSimulateOutcome not deterministic for error code: %q vs %q", *a.ErrorCode, *b.ErrorCode)
	}
}

func TestDevSimulateOutcome_ValidStatus(t *testing.T) {
	// Walk a thousand UUIDs; every outcome must be in the declared set.
	delivered := 0
	failed := 0
	for i := 0; i < 1000; i++ {
		id, err := uuid.NewRandom()
		if err != nil {
			t.Fatalf("uuid.NewRandom: %v", err)
		}
		out := DevSimulateOutcome(id)
		if out.Status != StatusDelivered && out.Status != StatusFailed {
			t.Errorf("DevSimulateOutcome(%v) returned unexpected status %q", id, out.Status)
		}
		switch out.Status {
		case StatusDelivered:
			delivered++
		case StatusFailed:
			failed++
		}
	}
	// ~75/25 split. Allow generous slack for random variance.
	if delivered < 600 || delivered > 850 {
		t.Errorf("Delivered/failed split is %d/%d; expected ~75%% delivered", delivered, failed)
	}
	if failed < 150 || failed > 400 {
		t.Errorf("Delivered/failed split is %d/%d; expected ~25%% failed", delivered, failed)
	}
}

func TestDevSimulateOutcome_FailedHasErrorCode(t *testing.T) {
	// Find at least one UUID that maps to failed by sampling.
	for i := 0; i < 200; i++ {
		id, err := uuid.NewRandom()
		if err != nil {
			t.Fatalf("uuid.NewRandom: %v", err)
		}
		out := DevSimulateOutcome(id)
		if out.Status == StatusFailed {
			if out.ErrorCode == nil || *out.ErrorCode == "" {
				t.Errorf("Failed outcome %v has no error code", id)
			}
			if out.Excerpt == "" {
				t.Errorf("Failed outcome %v has empty excerpt", id)
			}
			return
		}
	}
	t.Skip("no failed outcome found in 200 random UUIDs (very unlikely)")
}

func TestDevSimulateOutcome_DeliveredHasNoErrorCode(t *testing.T) {
	for i := 0; i < 200; i++ {
		id, err := uuid.NewRandom()
		if err != nil {
			t.Fatalf("uuid.NewRandom: %v", err)
		}
		out := DevSimulateOutcome(id)
		if out.Status == StatusDelivered {
			if out.ErrorCode != nil {
				t.Errorf("Delivered outcome %v has unexpected error code: %q", id, *out.ErrorCode)
			}
			return
		}
	}
	t.Skip("no delivered outcome found in 200 random UUIDs (very unlikely)")
}
