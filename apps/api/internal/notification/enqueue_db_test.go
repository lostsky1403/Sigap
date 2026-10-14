package notification

// Strict app/DB agreement regression test.
//
// The failure this pins: the Go denylist and the notification_outbox CHECK
// constraints are two implementations of ONE security predicate. When they
// diverge, a value the service accepts is rejected later by the database — and
// because Enqueue runs on a fire-and-forget goroutine, the notification is lost
// with only a log line.
//
// Every vector is driven through the REAL Service.Enqueue against a REAL
// PostgreSQL database carrying the release schema, and the observed outcome is
// compared with the Go-layer verdict. Any divergence fails the test.
//
// Requires DATABASE_URL. Skips when unset.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/migrate"
)

type agreementVector struct {
	name string
	body string
	vars map[string]string
	// wantRejectedByGo is the Go-layer verdict. The DB verdict must match it.
	wantRejectedByGo bool
	// why documents the security or product rationale.
	why string
}

func TestEnqueue_GoAndDatabaseAgree(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping Enqueue agreement test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	svc := NewService(pool)

	// Ensure the release schema is present. Applying the full migration set is
	// idempotent, so this is a no-op when another test already built it.
	dir, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve migration dir: %v", err)
	}
	if _, err := migrate.Run(ctx, pool, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// Confirm the schema under test really is the strengthened release shape.
	def, valid := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_body_chk")
	if !valid || !containsSub(def, "{10,}") {
		t.Fatalf("database is not the strengthened release schema: def=%q validated=%v", def, valid)
	}

	vectors := []agreementVector{
		// --- unformatted telephone numbers: must be rejected ---
		{"raw_phone_12", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "081234567890"}, true, "raw phone must never reach the body"},
		{"raw_phone_intl", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "6281234567890"}, true, "raw phone must never reach the body"},
		{"raw_phone_8", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "12345678"}, true, "exactly 8 digits is still a phone-length run"},
		{"raw_phone_in_facility_name", "Fasilitas {facility_name} menerima kunjungan.", map[string]string{"facility_name": "Klinik 081234567890"}, true, "database-controlled name can leak a phone too"},

		// --- formatted Indonesian phone numbers: must be rejected ---
		{"dash_phone", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "0812-3456-7890"}, true, "separated phone"},
		{"space_phone", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "0812 3456 7890"}, true, "separated phone"},
		{"dot_phone", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "0812.3456.7890"}, true, "separated phone"},
		{"paren_phone", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "(021) 555-1234"}, true, "separated phone"},

		// --- international formats: must be rejected ---
		{"intl_plus62", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "+62 812 3456 7890"}, true, "international phone"},
		{"intl_dash", "Nomor antrean: {queue_number}.", map[string]string{"queue_number": "+62-812-3456-7890"}, true, "international phone"},

		// --- legitimate content: must be ACCEPTED ---
		{"booking_confirmation", "Janji temu Anda berhasil dicatat. Kode check-in: {checkin_code}.",
			map[string]string{"checkin_code": "AB12CD"}, false, "the live booking template"},
		{"checkin_confirmation", "Check-in Anda berhasil. Nomor antrean: {queue_number}.",
			map[string]string{"queue_number": "FSK-0001"}, false, "the live check-in template"},
		{"checkin_alnum_code", "Check-in Anda berhasil. Nomor antrean: {queue_number}.",
			map[string]string{"queue_number": "PUSKESMAS-0042"}, false, "alphabetic facility prefix"},
		{"short_numeric_id", "Kode check-in: {checkin_code}.", map[string]string{"checkin_code": "1234567"}, false, "7-digit id is not a phone"},
		{"currency", "Biaya Rp {appointment_code} telah dibayarkan.", map[string]string{"appointment_code": "1.250.000"}, false, "currency must not be blocked"},
		{"iso_date_only", "Kunjungan pada {appointment_time}.", map[string]string{"appointment_time": "2026-10-09"}, false, "a bare ISO date is not a phone"},
		{"localized_datetime", "Janji temu pada {appointment_time}.",
			map[string]string{"appointment_time": "9 Oktober 2026 pukul 14.30"}, false, "the supported way to render a timestamp"},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			cctx, ccancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer ccancel()

			rendered, renderErr := RenderTemplate(v.body, v.vars)
			goRejected := renderErr != nil

			// The DATABASE verdict, observed independently.
			//
			// Service.Enqueue is NOT sufficient for this: it checks the subject
			// and the rendered body with the Go predicate BEFORE it reaches SQL
			// (service.go), so for any vector Go rejects, Enqueue would return
			// the Go verdict again and the database would never be consulted —
			// making the divergence assertion vacuous for exactly the vectors
			// that matter. Inserting the value directly bypasses the Go layer, so
			// what comes back is the CHECK constraint's own opinion.
			//
			// RenderTemplate returns "" on error, so the value the database would
			// actually have received is reconstructed here by the same purely
			// lexical substitution the renderer performs (it substitutes {name}
			// and does nothing else).
			toInsert := rendered
			if renderErr != nil {
				toInsert = substituteVars(v.body, v.vars)
			}
			dbRejected := phoneConstraintRejected(insertRawBody(t, pool, toInsert))

			// The end-to-end path, through the real service.
			_, enqueueErr := svc.Enqueue(cctx, EnqueueInput{
				Channel:             ChannelDev,
				TemplateKey:         "agree." + v.name,
				Subject:             "Uji Kontrak Notifikasi",
				BodyTemplate:        v.body,
				TemplateVars:        v.vars,
				RecipientType:       RecipientPatient,
				RecipientContact:    "+6281234567890",
				RelatedResourceType: "appointment",
				RelatedResourceID:   uuid.New().String(),
			})
			enqueueRejected := enqueueErr != nil

			if goRejected != v.wantRejectedByGo {
				t.Errorf("Go verdict = rejected:%v, want rejected:%v (%s)", goRejected, v.wantRejectedByGo, v.why)
			}
			if dbRejected != v.wantRejectedByGo {
				t.Errorf("DB verdict = rejected:%v, want rejected:%v (%s)", dbRejected, v.wantRejectedByGo, v.why)
			}
			// The contract: the two layers must never disagree. If Go accepts a
			// value the DB rejects, the insert fails after the fact and the
			// notification is silently lost.
			if goRejected != dbRejected {
				t.Errorf("APP/DB DIVERGENCE (%s): Go rejected=%v but DB rejected=%v; goErr=%v",
					v.why, goRejected, dbRejected, renderErr)
			}
			// Enqueue must agree with the Go layer: it is the same predicate, and
			// a mismatch here would mean Enqueue is not applying it.
			if enqueueRejected != goRejected {
				t.Errorf("ENQUEUE/GO MISMATCH (%s): Enqueue rejected=%v but the Go predicate rejected=%v; enqueueErr=%v",
					v.why, enqueueRejected, goRejected, enqueueErr)
			}
		})
	}
}

// TestPredicate_CorpusAgreesWithTheDatabase runs the SHARED predicateCorpus
// (denylist_test.go) through a live notification_outbox CHECK constraint, by
// inserting each value directly so the Go layer cannot mask the database's
// verdict. This is the test that can actually detect a Go↔PostgreSQL
// divergence — the in-process transcription check cannot, because it compares
// two Go regexps.
//
// Requires DATABASE_URL. Skips when unset.
func TestPredicate_CorpusAgreesWithTheDatabase(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping predicate corpus/database agreement test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	dir, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve migration dir: %v", err)
	}
	if _, err := migrate.Run(ctx, pool, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	def, valid := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_body_chk")
	if !valid || !containsSub(def, "{10,}") {
		t.Fatalf("database is not the strengthened release schema: def=%q validated=%v", def, valid)
	}

	for i, s := range predicateCorpus {
		goVerdict := ContainsRawPhoneDigits(s)
		dbVerdict := phoneConstraintRejected(insertRawBody(t, pool, s))
		if goVerdict != dbVerdict {
			t.Errorf("APP/DB DIVERGENCE on corpus[%d] %q: Go rejected=%v but DB rejected=%v", i, s, goVerdict, dbVerdict)
		}
	}
}

// phoneConstraintRejected reports whether the database refused a value
// SPECIFICALLY because of the notification_outbox phone predicate.
//
// The distinction matters: the corpus contains "" (a value the predicate
// correctly calls phone-free), and inserting an empty body_template is refused
// by an unrelated constraint. Treating any insert error as a phone rejection
// would make the empty string look like a predicate divergence.
func phoneConstraintRejected(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return strings.Contains(pgErr.ConstraintName, "no_raw_phone")
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// substituteVars performs the renderer's purely lexical {name} -> value
// substitution, WITHOUT its safety check. It exists so a test can reconstruct
// the exact value the database would have received for a vector the Go layer
// refused — RenderTemplate returns "" on error, so the rendered text is
// otherwise unavailable.
func substituteVars(tpl string, vars map[string]string) string {
	out := tpl
	for name, value := range vars {
		out = strings.ReplaceAll(out, "{"+name+"}", value)
	}
	return out
}

// TestRenderedQueueNumber_GoAndDatabaseAgree proves the short_code invariant at
// the layer that actually enforces it.
//
// facilities.short_code is interpolated into the queue number by the queue
// engine as `{short_code}-{next_number:04}` (apps/queue-engine/src/engine/
// queue.rs), and that queue number reaches notification_outbox.body_template
// through the {queue_number} template variable. The phone CHECK constraints
// therefore apply to the RENDERED queue number, and a short_code that renders
// an unsafe queue number would make EVERY notification for that facility fail
// at the database — silently, because Enqueue runs on a fire-and-forget
// goroutine.
//
// The handler's validateShortCode calls ContainsRawPhoneDigits on the rendered
// form. This test closes the loop: it inserts the rendered string DIRECTLY,
// bypassing the Go predicate entirely, and observes whether the DATABASE's own
// CHECK constraint accepts it. Go's verdict must equal the database's.
//
// Requires DATABASE_URL. Skips when unset.
func TestRenderedQueueNumber_GoAndDatabaseAgree(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping rendered-queue-number agreement test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	dir, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve migration dir: %v", err)
	}
	if _, err := migrate.Run(ctx, pool, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	def, valid := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_body_chk")
	if !valid || !containsSub(def, "{10,}") {
		t.Fatalf("database is not the strengthened release schema: def=%q validated=%v", def, valid)
	}

	cases := []struct {
		shortCode string
		// wantUnsafe is the verdict the handler's validator must reach, and the
		// verdict the database must independently reach on the rendered value.
		wantUnsafe bool
		why        string
	}{
		{"RSK", false, "a committed seed short_code"},
		{"PKM", false, "a committed seed short_code"},
		{"DEMO-GIGI", false, "a committed seed short_code with an internal hyphen"},
		{"f1", false, "a committed seed short_code with a lowercase prefix"},
		{"E2E123456", false, "the E2E suite's own code: 6 trailing digits, rendered span 11"},
		{"E2E000001", false, "the E2E suite's own code, lowest tag"},
		{"A123456", false, "6 trailing digits: the boundary the validator admits"},
		{"AB-123456", false, "6 trailing digits behind a hyphen"},
		{"A1234567", true, "7 trailing digits: rendered span 12, over the line"},
		{"E2E1234567", true, "the E2E shape with one digit too many"},
		{"AB12345678", true, "8 consecutive digits: the first conjunct"},
		{"ABCDEF1234567", true, "7 trailing digits behind a long prefix"},
	}

	// The counter is always four digits ({:04}, capped at 300), so every
	// counter yields the same rendered length; test both ends of the range.
	counters := []int{1, 300}

	for _, c := range cases {
		t.Run(c.shortCode, func(t *testing.T) {
			for _, n := range counters {
				rendered := c.shortCode + "-" + fmt.Sprintf("%04d", n)

				goVerdict := ContainsRawPhoneDigits(rendered)
				if goVerdict != c.wantUnsafe {
					t.Errorf("Go verdict for %q = unsafe:%v, want unsafe:%v (%s)",
						rendered, goVerdict, c.wantUnsafe, c.why)
				}

				// Bypass the Go predicate: write straight to the table.
				dbRejected := phoneConstraintRejected(insertRawBody(t, pool, rendered))
				if dbRejected != c.wantUnsafe {
					t.Errorf("DB verdict for %q = rejected:%v, want rejected:%v (%s)",
						rendered, dbRejected, c.wantUnsafe, c.why)
				}
				if goVerdict != dbRejected {
					t.Errorf("APP/DB DIVERGENCE for %q: Go rejected=%v but DB rejected=%v (%s)",
						rendered, goVerdict, dbRejected, c.why)
				}
			}
		})
	}
}

// insertRawBody writes body directly into notification_outbox, without going
// through Service.Enqueue, so the Go predicate cannot mask the database's own
// verdict. Returns the error the constraint raises, if any.
func insertRawBody(t *testing.T, pool *pgxpool.Pool, body string) error {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO notification_outbox
		   (channel, template_key, subject, body_template, recipient_type,
		    recipient_contact_masked, recipient_contact_hash, status,
		    attempt_count, next_attempt_at)
		 VALUES ('dev', 'queue.' || substr(md5(random()::text), 1, 8),
		         'Uji Nomor Antrean', $1, 'patient',
		         '+62••••1234', decode(repeat('00',32),'hex'), 'pending', 0, NOW())`,
		body)
	return err
}
