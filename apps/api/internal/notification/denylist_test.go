package notification

import (
	"regexp"
	"testing"
)

// sqlPredicate is a HAND TRANSCRIPTION of the database's semantic predicate,
// from packages/db/migrations/0006_notifications.sql and converged by
// 0011_notification_outbox_phone_constraints.sql:
//
//	CHECK (col !~ '[0-9]{8,}' AND col !~ '[0-9][0-9\-._() ]{10,}[0-9]')
//
// A column value violates the constraint when EITHER conjunct matches, which is
// what this regex expresses.
//
// BE HONEST ABOUT WHAT THIS PROVES. This is a Go regexp compared against the
// production Go regexp, so the test below can only detect the two GO copies
// drifting apart — it does NOT execute SQL and cannot detect a divergence
// between Go and PostgreSQL (RE2 vs ARE, or a standard_conforming_strings
// mangling). The real Go↔DB check is
// TestPredicate_CorpusAgreesWithTheDatabase in enqueue_db_test.go, which drives
// this same corpus through a live constraint. Keep the two in step: both read
// predicateCorpus.
var sqlPredicate = regexp.MustCompile(`[0-9]{8,}|[0-9][0-9\-._() ]{10,}[0-9]`)

// predicateCorpus is the shared value set for predicate testing. It is used by
// the in-process transcription check below AND by the live-database check in
// enqueue_db_test.go, so the two can never drift into testing different inputs.
var predicateCorpus = []string{
	// --- unformatted phone numbers ---
	"081234567890",
	"6281234567890",
	"Tel: 12345678",
	"Nomor telp: 123456789012",
	"Konfirmasi +6281234567890",
	// --- formatted Indonesian phone numbers ---
	"0812-3456-7890",
	"0812 3456 7890",
	"0812.3456.7890",
	"0812_3456_7890",
	"(021) 555-1234",
	"Hubungi 0812-3456-7890 ya",
	// --- international formats ---
	"+62 812 3456 7890",
	"+62-812-3456-7890",
	"62812-3456-7890",
	"1234567-8901",
	// --- legitimate content that must be ACCEPTED ---
	"",
	"Konfirmasi Janji Temu Sigap",
	"Kode check-in: AB12CD.",
	"Nomor antrean: FSK-0001.",
	"Nomor antrian Anda: 0042",
	"Fasilitas: RSUD Kota Sehat",
	"Order #1234567",
	"Code: 1234",
	"12-34-56",
	"Floors 1-2-3-4-5",
	"Rp 1.250.000 dibayarkan.",
	"Kunjungan pada 2026-10-09.",
	"Janji temu pada 9 Oktober 2026 pukul 14.30.",
	"Janji temu pada 9 Oktober 2026, pukul 14.30 WIB.",
	// --- accepted by neither layer (bare ISO date adjacent to a time) ---
	"Pada 2026-06-22 09:00",
	"Janji temu pada 2026-10-09 14:30.",
	"Janji temu pada 2026-10-09 14:30:00.",
	// --- boundary: the digit-run conjunct, at and around 8 digits ---
	"1234567",   // 7 digits: accepted
	"12345678",  // 8 digits: rejected by conjunct 1
	"123456789", // 9 digits: rejected
}

// TestPredicate_GoTranscriptionMatchesGoPredicate is a DRIFT CHECK between the
// production Go predicate and the hand-transcribed SQL predicate above. It runs
// no SQL. The live Go↔DB check is
// TestPredicate_CorpusAgreesWithTheDatabase in enqueue_db_test.go.
func TestPredicate_GoTranscriptionMatchesGoPredicate(t *testing.T) {
	for _, s := range predicateCorpus {
		goVerdict := ContainsRawPhoneDigits(s)
		sqlVerdict := sqlPredicate.MatchString(s)
		if goVerdict != sqlVerdict {
			t.Errorf("Go/SQL transcription divergence on %q: Go=%v transcribed-SQL=%v", s, goVerdict, sqlVerdict)
		}
	}
}

func TestContainsRawPhoneDigits_AcceptsSafeText(t *testing.T) {
	safe := []string{
		"",
		"Konfirmasi Janji Temu Sigap",
		"Kode check-in: ABC123",
		"Nomor antrian Anda: 0042",
		"Fasilitas: RSUD Kota Sehat",
		"Janji temu Anda berhasil dicatat.", // has "0"+"6"+"9" — but not 8 consecutive
		"Nomor antrean: FSK-0001.",
		"Order #1234567",
		"Code: 1234",
		"12-34-56",
		"Floors 1-2-3-4-5",
		// Localized date+time: the supported way to render a timestamp in a
		// notification body. See digitRunRegex in masking.go.
		"Janji temu pada 9 Oktober 2026 pukul 14.30.",
	}
	for _, s := range safe {
		if ContainsRawPhoneDigits(s) {
			t.Errorf("ContainsRawPhoneDigits(%q) returned true for safe text", s)
		}
	}
}

func TestContainsRawPhoneDigits_RejectsPhoneLike(t *testing.T) {
	unsafe := []string{
		"Konfirmasi +6281234567890",
		"Hubungi 081234567890 untuk info",
		"Phone: 6281234567890",
		"Tel: 12345678",            // exactly 8 digits — should be caught
		"Nomor telp: 123456789012", // 12 digits
		// Formatted numbers with no 8-digit run. These are caught by the second
		// alternative, and MUST be caught here too, because the database
		// rejects them.
		"0812-3456-7890",
		"+62 812 3456 7890",
		"(021) 555-1234",
		"62812-3456-7890",
	}
	for _, s := range unsafe {
		if !ContainsRawPhoneDigits(s) {
			t.Errorf("ContainsRawPhoneDigits(%q) returned false for phone-like text", s)
		}
	}
}

// TestContainsRawPhoneDigits_RejectsBareISODateTime pins the deliberate
// behaviour change: a bare ISO date adjacent to a time is rejected, because the
// database CHECK constraint rejects it. Rejecting it here turns a lost
// fire-and-forget notification into a named error at the service boundary.
func TestContainsRawPhoneDigits_RejectsBareISODateTime(t *testing.T) {
	rejected := []string{
		"Pada 2026-06-22 09:00",
		"Janji temu pada 2026-10-09 14:30.",
		"Janji temu pada 2026-10-09 14:30:00.",
	}
	for _, s := range rejected {
		if !ContainsRawPhoneDigits(s) {
			t.Errorf("ContainsRawPhoneDigits(%q) = false, want true (DB rejects this value)", s)
		}
	}
}

func TestContainsRawPhoneDigits_AllowsShortDigitRuns(t *testing.T) {
	// Short numeric IDs and ordinary prose with numbers must not trigger.
	short := []string{
		"Order #1234567",   // 7 digits
		"Code: 1234",       // 4 digits
		"12-34-56",         // 6 digits with separators
		"Floors 1-2-3-4-5", // 5 digits with separators
		"Kunjungan pada 2026-10-09.",
		"Rp 1.250.000 dibayarkan.",
	}
	for _, s := range short {
		if ContainsRawPhoneDigits(s) {
			t.Errorf("ContainsRawPhoneDigits(%q) returned true for short digit run", s)
		}
	}
}
