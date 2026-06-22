package notification

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderTemplate_HappyPath(t *testing.T) {
	got, err := RenderTemplate(
		"Janji temu di {facility_name} pada {appointment_time}.",
		map[string]string{
			"facility_name":    "RSUD Kota Sehat",
			"appointment_time": "09:00",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Janji temu di RSUD Kota Sehat pada 09:00."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderTemplate_RepeatedPlaceholder(t *testing.T) {
	got, err := RenderTemplate(
		"{facility_name} - {facility_name}",
		map[string]string{"facility_name": "Puskesmas Melati"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Puskesmas Melati - Puskesmas Melati"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderTemplate_NoPlaceholders(t *testing.T) {
	// Pure text, no {name} pattern. Should pass through unchanged
	// (after the digit-denial check, which it passes because the
	// string contains no 8+ consecutive digits).
	got, err := RenderTemplate(
		"Janji temu Anda berhasil dicatat.",
		map[string]string{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Janji temu Anda berhasil dicatat." {
		t.Errorf("got %q", got)
	}
}

func TestRenderTemplate_EmptyTemplate(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n\r"} {
		_, err := RenderTemplate(in, map[string]string{})
		if !errors.Is(err, ErrEmptyTemplate) {
			t.Errorf("RenderTemplate(%q): got %v, want ErrEmptyTemplate", in, err)
		}
	}
}

func TestRenderTemplate_MissingPlaceholder(t *testing.T) {
	_, err := RenderTemplate(
		"Janji temu di {facility_name} pada {appointment_time}.",
		map[string]string{"facility_name": "RSUD Kota Sehat"}, // appointment_time missing
	)
	if !errors.Is(err, ErrMissingPlaceholder) {
		t.Errorf("got %v, want ErrMissingPlaceholder", err)
	}
}

func TestRenderTemplate_UnsafeVariableRejectedBeforeSubstitution(t *testing.T) {
	// `raw_phone` is not on the allow-list. The renderer must reject
	// it BEFORE any substitution, so the rendered string is "".
	got, err := RenderTemplate(
		"Telepon: {raw_phone}",
		map[string]string{"raw_phone": "+6281234567890"},
	)
	if !errors.Is(err, ErrUnsafeVariable) {
		t.Errorf("got %v, want ErrUnsafeVariable", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string (refuse to render anything on unsafe-variable error)", got)
	}
}

func TestRenderTemplate_AllowListEnforced(t *testing.T) {
	// The allow-list is closed. Any name outside it is rejected.
	disallowed := []string{
		"phone", "email", "patient_phone", "raw", "raw_phone",
		"otp", "secret", "api_key", "url", "link",
	}
	for _, name := range disallowed {
		_, err := RenderTemplate(
			"val="+"{x}", map[string]string{name: "anything"},
		)
		// Use a template placeholder named "x" but supply an
		// unsafe var; the var allow-list check fires first.
		if !errors.Is(err, ErrUnsafeVariable) {
			t.Errorf("var %q should be unsafe; got %v", name, err)
		}
	}
}

func TestRenderTemplate_RawDigitDenialOnRenderedOutput(t *testing.T) {
	// Allowed variable name; its value contains 8+ consecutive digits.
	// Renderer must reject because the rendered string would expose
	// raw digits, even though the variable name itself is allowed.
	_, err := RenderTemplate(
		"Janji temu {appointment_time}.",
		map[string]string{"appointment_time": "2026-06-22T09:00:00Z12345678"},
	)
	if !errors.Is(err, ErrRenderedOutputContainsRawDigits) {
		t.Errorf("got %v, want ErrRenderedOutputContainsRawDigits", err)
	}
}

func TestRenderTemplate_AllowedNameButSafeValue(t *testing.T) {
	// `checkin_code` is on the allow-list; a typical value is 6 chars
	// alphanumeric (no 8+ consecutive digits). Should pass.
	got, err := RenderTemplate(
		"Kode check-in: {checkin_code}.",
		map[string]string{"checkin_code": "AB12CD"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "AB12CD") {
		t.Errorf("got %q, expected to contain AB12CD", got)
	}
}

func TestRenderTemplate_NoHTMLExecution(t *testing.T) {
	// <script> tags and other HTML-looking tokens are treated as plain
	// text. No parsing, no execution.
	got, err := RenderTemplate(
		"<script>alert('xss')</script> at {facility_name}",
		map[string]string{"facility_name": "Puskesmas"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "<script>") {
		t.Errorf("expected raw HTML passthrough, got %q", got)
	}
	if !strings.HasSuffix(got, "Puskesmas") {
		t.Errorf("expected placeholder substituted, got %q", got)
	}
}

func TestRenderTemplate_PlaceholderOutsideAllowListSyntaxIsLiteral(t *testing.T) {
	// The regex requires `[a-z_]{1,64}`. Names with capitals,
	// digits, or other characters are NOT recognised as placeholders
	// and are passed through verbatim.
	got, err := RenderTemplate(
		"Visit {WWW.example.com} or {123} for {facility_name}",
		map[string]string{"facility_name": "Puskesmas"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// {WWW.example.com} and {123} are not captured by the regex
	// (they contain characters outside [a-z_]) so they pass through
	// as literal braces. {facility_name} IS substituted.
	if !strings.Contains(got, "{WWW.example.com}") || !strings.Contains(got, "{123}") {
		t.Errorf("non-matching braces should be literal; got %q", got)
	}
	if !strings.Contains(got, "Puskesmas") {
		t.Errorf("expected placeholder substituted; got %q", got)
	}
}

func TestRenderTemplate_LongVarNameIgnored(t *testing.T) {
	// A placeholder name longer than 64 chars is not captured by
	// the regex and is treated as literal.
	long := strings.Repeat("a", 65)
	got, err := RenderTemplate("{"+long+"}", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "{"+long+"}" {
		t.Errorf("expected literal long-name placeholder, got %q", got)
	}
}

func TestRenderTemplate_AllowedNamesIsStable(t *testing.T) {
	// Sanity: the allow-list exposes a non-empty, sorted list. If the
	// list ever drifts, this test catches it.
	got := AllowedNames()
	if len(got) == 0 {
		t.Fatalf("AllowedNames() returned empty list")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("AllowedNames() not sorted at index %d: %v", i, got)
		}
	}
}