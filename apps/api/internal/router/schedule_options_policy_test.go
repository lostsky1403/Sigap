package router

import (
	"net/http"
	"testing"
)

// TestScheduleOptionsRoutePolicy pins the COARSE route policy for the Phase
// 3B5.0 schedule mutation-options endpoint.
//
// The endpoint answers a PER-FACILITY question ("which facilities may I build a
// schedule for?") and the handler answers it from DB-resolved schedule.manage
// provenance. The route-level policy is therefore only a coarse admission
// check, and it is deliberately `schedule.read` — the same gate the schedule
// LIST uses. Three properties are worth stating explicitly, because each is a
// decision that could be quietly undone:
//
//  1. The path is REACHABLE. `/api/v1/admin/schedules/options` is matched by
//     the existing `schedule.read` prefix rule. If someone ever registers a
//     narrower or earlier rule, this fails rather than silently changing which
//     routes the handler serves.
//
//  2. The policy is `schedule.read`, NOT `schedule.manage`. Gating on manage
//     would reject — before the handler ever runs — the exact actor the feature
//     exists for: one who manages facility B but only reads A and B. That is
//     the mixed-provenance actor of §5/§12, and a 403 for them would be both
//     wrong and unhelpful, because the honest answer is "you may manage some
//     facilities", not "you may manage none".
//
//  3. The detail read is NOT HIJACKED. An exact `/options` rule placed BEFORE
//     the prefix rule would silently take over matching for every
//     `/api/v1/admin/schedules/*` detail read. `Match` returns the first hit,
//     so ordering is load-bearing, not cosmetic.
func TestScheduleOptionsRoutePolicy(t *testing.T) {
	const optionsPath = "/api/v1/admin/schedules/options"

	rule, ok := Match(http.MethodGet, optionsPath)
	if !ok {
		t.Fatalf("GET %s is not routable; the schedule.read prefix rule must cover it", optionsPath)
	}
	if rule.RequiredPolicy != "schedule.read" {
		t.Errorf("GET %s policy = %q, want %q — the coarse gate must stay schedule.read so a "+
			"facility-scoped schedule manager is not rejected before the handler runs",
			optionsPath, rule.RequiredPolicy, "schedule.read")
	}

	// The detail read must still resolve to the same prefix rule. If this ever
	// resolves to a DIFFERENT rule, an exact entry was inserted ahead of the
	// prefix and is now hijacking detail reads.
	detailPath := "/api/v1/admin/schedules/550e8400-e29b-41d4-a716-446655440000"
	detailRule, ok := Match(http.MethodGet, detailPath)
	if !ok {
		t.Fatalf("GET %s is not routable", detailPath)
	}
	if detailRule.Path != rule.Path {
		t.Errorf("detail read resolved to rule %q but options resolved to %q; "+
			"an exact entry placed before the prefix rule would take over detail matching",
			detailRule.Path, rule.Path)
	}

	// The mutation routes must NOT have been loosened. Creating and patching a
	// schedule are still schedule.manage.
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/admin/schedules"},
		{http.MethodPatch, "/api/v1/admin/schedules/550e8400-e29b-41d4-a716-446655440000"},
	} {
		mutationRule, ok := Match(c.method, c.path)
		if !ok {
			t.Fatalf("%s %s is not routable", c.method, c.path)
		}
		if mutationRule.RequiredPolicy != "schedule.manage" {
			t.Errorf("%s %s policy = %q, want schedule.manage",
				c.method, c.path, mutationRule.RequiredPolicy)
		}
	}

	// A non-admin method on the options path must still be denied, so the
	// endpoint cannot be reached as a write.
	if _, ok := Match(http.MethodDelete, optionsPath); ok {
		t.Errorf("DELETE %s must not be routable", optionsPath)
	}
	if _, ok := Match(http.MethodPost, optionsPath); ok {
		t.Errorf("POST %s must not be routable; options is a read", optionsPath)
	}
}
