package notification

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestMaskPhone_KnownFormats(t *testing.T) {
	cases := []struct {
		in   string
		want string // exact equality
	}{
		{"+6281234567890", "+62••••7890"},
		{"081234567890", "+62••••7890"},
		{"+62-812-3456-7890", "+62••••7890"},
		{"6281234567890", "••••••••7890"}, // no + or leading 0; falls back to generic mask
	}
	for _, tc := range cases {
		got := MaskPhone(tc.in)
		if got != tc.want {
			t.Errorf("MaskPhone(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// Privacy invariant: the bulk of the digits must NOT appear
		// in the masked output. We check that the original digit
		// substring is not present anywhere in the masked form.
		digits := onlyDigits(tc.in)
		if len(digits) > 4 {
			bulk := digits[:len(digits)-4]
			if strings.Contains(got, bulk) {
				t.Errorf("MaskPhone(%q) leaked bulk digits in %q", tc.in, got)
			}
		}
	}
}

func TestMaskPhone_EmptyOrJunk(t *testing.T) {
	for _, in := range []string{"", " ", "abc", "----"} {
		got := MaskPhone(in)
		if strings.Contains(got, onlyDigits(in)) && onlyDigits(in) != "" {
			t.Errorf("MaskPhone(%q) leaked digits in %q", in, got)
		}
	}
}

func TestMaskEmail_KnownFormats(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"budi@example.com", "b•••@example.com"},
		{"alicia.wijaya@puskesmas-sukajaya.go.id", "a•••@puskesmas-sukajaya.go.id"},
	}
	for _, tc := range cases {
		got := MaskEmail(tc.in)
		if got != tc.want {
			t.Errorf("MaskEmail(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// Privacy invariant: the local part after the first character
		// must NOT appear in the masked output.
		at := strings.LastIndex(tc.in, "@")
		if at > 1 {
			localRest := tc.in[1:at]
			if strings.Contains(got, localRest) {
				t.Errorf("MaskEmail(%q) leaked local part in %q", tc.in, got)
			}
		}
	}
}

func TestMaskEmail_Malformed(t *testing.T) {
	for _, in := range []string{"", "@example.com", "no-at-sign", "alice@", "  "} {
		got := MaskEmail(in)
		if strings.Contains(got, "@") && got == in {
			t.Errorf("MaskEmail(%q) returned input unchanged: %q", in, got)
		}
	}
}

func TestHashContact_Deterministic(t *testing.T) {
	a := HashContact("+6281234567890")
	b := HashContact("+6281234567890")
	if hex.EncodeToString(a) != hex.EncodeToString(b) {
		t.Errorf("HashContact is not deterministic: %x vs %x", a, b)
	}
	if len(a) != sha256.Size {
		t.Errorf("HashContact returned %d bytes, want %d", len(a), sha256.Size)
	}
}

func TestHashContact_NormalisesEquivalents(t *testing.T) {
	// All of these should hash to the same value because they refer
	// to the same Indonesian phone number.
	raw := []string{
		"+6281234567890",
		"081234567890",
		"+62-812-3456-7890",
		"  +62 812 3456 7890  ",
	}
	first := hex.EncodeToString(HashContact(raw[0]))
	for _, r := range raw[1:] {
		got := hex.EncodeToString(HashContact(r))
		if got != first {
			t.Errorf("HashContact(%q) = %s, want %s (canonical)", r, got, first)
		}
	}
}

func TestHashContact_EmailIsLowercased(t *testing.T) {
	a := hex.EncodeToString(HashContact("Alice@example.com"))
	b := hex.EncodeToString(HashContact("alice@example.com"))
	if a != b {
		t.Errorf("HashContact does not normalise email case: %s vs %s", a, b)
	}
}

func TestHashContact_DistinctInputsDiffer(t *testing.T) {
	a := HashContact("+6281234567890")
	b := HashContact("+6281234567891")
	if hex.EncodeToString(a) == hex.EncodeToString(b) {
		t.Errorf("HashContact collision for distinct inputs")
	}
}
