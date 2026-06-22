package notification

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
)

// Domain errors returned by RenderTemplate. They are returned ONLY after
// no rendered output is produced — callers can safely log the error and
// never display the empty string.
var (
	// ErrMissingPlaceholder is returned when the template references a
	// {name} placeholder whose name is present in the template but
	// absent from vars. Callers MUST NOT silently substitute empty.
	ErrMissingPlaceholder = errors.New("notification: template placeholder missing from vars")

	// ErrUnsafeVariable is returned when vars contains a key whose
	// name is NOT in the closed allow-list. This is rejected BEFORE
	// any substitution so that arbitrary caller keys (e.g.
	// vars["raw_phone"]) cannot be interpolated.
	ErrUnsafeVariable = errors.New("notification: template variable name is not in the allow-list")

	// ErrRenderedOutputContainsRawDigits is returned when the rendered
	// output matches the existing ContainsRawPhoneDigits denylist
	// (8+ consecutive ASCII digits). This is defence-in-depth on top
	// of the database CHECK constraints enforced by migration
	// 0006_notifications.sql.
	ErrRenderedOutputContainsRawDigits = errors.New("notification: rendered output contains raw-phone-like digits")

	// ErrEmptyTemplate is returned when the template argument is empty
	// or whitespace-only. Refusing an empty template is a safety
	// guard; callers should never enqueue an empty body.
	ErrEmptyTemplate = errors.New("notification: template is empty")
)

// allowedTemplateVars is the closed allow-list of variable names that
// RenderTemplate will accept. Any name in `vars` that is not on this list
// causes ErrUnsafeVariable BEFORE substitution. Any name referenced in
// the template but absent from vars causes ErrMissingPlaceholder.
//
// The allow-list is intentionally closed: future additions go through
// a review pass and a corresponding test in renderer_test.go.
var allowedTemplateVars = map[string]struct{}{
	"appointment_code": {},
	"appointment_time": {},
	"checkin_code":     {},
	"checked_in_at":    {},
	"facility_name":    {},
	"patient_name":     {},
	"queue_number":     {},
	"template_key":     {},
}

// placeholderRegex captures {name} where name is a snake-case ASCII
// identifier. Names longer than 64 chars or with characters outside
// [a-z_] are rejected by the regex itself (no allocation, no
// substitution).
var placeholderRegex = regexp.MustCompile(`\{([a-z_]{1,64})\}`)

// AllowedNames returns a sorted slice of the allowed variable names.
// Used by renderer_test.go to keep the test in sync with the
// allow-list and for documentation strings.
func AllowedNames() []string {
	out := make([]string, 0, len(allowedTemplateVars))
	for k := range allowedTemplateVars {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RenderTemplate substitutes every {name} placeholder in tpl with
// vars[name] and returns the rendered string. It is a pure function:
// no I/O, no clock, no randomness.
//
// Validation rules (all enforced before substitution except the
// digit-denial rule, which is enforced on the rendered output):
//
//   1. tpl must be non-empty (whitespace-only counts as empty).
//   2. Every {name} placeholder must reference a name that exists
//      in vars. Missing → ErrMissingPlaceholder.
//   3. Every key in vars must be on the closed allow-list
//      (see AllowedNames()). Not on the list → ErrUnsafeVariable.
//   4. The rendered string must not match ContainsRawPhoneDigits.
//      Matches → ErrRenderedOutputContainsRawDigits.
//
// On any error, the returned string is "" — never the partial output.
// Callers can safely surface the error and MUST NOT log the empty
// string as if it were content.
func RenderTemplate(tpl string, vars map[string]string) (string, error) {
	if tpl == "" || len(tpl) > 0 && allWhitespace(tpl) {
		return "", ErrEmptyTemplate
	}

	// Pre-flight: every key in vars must be on the allow-list.
	// This runs BEFORE substitution so that callers cannot smuggle
	// arbitrary keys (e.g. "raw_phone") into the rendered output.
	for k := range vars {
		if _, ok := allowedTemplateVars[k]; !ok {
			return "", fmt.Errorf("%w: %q", ErrUnsafeVariable, k)
		}
	}

	// Pre-flight: every placeholder in tpl must be present in vars.
	// We collect placeholders in a map so duplicates do not trigger
	// duplicate errors.
	placeholders := map[string]struct{}{}
	for _, m := range placeholderRegex.FindAllStringSubmatch(tpl, -1) {
		placeholders[m[1]] = struct{}{}
	}
	for name := range placeholders {
		if _, ok := vars[name]; !ok {
			return "", fmt.Errorf("%w: %q", ErrMissingPlaceholder, name)
		}
	}

	// Substitute. The substitution is purely lexical — `{name}` is
	// replaced by `vars[name]`. We do not parse HTML, do not run
	// script, do not URL-decode, do not transform case.
	rendered := placeholderRegex.ReplaceAllStringFunc(tpl, func(match string) string {
		name := match[1 : len(match)-1] // strip the surrounding { }
		return vars[name]
	})

	// Defence-in-depth: ensure the rendered output does not contain
	// 8+ consecutive digits. This is a backstop; the database CHECK
	// constraint is the primary line of defence.
	if ContainsRawPhoneDigits(rendered) {
		return "", ErrRenderedOutputContainsRawDigits
	}

	return rendered, nil
}

// allWhitespace reports whether s contains only whitespace characters.
// Used by RenderTemplate to reject whitespace-only templates.
func allWhitespace(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '\v' && c != '\f' {
			return false
		}
	}
	return true
}