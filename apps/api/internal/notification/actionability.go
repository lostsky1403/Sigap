package notification

// ---------------------------------------------------------------------------
// Actionability (Phase 3B5.0)
//
// WHY THIS FILE EXISTS.
//
// Retry and Cancel are enforced in SQL: their UPDATE statements carry a
// `status IN (...)` predicate, and a row outside that set yields
// ErrInvalidState, which the handler maps to 409. That SQL is the authority and
// is not being changed.
//
// But the admin list now has to tell the browser which actions to OFFER. If
// that were computed from a second, hand-written copy of the status rules, the
// two would drift: someone would widen the SQL and forget the affordance, and
// the UI would offer an action the server refuses with a 409 — the exact
// "control that silently does nothing" failure Phase 3B4 was written against.
//
// So the status rules are declared ONCE, here, as a pure predicate, and
// `TestActionabilityMatchesMutationSQL` proves the declaration still matches the
// SQL text. The affordance calculation and the human-readable comment on the
// mutation handlers then both consume this one source.
//
// The mutation endpoints themselves do NOT consult this file: they keep using
// their SQL, which remains the enforcing layer. This is an affordance
// projection, never a second authorization or state authority.
// ---------------------------------------------------------------------------

// CanRetryStatus reports whether a notification in `status` is eligible for the
// retry transition.
//
// Mirrors Service.Retry: `AND status IN ('failed','pending')`. A pending row can
// be retried because the worker has not picked it up yet; re-queueing it clears
// the error and bumps the attempt counter.
func CanRetryStatus(status string) bool {
	switch status {
	case string(StatusPending), string(StatusFailed):
		return true
	}
	return false
}

// CanCancelStatus reports whether a notification in `status` may be cancelled.
//
// Mirrors Service.Cancel: `AND status IN ('pending','failed','cancelled')`. The
// 'cancelled' member is what makes cancel IDEMPOTENT — cancelling an already
// cancelled row succeeds and returns the current row, so the affordance stays
// true and the UI does not flip a control off for an action the server will
// happily accept.
//
// 'delivered' is the one status the SQL excludes, which is the entire reason a
// 409 exists on this endpoint.
func CanCancelStatus(status string) bool {
	switch status {
	case string(StatusPending), string(StatusFailed), string(StatusCancelled):
		return true
	}
	return false
}

// Actionability is the per-row affordance projection returned to the admin UI.
//
// It is a DOMAIN fact ("is this action legal for this row's status?") and is
// deliberately separate from AUTHORIZATION ("may this actor take it at this
// facility?"). The handler combines the two; this struct only carries the first.
type Actionability struct {
	CanRetry  bool `json:"can_retry"`
	CanCancel bool `json:"can_cancel"`
}

// ActionabilityForStatus projects the status rules for one row.
//
// This is status-only by design. Permission and facility provenance are applied
// by the handler, because they depend on the actor and this function cannot see
// either. Keeping the two apart is what stops `can_retry` from becoming a
// permission signal: a row that is retryable by status but sits at a facility
// where the actor lacks notification.manage is still reported as
// NOT actionable by the handler.
func ActionabilityForStatus(status string) Actionability {
	return Actionability{
		CanRetry:  CanRetryStatus(status),
		CanCancel: CanCancelStatus(status),
	}
}
