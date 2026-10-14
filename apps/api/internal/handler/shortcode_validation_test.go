package handler

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sigap/sigap/apps/api/internal/notification"
)

// TestValidateShortCode_AcceptsExistingSeedValues guards against a validation
// change that would make the committed seed data un-insertable. These are the
// short_code values in packages/db/seed/dev.sql and packages/db/seed/demo.sql.
func TestValidateShortCode_AcceptsExistingSeedValues(t *testing.T) {
	for _, code := range []string{"RSK", "PKM", "RSM", "PMI", "RSJ", "PHB", "f1", "DEMO-GIGI", "DEMO-UMUM"} {
		if err := validateShortCode(code); err != nil {
			t.Errorf("validateShortCode(%q) = %v, want nil (committed seed value)", code, err)
		}
	}
}

func TestValidateShortCode_AcceptsSafeNewValues(t *testing.T) {
	for _, code := range []string{"RSUD", "RS1", "A", "UPT12", "RS-A1", "Puskesmas7"} {
		if err := validateShortCode(code); err != nil {
			t.Errorf("validateShortCode(%q) = %v, want nil", code, err)
		}
	}
}

// TestValidateShortCode_AcceptsTheE2ESuitesOwnCode pins the contract the E2E
// suite depends on. apps/web/e2e/admin-facility-mutations.spec.ts generates its
// short_code as "E2E" + a six-digit run tag, so "E2E123456" must be accepted.
//
// This is the regression that a hand-derived digit cap caused: an earlier
// revision capped total digits at 2, which rejected this code and turned the
// facility-creation spec's expected 404 into a 400. The code is genuinely safe
// — its rendered queue number "E2E123456-0300" is 11 characters, and the
// predicate's second conjunct needs 12 — so rejecting it was a false rejection
// that narrowed the product.
func TestValidateShortCode_AcceptsTheE2ESuitesOwnCode(t *testing.T) {
	for _, code := range []string{"E2E123456", "E2E000001", "E2E999999"} {
		if err := validateShortCode(code); err != nil {
			t.Errorf("validateShortCode(%q) = %v, want nil: the E2E suite generates codes of this shape", code, err)
		}
	}
}

func TestValidateShortCode_RejectsUnsafeValues(t *testing.T) {
	cases := []struct {
		code string
		why  string
	}{
		{"", "empty"},
		{"123", "must start with a letter"},
		{"12-34-5", "must start with a letter"},
		{"ABCDEFGHIJK", "longer than 10 characters"},
		{"RS K", "space is not permitted"},
		{"RS.K", "dot is not permitted"},
		{"RS/K", "slash is not permitted"},
		{"RS_K", "underscore is not permitted"},

		// The genuinely unsafe shapes: these render a queue number the
		// notification_outbox phone constraint rejects.
		{"AB12345678", "8 consecutive digits in the rendered queue number"},
		{"A1234567", "7 trailing digits: rendered span is 7+1+4 = 12, which the second conjunct rejects"},
		{"ABCDEF1234567", "same, with a longer letter prefix"},
	}
	for _, c := range cases {
		if err := validateShortCode(c.code); err == nil {
			t.Errorf("validateShortCode(%q) = nil, want an error (%s)", c.code, c.why)
		}
	}
}

// TestShortCode_RenderedQueueNumberIsAlwaysSafe is the load-bearing test. It
// proves the invariant directly: for every short_code the validator ACCEPTS,
// the queue number the Rust engine renders ("{short_code}-{NNNN}", counter
// capped at the 300 daily limit) does not match the notification_outbox phone
// predicate; and for every short_code it REJECTS on safety grounds, the
// rendered queue number DOES match.
//
// The predicate is not re-implemented here. It is notification's
// ContainsRawPhoneDigits, which is the same function the validator calls and
// the Go expression of the database's CHECK constraint. A local mirror would be
// a second implementation of a security predicate — the exact divergence this
// change exists to remove.
// renderQueueNumber mirrors the queue engine's ONLY render site,
// apps/queue-engine/src/engine/queue.rs:
//
//	format!("{}-{:04}", short_code, next_number)
//
// It uses the same Go format verb (%04d) rather than a hand-rolled padder, so
// the width is expressed once. The format itself is pinned on the Rust side by
// queue_number_format_matches_the_notification_contract in that crate — a change
// to {:04} or to DAILY_QUEUE_LIMIT fails there, not silently here.
func renderQueueNumber(shortCode string, counter int) string {
	return fmt.Sprintf("%s-%04d", shortCode, counter)
}

// TestShortCode_QueueNumberFormatMatchesTheEngine pins the coupling between this
// package's mirror of the queue-number format and the Rust source that actually
// produces it.
//
// The Go tests reconstruct the queue number with renderQueueNumber, which is a
// SECOND copy of a format that lives in apps/queue-engine/src/engine/queue.rs.
// A change there — the {:04} width, the "-" separator, or DAILY_QUEUE_LIMIT —
// would leave every Go test green while invalidating the safety argument, since
// validateShortCode's witness ("-0300") and the boundary tests all assume four
// digits. This test reads the Rust file and fails if those change, which forces
// the Go side to be updated in the same commit.
//
// It is a static pin, not a re-implementation: it asserts the exact format
// string and constant are present, so it can only fail for the right reason.
func TestShortCode_QueueNumberFormatMatchesTheEngine(t *testing.T) {
	// apps/api/internal/handler -> repo root is four levels up.
	const rustPath = "../../../../apps/queue-engine/src/engine/queue.rs"
	src, err := os.ReadFile(rustPath)
	if err != nil {
		t.Fatalf("read the queue engine source (%s): %v", rustPath, err)
	}
	text := string(src)

	for _, want := range []string{
		`format!("{}-{:04}", short_code, next_number)`,
		"const DAILY_QUEUE_LIMIT: i32 = 300;",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("queue.rs no longer contains %q — update renderQueueNumber, "+
				"queueNumberWorstCase and the boundary fixtures in this file to match", want)
		}
	}
}

func TestShortCode_RenderedQueueNumberIsAlwaysSafe(t *testing.T) {
	// Counters spanning the whole permitted range, including the widest.
	counters := []int{1, 9, 10, 99, 100, 300}

	// Accepted codes, including the boundary case: 6 trailing digits is the
	// most that can precede the 4-digit counter and still stay under the
	// predicate's 12-character span.
	accepted := []string{
		"RSK", "PKM", "f1", "DEMO-GIGI", "DEMO-UMUM",
		"A12", "A1-2", "A-12", "A1-2-", "A--12", "AB12", "A12-B",
		"E2E123456", "E2E000001",
		"AB-123456", "A123456",
	}
	for _, sc := range accepted {
		if err := validateShortCode(sc); err != nil {
			t.Fatalf("test fixture %q is rejected by the validator: %v", sc, err)
		}
		for _, n := range counters {
			rendered := renderQueueNumber(sc, n)
			if notification.ContainsRawPhoneDigits(rendered) {
				t.Errorf("accepted short_code %q with counter %d renders %q, which the phone constraint would reject",
					sc, n, rendered)
			}
		}
	}

	// One digit more must flip the verdict, or the boundary above is not the
	// real boundary and this test is proving nothing. Each string is put through
	// the VALIDATOR first — asserting only the predicate's verdict on a string
	// this test built itself would prove nothing about validateShortCode, and a
	// no-op safety check would satisfy it.
	unsafe := []string{"A1234567", "E2E1234567", "AB12345678", "ABCDEF1234567"}
	for _, sc := range unsafe {
		if err := validateShortCode(sc); err == nil {
			t.Errorf("unsafe short_code %q was ACCEPTED by the validator; the safety check is not in force", sc)
		}
		for _, n := range counters {
			rendered := renderQueueNumber(sc, n)
			if !notification.ContainsRawPhoneDigits(rendered) {
				t.Errorf("unsafe short_code %q with counter %d renders %q, which the predicate does NOT reject; the boundary moved",
					sc, n, rendered)
			}
		}
	}
}

// TestValidateShortCode_IsWiredIntoTheHandlers drives the ACTUAL call sites.
//
// Testing the helper alone does not prove the fix: the defect was that
// facilities.short_code reached the database unvalidated, and the repair is the
// wiring in validateCreateFacility and validateUpdateFacility. A mutation that
// removed just those two calls would leave a helper-only test green.
func TestValidateShortCode_IsWiredIntoTheHandlers(t *testing.T) {
	base := func(code string) CreateFacilityRequest {
		return CreateFacilityRequest{
			Name:          "Klinik Uji",
			Type:          "puskesmas",
			Address:       "Jl. Uji No. 1",
			Kecamatan:     "Cibinong",
			KabupatenKota: "Bogor",
			Provinsi:      "Jawa Barat",
			Phone:         "08123456789",
			TotalBeds:     10,
			AvailableBeds: 5,
			ShortCode:     code,
		}
	}

	// Create: an unsafe short_code must be refused BY THE CALLER.
	for _, code := range []string{"AB12345678", "A1234567", "E2E1234567"} {
		if err := validateCreateFacility(base(code)); err == nil {
			t.Errorf("validateCreateFacility accepted unsafe short_code %q; the validator is not wired into create", code)
		}
	}
	// Create: the boundary-accepted set must pass, so the wiring is not merely
	// refusing everything.
	for _, code := range []string{"A123456", "E2E123456", "RSK"} {
		if err := validateCreateFacility(base(code)); err != nil {
			t.Errorf("validateCreateFacility rejected safe short_code %q: %v", code, err)
		}
	}

	// Update: same, through the pointer field.
	for _, code := range []string{"AB12345678", "A1234567", "E2E1234567"} {
		c := code
		if err := validateUpdateFacility(UpdateFacilityRequest{ShortCode: &c}); err == nil {
			t.Errorf("validateUpdateFacility accepted unsafe short_code %q; the validator is not wired into update", code)
		}
	}
	for _, code := range []string{"A123456", "E2E123456", "RSK"} {
		c := code
		if err := validateUpdateFacility(UpdateFacilityRequest{ShortCode: &c}); err != nil {
			t.Errorf("validateUpdateFacility rejected safe short_code %q: %v", code, err)
		}
	}
	// Update: omitting ShortCode must stay valid — a PATCH that does not touch
	// the code must not be forced to supply one.
	if err := validateUpdateFacility(UpdateFacilityRequest{}); err != nil {
		t.Errorf("validateUpdateFacility rejected a request with no ShortCode: %v", err)
	}
}

// TestShortCode_QueueNumberWorstCaseMatchesEveryCounter guards the constant the
// validator uses as its witness. validateShortCode tests "code-0300"; if a real
// counter could render wider or differently, that witness would stop covering
// the range it claims to.
func TestShortCode_QueueNumberWorstCaseMatchesEveryCounter(t *testing.T) {
	// DAILY_QUEUE_LIMIT in apps/queue-engine/src/engine/queue.rs.
	const dailyQueueLimit = 300
	for n := 1; n <= dailyQueueLimit; n++ {
		got := fmt.Sprintf("-%04d", n)
		if len(got) != len(queueNumberWorstCase) {
			t.Fatalf("counter %d renders %q (%d chars), but the witness %q is %d chars",
				n, got, len(got), queueNumberWorstCase, len(queueNumberWorstCase))
		}
	}
	// The witness itself must be one of the renderable counters, so it is a
	// real value and not an invented one.
	if queueNumberWorstCase != fmt.Sprintf("-%04d", dailyQueueLimit) {
		t.Fatalf("queueNumberWorstCase = %q, want %q", queueNumberWorstCase, fmt.Sprintf("-%04d", dailyQueueLimit))
	}
}
