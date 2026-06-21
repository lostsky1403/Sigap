package notification

// ContainsRawPhoneDigits reports whether s contains a sequence of 8 or
// more ASCII digits. This is the denylist predicate used by the service
// to reject subjects and body templates that look like they contain a
// raw phone number. The same predicate is also expressed as a CHECK
// constraint in packages/db/migrations/0006_notifications.sql for
// defence in depth.
//
// Why 8+ digits? Phone numbers in our scope are at least 10 digits;
// formatting separators (spaces, dashes, dots) are stripped by callers
// before reaching the service. Catching 8+ consecutive digits catches
// every plausible phone (and also catches long ID-like sequences, which
// is fine because outbox subjects are short user-facing messages, not
// document IDs).
func ContainsRawPhoneDigits(s string) bool {
	return digitRunRegex.MatchString(s)
}
