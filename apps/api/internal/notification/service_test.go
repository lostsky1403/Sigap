package notification

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateEnqueue_RejectsEmptyFields(t *testing.T) {
	good := EnqueueInput{
		Channel:          ChannelDev,
		TemplateKey:      "appointment.booked.confirmation",
		Subject:          "Konfirmasi",
		BodyTemplate:     "Anda terdaftar.",
		RecipientType:    RecipientPatient,
		RecipientContact: "+6281234567890",
	}

	cases := []struct {
		name string
		mut  func(e *EnqueueInput)
		want error
	}{
		{"empty template_key", func(e *EnqueueInput) { e.TemplateKey = "" }, ErrEmptyTemplateKey},
		{"whitespace template_key", func(e *EnqueueInput) { e.TemplateKey = "   " }, ErrEmptyTemplateKey},
		{"empty subject", func(e *EnqueueInput) { e.Subject = "" }, ErrEmptySubject},
		{"empty body", func(e *EnqueueInput) { e.BodyTemplate = "" }, ErrEmptyBodyTemplate},
		{"empty contact", func(e *EnqueueInput) { e.RecipientContact = "" }, ErrEmptyRecipientContact},
		{"invalid channel", func(e *EnqueueInput) { e.Channel = Channel("nope") }, ErrInvalidChannel},
		{"invalid recipient", func(e *EnqueueInput) { e.RecipientType = RecipientType("alien") }, ErrInvalidRecipientType},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := good
			tc.mut(&e)
			if err := validateEnqueue(e); err != tc.want {
				t.Errorf("validateEnqueue: got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateEnqueue_AcceptsGoodInput(t *testing.T) {
	in := EnqueueInput{
		Channel:          ChannelDev,
		TemplateKey:      "appointment.booked.confirmation",
		Subject:          "Konfirmasi",
		BodyTemplate:     "Anda terdaftar pada 09:00.",
		RecipientType:    RecipientPatient,
		RecipientContact: "+6281234567890",
	}
	if err := validateEnqueue(in); err != nil {
		t.Errorf("validateEnqueue rejected good input: %v", err)
	}
}

func TestValidateEnqueue_NilFacilityIDAllowed(t *testing.T) {
	in := EnqueueInput{
		FacilityID:       nil,
		Channel:          ChannelDev,
		TemplateKey:      "appointment.booked.confirmation",
		Subject:          "Konfirmasi",
		BodyTemplate:     "Anda terdaftar.",
		RecipientType:    RecipientPatient,
		RecipientContact: "+6281234567890",
	}
	if err := validateEnqueue(in); err != nil {
		t.Errorf("validateEnqueue rejected nil FacilityID: %v", err)
	}
}

func TestChannelStatusRecipientType_Valid(t *testing.T) {
	goodChannels := []Channel{ChannelDev, ChannelSMS, ChannelWhatsApp, ChannelEmail}
	for _, c := range goodChannels {
		if !c.Valid() {
			t.Errorf("Channel %q reported invalid", c)
		}
	}
	for _, c := range []Channel{"", "telegram", "x"} {
		if c.Valid() {
			t.Errorf("Channel %q reported valid", c)
		}
	}

	goodStatuses := []Status{StatusPending, StatusProcessing, StatusDelivered, StatusFailed, StatusCancelled}
	for _, s := range goodStatuses {
		if !s.Valid() {
			t.Errorf("Status %q reported invalid", s)
		}
	}
	for _, s := range []Status{"", "unknown", "retry"} {
		if s.Valid() {
			t.Errorf("Status %q reported valid", s)
		}
	}

	goodRecipients := []RecipientType{RecipientPatient, RecipientStaff, RecipientFacilityAdmin}
	for _, r := range goodRecipients {
		if !r.Valid() {
			t.Errorf("RecipientType %q reported invalid", r)
		}
	}
	for _, r := range []RecipientType{"", "doctor", "x"} {
		if r.Valid() {
			t.Errorf("RecipientType %q reported valid", r)
		}
	}
}

// TestOutboxRowHasNoRawContactField is a compile-time guarantee that the
// OutboxRow struct (which is what JSON-marshals and is returned by the
// API) does not expose the un-masked contact field or the dedup hash.
// If someone adds either field, this test fails fast.
func TestOutboxRowHasNoRawContactField(t *testing.T) {
	rt := reflect.TypeOf(OutboxRow{})
	if _, ok := rt.FieldByName("RecipientContact"); ok {
		t.Errorf("OutboxRow must NOT have a RecipientContact field (it would expose raw PII)")
	}
	if _, ok := rt.FieldByName("RecipientContactHash"); ok {
		t.Errorf("OutboxRow must NOT have a RecipientContactHash field (hash is internal dedup key, never returned)")
	}
}

// TestMaskContactDoesNotLeakRawDigits is a regression guard: any future
// refactor of MaskPhone / MaskEmail that accidentally returns the raw
// value must fail this test.
func TestMaskContactDoesNotLeakRawDigits(t *testing.T) {
	inputs := []string{
		"+6281234567890",
		"alice.wonderland@example.com",
		"6281234567",
	}
	for _, in := range inputs {
		m1 := MaskPhone(in)
		m2 := MaskEmail(in)
		combined := m1 + " " + m2
		digits := onlyDigits(in)
		if len(digits) > 4 {
			bulk := digits[:len(digits)-4]
			if strings.Contains(combined, bulk) {
				t.Errorf("mask leaked bulk digits %q in %q + %q", bulk, m1, m2)
			}
		}
	}
}

// Compile-time guard: the helper UUIDs in tests must remain valid.
var _ = uuid.MustParse("00000000-0000-0000-0000-000000000001")
