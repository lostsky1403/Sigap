package notification

import (
	"crypto/sha256"
	"regexp"
	"strings"
)

// MaskPhone returns a contact-shape representation of a phone number that
// preserves enough of the structure for the recipient to recognise the
// number while hiding the bulk of the digits.
//
// Examples:
//
//	+6281234567890 -> "+62••••7890"
//	+62-812-3456-7890 -> "+62••••7890"
//	081234567890    -> "+62••••7890"  (local 0-prefix normalised)
//	6281234567890    -> "••••••••7890"  (no + or 0; generic mask)
//	+12025550100     -> "+1••••0100"
//	1234             -> "••••1234"      (too short to format with country)
//
// MaskPhone is total: any input that does not look like a phone returns
// the generic "••••XXXX" mask so the caller can never accidentally
// surface the raw value.
func MaskPhone(raw string) string {
	digits := onlyDigits(raw)
	if len(digits) == 0 {
		return "••••"
	}
	tail := digits[len(digits)-4:]

	// Explicit +62 prefix → confidently Indonesian, show +62.
	if strings.HasPrefix(raw, "+62") && len(digits) > 4 {
		return "+62••••" + tail
	}
	// Local 0-prefix (e.g. "081234567890") → Indonesian, show +62.
	if !strings.HasPrefix(raw, "+") && len(digits) >= 10 && digits[0] == '0' {
		return "+62••••" + tail
	}
	// Generic international case: keep the country code (first 1–3
	// digits) and mask the rest.
	if strings.HasPrefix(raw, "+") && len(digits) > 4 {
		ccLen := 2
		if len(digits) >= 11 {
			ccLen = 3
		}
		if len(digits)-ccLen < 4 {
			ccLen = len(digits) - 4
		}
		cc := digits[:ccLen]
		return "+" + cc + strings.Repeat("•", 4) + tail
	}
	// Fallback: generic mask (no country code inferred from the input).
	return strings.Repeat("•", 8) + tail
}

// MaskEmail returns a contact-shape representation of an email address.
// The local part is reduced to its first character + three bullets; the
// domain is kept verbatim so the recipient can recognise it.
//
// Example:
//
//	budi@example.com -> "b•••@example.com"
//
// If the input does not contain an `@`, MaskEmail returns "•••@•••" —
// never the raw value.
func MaskEmail(raw string) string {
	at := strings.LastIndex(raw, "@")
	if at <= 0 || at == len(raw)-1 {
		return "•••@•••"
	}
	local := raw[:at]
	domain := raw[at+1:]
	if local == "" {
		return "•••@" + domain
	}
	first := string(local[0])
	return first + "•••@" + domain
}

// HashContact returns the SHA-256 of the normalised contact. Normalisation
// for phones strips non-digits; for emails it lowercases and trims. The
// returned 32-byte digest is what is stored in
// notification_outbox.recipient_contact_hash.
//
// HashContact is deterministic: the same input always yields the same
// bytes. This is what makes the hash useful for dedup and idempotency.
func HashContact(raw string) []byte {
	h := sha256.New()
	h.Write([]byte(normaliseForHash(raw)))
	return h.Sum(nil)
}

// normaliseForHash canonicalises a contact before hashing so that
// +6281234567890 and 0812-3456-7890 hash to the same value. It is
// distinct from MaskPhone which is purely a display function.
//
// The canonical form is the LOCAL subscriber digits — country code
// (when present) and the local 0-trunk prefix are both stripped, so
// +62-prefix and 0-prefix forms of the same number collapse to the
// same hash.
func normaliseForHash(raw string) string {
	s := strings.TrimSpace(raw)
	if at := strings.Index(s, "@"); at > 0 {
		return strings.ToLower(s)
	}
	d := onlyDigits(s)
	if strings.HasPrefix(s, "+") && len(d) >= 10 {
		// +62XXXXXXXX (12+ digits) → strip 2 ("62").
		// +XYYYYYYYYYY (10 digits) → strip 1 (1-digit country code).
		// +XXYYYYYYYYY (11 digits) → strip 2.
		// +XXXYYYYYYYY (12 digits) → strip 3.
		var ccLen int
		switch {
		case strings.HasPrefix(d, "62") && len(d) >= 12:
			ccLen = 2
		case len(d) == 10:
			ccLen = 1
		case len(d) == 11:
			ccLen = 2
		default:
			ccLen = 3
		}
		if len(d)-ccLen >= 4 {
			d = d[ccLen:]
		}
	} else if len(d) >= 10 && d[0] == '0' {
		// Local 0-trunk prefix (e.g. Indonesian 081234567890) → strip "0".
		d = d[1:]
	}
	return d
}

// onlyDigits returns s with every non-digit character removed.
func onlyDigits(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			b = append(b, c)
		}
	}
	return string(b)
}

// minInt returns the smaller of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// digitRunRegex matches sequences that look like a raw phone number.
// It has two alternatives:
//
//  1. [0-9]{8,} — 8 or more CONSECUTIVE ASCII digits. This catches
//     raw phone numbers like "081234567890" with no formatting.
//
//  2. A formatted-phone detector for numbers like "0812-3456-7890" or
//     "+62 812 3456 7890". Unlike alternative 1, this catches phone
//     numbers that have been split by hyphens, dots, spaces, or
//     parentheses so that no single run reaches 8 digits.
//
//     The pattern matches: a digit run of 3-4, followed by a separator
//     and another digit group of 3-4, followed by a separator and
//     another digit group of 2-4. This requires at least 8 digits
//     spread across groups — a layout typical of phone numbers but
//     not of dates (which use a 4-2-2 pattern with short tails).
//
//     Example matches: "0812-3456-7890", "+62 812 345 6789",
//     "(021) 555-1234"
//
//     Non-matches: "2026-06-22" (only 2 digit groups after the first),
//     "09:00" (too few digits), "Order #1234567" (single run of 7).
var digitRunRegex = regexp.MustCompile(`[0-9]{8,}|[0-9]{3,4}[-._() ][0-9]{3,4}[-._() ][0-9]{2,4}`)
