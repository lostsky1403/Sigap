package notification

// ContainsRawPhoneDigits reports whether s contains a sequence that looks
// like a raw phone number.
//
// This is the Go expression of the SAME semantic predicate enforced by the
// notification_outbox CHECK constraints in packages/db/migrations/
// 0006_notifications.sql (converged forward by 0011). The database predicate is
// authoritative; see digitRunRegex in masking.go for the full contract and the
// reason the two must agree.
//
// Why 8+ consecutive digits, and why the second alternative exists? Phone
// numbers in our scope are at least 10 digits, and callers strip formatting
// separators before the value reaches the service in most cases. The first
// alternative catches any plausible unformatted phone (and also long ID-like
// sequences, which is acceptable because outbox subjects and bodies are short
// user-facing messages, not document IDs). The second catches numbers that were
// formatted with separators so that no single run reaches 8 digits.
//
// The second alternative also rejects a bare ISO date adjacent to a time
// ("2026-06-22 09:00"). That is intentional and matches the database: such a
// body would be rejected by the CHECK constraint anyway, so it is rejected
// here first, with a named error, instead of failing the insert after the
// fact. Render dates in the localized form instead.
func ContainsRawPhoneDigits(s string) bool {
	return digitRunRegex.MatchString(s)
}
