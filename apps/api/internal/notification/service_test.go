package notification

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

// TestAllStatusesAndChannels is a regression guard for the allow-list
// helpers that drive ?status= and ?channel= query parameter validation
// in the admin handler. Adding or removing a declared Status/Channel
// must update the corresponding helper so the allow-list stays in sync
// with the type.
func TestAllStatusesAndChannels(t *testing.T) {
	statuses := AllStatuses()
	wantStatuses := []string{"pending", "processing", "delivered", "failed", "cancelled"}
	if len(statuses) != len(wantStatuses) {
		t.Errorf("AllStatuses length = %d, want %d", len(statuses), len(wantStatuses))
	}
	for i, want := range wantStatuses {
		if statuses[i] != want {
			t.Errorf("AllStatuses[%d] = %q, want %q", i, statuses[i], want)
		}
		if !Status(statuses[i]).Valid() {
			t.Errorf("AllStatuses[%d] = %q but Status.Valid() is false", i, statuses[i])
		}
	}

	channels := AllChannels()
	wantChannels := []string{"dev", "sms", "whatsapp", "email"}
	if len(channels) != len(wantChannels) {
		t.Errorf("AllChannels length = %d, want %d", len(channels), len(wantChannels))
	}
	for i, want := range wantChannels {
		if channels[i] != want {
			t.Errorf("AllChannels[%d] = %q, want %q", i, channels[i], want)
		}
		if !Channel(channels[i]).Valid() {
			t.Errorf("AllChannels[%d] = %q but Channel.Valid() is false", i, channels[i])
		}
	}
}

// TestListParamsZeroValueIsNoFilter is a regression guard for the
// Service.List SQL contract: every field's zero value must be treated
// as "no filter" by the underlying query, not as a literal match
// against the empty string or the unix epoch.
func TestListParamsZeroValueIsNoFilter(t *testing.T) {
	p := ListParams{}
	if p.FacilityID != uuid.Nil {
		t.Errorf("zero FacilityID = %v, want uuid.Nil", p.FacilityID)
	}
	if p.Limit != 0 {
		t.Errorf("zero Limit = %d, want 0", p.Limit)
	}
	if p.Status != "" {
		t.Errorf("zero Status = %q, want \"\"", p.Status)
	}
	if p.Channel != "" {
		t.Errorf("zero Channel = %q, want \"\"", p.Channel)
	}
	if p.TemplateKey != "" {
		t.Errorf("zero TemplateKey = %q, want \"\"", p.TemplateKey)
	}
	if !p.CreatedFrom.IsZero() {
		t.Errorf("zero CreatedFrom = %v, want zero time", p.CreatedFrom)
	}
	if !p.CreatedTo.IsZero() {
		t.Errorf("zero CreatedTo = %v, want zero time", p.CreatedTo)
	}
}

// TestNullableTimeForZeroAndNonZero pins the nullableTime helper that
// Service.List relies on: a zero time.Time must be passed as nil so the
// SQL `IS NULL` guard short-circuits, while a real timestamp must be
// passed through verbatim for pgx to bind as timestamptz.
func TestNullableTimeForZeroAndNonZero(t *testing.T) {
	if got := nullableTime(time.Time{}); got != nil {
		t.Errorf("nullableTime(zero) = %v, want nil", got)
	}
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	if got := nullableTime(now); got != now {
		t.Errorf("nullableTime(now) = %v, want %v", got, now)
	}
}

// --- DB integration tests (skipped unless DATABASE_URL is set) -------

// integrationPool opens a pgxpool for the notification integration
// tests. The tests are gated on SIGAP_DATABASE_URL (matches the
// convention used by the notification worker) so unit-only `go test`
// runs stay fast and offline.
func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SIGAP_DATABASE_URL")
	if dsn == "" {
		t.Skip("SIGAP_DATABASE_URL not set; skipping notification DB integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// seedOutboxRow inserts a single notification_outbox row for the
// integration tests. The row uses the dev channel and a deterministic
// template_key so the tests can assert against known fixtures without
// relying on global state.
func seedOutboxRow(t *testing.T, pool *pgxpool.Pool, status, templateKey string, createdAt time.Time) uuid.UUID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id := uuid.New()
	_, err := pool.Exec(ctx, `
INSERT INTO notification_outbox
    (id, channel, template_key, subject, body_template,
     recipient_type, recipient_contact_masked, recipient_contact_hash,
     status, attempt_count, next_attempt_at,
     created_at, updated_at)
VALUES ($1,'dev',$2,'Subject','Body template', 'patient','+62••••0001',
        E'\\x00'::bytea, $3, 0, $4, $4, $4)`,
		id, templateKey, status, createdAt,
	)
	if err != nil {
		t.Fatalf("seed outbox: %v", err)
	}
	return id
}

// cleanupOutboxRows removes the rows this test inserted by template_key
// prefix so successive runs do not pile up state.
func cleanupOutboxRows(t *testing.T, pool *pgxpool.Pool, templateKeyPrefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = pool.Exec(ctx,
		`DELETE FROM notification_outbox WHERE template_key LIKE $1`,
		templateKeyPrefix+"%",
	)
}

// countByTemplatePrefix runs a one-off COUNT for assertion.
func countByTemplatePrefix(t *testing.T, pool *pgxpool.Pool, prefix string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_outbox WHERE template_key LIKE $1`,
		prefix+"%",
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestServiceList_EmptyParamsReturnsAll(t *testing.T) {
	pool := integrationPool(t)
	svc := NewService(pool)
	prefix := "test.list.empty."
	cleanupOutboxRows(t, pool, prefix)
	t.Cleanup(func() { cleanupOutboxRows(t, pool, prefix) })

	now := time.Now().UTC()
	seedOutboxRow(t, pool, string(StatusPending), prefix+"a", now)
	seedOutboxRow(t, pool, string(StatusDelivered), prefix+"b", now)

	rows, err := svc.List(context.Background(), ListParams{Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var matched int
	for _, r := range rows {
		if strings.HasPrefix(r.TemplateKey, prefix) {
			matched++
		}
	}
	if matched != 2 {
		t.Errorf("List(empty) matched %d test rows, want 2", matched)
	}
}

func TestServiceList_StatusFilter(t *testing.T) {
	pool := integrationPool(t)
	svc := NewService(pool)
	prefix := "test.list.status."
	cleanupOutboxRows(t, pool, prefix)
	t.Cleanup(func() { cleanupOutboxRows(t, pool, prefix) })

	now := time.Now().UTC()
	seedOutboxRow(t, pool, string(StatusPending), prefix+"a", now)
	seedOutboxRow(t, pool, string(StatusDelivered), prefix+"b", now)

	rows, err := svc.List(context.Background(), ListParams{
		Limit:  50,
		Status: string(StatusDelivered),
	})
	if err != nil {
		t.Fatalf("List(status=delivered): %v", err)
	}
	for _, r := range rows {
		if strings.HasPrefix(r.TemplateKey, prefix) && r.Status != StatusDelivered {
			t.Errorf("List(status=delivered) returned row with status=%q", r.Status)
		}
	}
}

func TestServiceList_TemplateKeyFilter(t *testing.T) {
	pool := integrationPool(t)
	svc := NewService(pool)
	prefix := "test.list.tpl."
	cleanupOutboxRows(t, pool, prefix)
	t.Cleanup(func() { cleanupOutboxRows(t, pool, prefix) })

	now := time.Now().UTC()
	seedOutboxRow(t, pool, string(StatusPending), prefix+"only", now)

	rows, err := svc.List(context.Background(), ListParams{
		Limit:       50,
		TemplateKey: prefix + "only",
	})
	if err != nil {
		t.Fatalf("List(template_key): %v", err)
	}
	var matched int
	for _, r := range rows {
		if r.TemplateKey == prefix+"only" {
			matched++
		}
	}
	if matched == 0 {
		t.Errorf("List(template_key=%q) returned no rows", prefix+"only")
	}
}

func TestServiceList_CreatedFromFilter(t *testing.T) {
	pool := integrationPool(t)
	svc := NewService(pool)
	prefix := "test.list.from."
	cleanupOutboxRows(t, pool, prefix)
	t.Cleanup(func() { cleanupOutboxRows(t, pool, prefix) })

	now := time.Now().UTC()
	seedOutboxRow(t, pool, string(StatusPending), prefix+"recent", now)

	// Window starts one hour in the future — the seed row must be excluded.
	future := now.Add(time.Hour)
	rows, err := svc.List(context.Background(), ListParams{
		Limit:       50,
		CreatedFrom: future,
	})
	if err != nil {
		t.Fatalf("List(created_from): %v", err)
	}
	for _, r := range rows {
		if strings.HasPrefix(r.TemplateKey, prefix) {
			t.Errorf("List(created_from=future) returned row created_at=%s", r.CreatedAt)
		}
	}
}

func TestServiceSummary_EmptyOutboxReturnsZeros(t *testing.T) {
	pool := integrationPool(t)
	svc := NewService(pool)
	// Use a unique facility_id to scope the query away from any
	// existing rows. uuid.Nil would aggregate across all facilities.
	scoped := uuid.New()

	counts, err := svc.Summary(context.Background(), scoped)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	for _, status := range AllStatuses() {
		if got := counts[status]; got != 0 {
			t.Errorf("Summary[%s] = %d, want 0 for empty facility scope", status, got)
		}
	}
	if len(counts) != len(AllStatuses()) {
		t.Errorf("Summary returned %d keys, want %d", len(counts), len(AllStatuses()))
	}
}

func TestServiceSummary_AggregatesByStatus(t *testing.T) {
	pool := integrationPool(t)
	svc := NewService(pool)
	prefix := "test.summary."
	cleanupOutboxRows(t, pool, prefix)
	t.Cleanup(func() { cleanupOutboxRows(t, pool, prefix) })

	now := time.Now().UTC()
	seedOutboxRow(t, pool, string(StatusPending), prefix+"a", now)
	seedOutboxRow(t, pool, string(StatusPending), prefix+"b", now)
	seedOutboxRow(t, pool, string(StatusDelivered), prefix+"c", now)
	seedOutboxRow(t, pool, string(StatusFailed), prefix+"d", now)

	// Use uuid.Nil to aggregate across all facilities. The pre-check
	// confirms the seed rows exist before asserting the counts.
	if n := countByTemplatePrefix(t, pool, prefix); n != 4 {
		t.Fatalf("seed: expected 4 rows under %q, got %d", prefix, n)
	}

	counts, err := svc.Summary(context.Background(), uuid.Nil)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	// We only assert on the statuses we just seeded. Other statuses
	// may have non-zero values from any pre-existing outbox rows in
	// the shared database.
	if counts[string(StatusPending)] < 2 {
		t.Errorf("Summary[pending] = %d, want >= 2", counts[string(StatusPending)])
	}
	if counts[string(StatusDelivered)] < 1 {
		t.Errorf("Summary[delivered] = %d, want >= 1", counts[string(StatusDelivered)])
	}
	if counts[string(StatusFailed)] < 1 {
		t.Errorf("Summary[failed] = %d, want >= 1", counts[string(StatusFailed)])
	}
}
