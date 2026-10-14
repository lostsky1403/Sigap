# Dead Public API — Impact Analysis (Phase 3B7 §13)

**Scope:** read-only analysis. **No Go router change is made by Phase 3B7.**
**Trigger:** Gate 6 reported two now-unused public backend surfaces after the legacy demo
dashboard was deleted (T-3B6-02).

The deleted `BedAvailabilityDashboard.svelte` was the only known web consumer of both.
This document classifies each against: current consumers, external/public-contract risk,
auth policy, documentation references, and tests.

---

## Summary

| Endpoint | Classification | Auth | Recommendation |
|---|---|---|---|
| `GET /api/v1/facilities/nearby` | **A — safe dead endpoint candidate** (and it serves fabricated data) | public | remove in a separate, reviewed task |
| `GET /api/v1/events/beds` | **B — compatibility surface intentionally retained** | public (SSE) | retain; revisit only with a product decision |

Neither is removed here: T-3B7 is a readiness phase, and the canonical task list does not
require a router change. Both are recorded as post-redesign technical debt.

---

## 1. `GET /api/v1/facilities/nearby`

**Registration**
- mux: `apps/api/cmd/server/main.go:359` → `enableCORS(facilitiesNearbyHandler)`
- registry: `apps/api/internal/router/router.go:36` (no `RequiredPolicy` ⇒ public)
- handler: `apps/api/cmd/server/main.go:467-…`

**Consumers**
- Web: **none.** There is no SvelteKit proxy for it (`apps/web/src/routes/api/v1/facilities/nearby/` does not exist).
- Repo-wide, the only remaining references are the Go registration, the router test, and
  prose in `docs/FACILITY_ADMIN_REPORT.md`, `docs/PRODUCTION_READINESS_AUDIT.md`, and
  `design/sigap-redesign-implementation-plan.md` (Appendix A). `README.md` does **not**
  mention it.
- The deleted `BedAvailabilityDashboard.svelte` fetched `…/facilities/nearby?lat=…`.

**What it actually returns — and why this is more than dead code**
The handler does **not** query the database. It returns a **hardcoded** list of six
facilities with synthetic IDs (`f1`…`f6`) and fixed bed counts
(`apps/api/cmd/server/main.go:478-495`). The comment says so explicitly
("Hardcoded master list … In prod: SELECT … from facilities + ST_Distance").

So on a public, unauthenticated route, the API serves **fabricated operational data** —
including bed-availability numbers — that does not correspond to any real facility. A
client that trusts it would display invented capacity figures.

**Auth policy:** public (no `RequiredPolicy`; not in the `AllowList`, so it passes through
the deny-by-default router as a declared public route).

**Contract risk:** low-to-moderate. It is a public endpoint, so an unknown external consumer
*could* exist; but it is undocumented as a public API, is not consumed anywhere in this
repository, and returns placeholder data no real client could rely on.

**Tests:** `apps/api/internal/router/router_test.go:39` asserts it is reachable as a public
route. That test pins *intent* (it is declared public), not usage.

**Classification: A — safe dead endpoint candidate.** It is unreferenced by any consumer,
returns fabricated data, and carries no external contract. It should be removed in a
separate, reviewed task together with its router-test row and its documentation mentions.
It is **not** removed in 3B7 (out of scope; a Go router change is not required for readiness).

---

## 2. `GET /api/v1/events/beds`

**Registration**
- mux: `apps/api/cmd/server/main.go:356` → `enableCORS(events.Bus.ServeSSE)`
- registry: `apps/api/internal/router/router.go:35` (no `RequiredPolicy` ⇒ public)
- web proxy: `apps/web/src/routes/api/v1/events/beds/+server.ts` **exists** (a live SvelteKit route)

**Consumers**
- Web page/component: **none.** No `EventSource('/api/v1/events/beds')` remains in
  `apps/web/src` (the deleted dashboard held the only one).
- The SvelteKit proxy route still exists and still forwards to the Go SSE stream.
- Documentation references: `README.md` (architecture diagram + SSE example),
  `docs/LOCAL_DEMO_RUNBOOK.md`, `docs/LOCAL_DEMO_RUNBOOK.id.md`,
  `docs/PRODUCTION_READINESS_AUDIT.md`.

**Auth policy:** public (SSE; EventSource cannot send custom auth headers, which is why it
is public by design).

**Contract risk:** low, but distinct from §1 — this is a **real-time stream** whose proxy
route is part of the web build. Removing it is a two-sided change (Go mux + registry **and**
the SvelteKit proxy), and it is referenced by the demo runbooks as the live-update mechanism.

**Tests:** `apps/web/tests/proxy-contracts.test.js` pins the proxy contract;
`apps/api/internal/router/router_test.go` pins the public route.

**Classification: B — compatibility surface intentionally retained.** The stream is a
designed public surface with a live proxy; it is not obviously dead in the way §1 is. Its
web consumer is gone, so it is currently unused *in this repo*, but removing it is a
product decision (does Sigap still want a public bed/queue event stream?) rather than a
cleanup. Recorded as technical debt; not removed in 3B7.

---

## 3. Why neither is removed in Phase 3B7

- T-3B7 is a **readiness** phase; its canonical tasks are documentation (T-3B7-01), smoke
  extension (T-3B7-02), and config safety (T-3B7-03). None requires a router change.
- Removing a **public** endpoint is an API-contract change with its own risk surface
  (unknown external consumers) and its own review needs.
- The Gate 6 security review explicitly advised against automatic removal and recommended
  impact analysis first — which is this document.

## 4. Recommended follow-up (not part of 3B7)

1. Delete `facilitiesNearbyHandler` and its mux registration; remove the
   `/api/v1/facilities/nearby` row from `router.Registry` and from `router_test.go`; update
   the README/audit prose. **Priority: higher than §2**, because it serves fabricated data
   publicly.
2. Decide whether the public bed/queue event stream is still a product requirement. If not,
   remove the Go handler + registry row and the SvelteKit proxy together, and update
   `proxy-contracts.test.js` and the demo runbooks. If yes, document it as a supported
   public surface.
