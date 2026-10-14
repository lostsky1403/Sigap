package notification

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Anti-drift guard: the affordance predicate and the mutation SQL must agree.
//
// WHY THIS TEST EXISTS
//
// Retry and Cancel enforce their state rules in SQL, not in Go:
//
//	UPDATE notification_outbox SET ... WHERE id = $1 AND status IN ('failed','pending')
//	UPDATE notification_outbox SET ... WHERE id = $1 AND status IN ('pending','failed','cancelled')
//
// Phase 3B5.0 introduced CanRetryStatus/CanCancelStatus, a PURE Go restatement
// of those two clauses, so the list endpoint can advertise affordances without
// mutating anything. That creates a duplication hazard with teeth: a pure
// restatement is trivially editable, and if the two copies drift the UI starts
// advertising actions the server will reject with 409.
//
// The behavioural proof lives in the handler package
// (TestNotificationActionability_N2b_AffordanceMatchesRealMutationOutcome),
// which actually calls retry and cancel. This test is the cheap, always-on
// guard that fires at the source: it parses the SQL and compares it to the Go
// predicate, so a drift is reported as a compile-adjacent failure rather than
// as a mysterious 409 in a UI test.
//
// The parse is deliberately structural (an IN-list scanner) rather than a
// substring match on a whole statement, so reformatting the SQL does not break
// it but CHANGING THE STATUSES does.
// ---------------------------------------------------------------------------

// sqlStatusInList extracts the status set from the first
// `status IN ('a','b',...)` clause found in the source.
var sqlStatusInList = regexp.MustCompile(`status\s+IN\s*\(([^)]*)\)`)

// parseSQLStatusSet finds the nth (0-indexed) `status IN (...)` clause and
// returns the quoted statuses inside it, lowercased and sorted.
func parseSQLStatusSet(t *testing.T, source string, occurrence int) []string {
	t.Helper()
	matches := sqlStatusInList.FindAllStringSubmatch(source, -1)
	if occurrence >= len(matches) {
		t.Fatalf("expected at least %d `status IN (...)` clauses in the source, found %d",
			occurrence+1, len(matches))
	}
	inner := matches[occurrence][1]
	quoted := regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(inner, -1)
	set := make([]string, 0, len(quoted))
	for _, q := range quoted {
		set = append(set, strings.ToLower(q[1]))
	}
	sort.Strings(set)
	return set
}

// statusSetFromPredicate is the same idea, but from the Go switch. The switch
// enumerates cases rather than holding a slice, so it is read through the
// predicate itself: a status is "in" the set if the predicate accepts it.
func statusSetFromPredicate(t *testing.T, all []string, predicate func(string) bool) []string {
	t.Helper()
	set := make([]string, 0, len(all))
	for _, status := range all {
		if predicate(status) {
			set = append(set, status)
		}
	}
	sort.Strings(set)
	return set
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestActionabilityPredicatesMatchMutationSQL compares each Go predicate
// against the status list its mutation actually enforces.
func TestActionabilityPredicatesMatchMutationSQL(t *testing.T) {
	source, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	all := AllStatuses()
	allStrings := make([]string, 0, len(all))
	for _, s := range all {
		allStrings = append(allStrings, string(s))
	}

	// Occurrence 0 is Retry's guard, occurrence 1 is Cancel's. The order is
	// the source order of the UPDATE statements, and the counts below assert
	// that assumption still holds, so inserting a new guarded UPDATE above
	// them fails loudly instead of silently re-pointing the comparison.
	t.Run("retry", func(t *testing.T) {
		sql := parseSQLStatusSet(t, string(source), 0)
		goSet := statusSetFromPredicate(t, allStrings, CanRetryStatus)
		if !equalStringSets(sql, goSet) {
			t.Errorf("retry affordance has drifted from the mutation SQL\n"+
				"  SQL allows:     %v\n"+
				"  CanRetryStatus: %v\n"+
				"If the SQL changed, update CanRetryStatus (or route the mutation through it).",
				sql, goSet)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		sql := parseSQLStatusSet(t, string(source), 1)
		goSet := statusSetFromPredicate(t, allStrings, CanCancelStatus)
		if !equalStringSets(sql, goSet) {
			t.Errorf("cancel affordance has drifted from the mutation SQL\n"+
				"  SQL allows:      %v\n"+
				"  CanCancelStatus: %v\n"+
				"If the SQL changed, update CanCancelStatus (or route the mutation through it).",
				sql, goSet)
		}
	})
}

// TestActionabilitySQLClauseCountIsStable pins the assumption the two
// subtests above depend on: that occurrences 0 and 1 really are Retry's and
// Cancel's guards. Without this, inserting a new status-guarded UPDATE earlier
// in the file would silently re-point the comparison at the wrong statement
// and the drift guard would go quiet.
func TestActionabilitySQLClauseCountIsStable(t *testing.T) {
	source, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	if got := len(sqlStatusInList.FindAllStringSubmatch(string(source), -1)); got != 2 {
		t.Fatalf("expected exactly 2 `status IN (...)` clauses in service.go, found %d — "+
			"if you added a guarded UPDATE, update TestActionabilityPredicatesMatchMutationSQL "+
			"to point at the right occurrences", got)
	}
}
