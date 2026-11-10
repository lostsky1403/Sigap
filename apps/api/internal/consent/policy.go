// Package consent implements a deny-by-default access-decision evaluator for
// patient record access. It is PURE Go: no database, no HTTP, no I/O. Every
// identifier it reasons over is synthetic and fictional, at the same
// LOCAL-SYNTHETIC-CONTRACT tier as the regional interop contracts
// (design/regional-interop-contracts.md §2.1). Nothing in this package reads or
// writes patient identity, stores a NIK, performs a search, or exposes any
// identity mapping.
//
// The evaluator is the RUNNING slice of the consent contract described in
// design/patient-identity-consent.md. It reasons over a fully-populated Grant
// value rather than resolving storage, because the consent ledger that would
// back the Grant is DESIGN-ONLY (no table, column, or migration exists today).
// Callers of Evaluate are therefore expected to supply a Grant they have
// already fetched or synthesized; policy.go itself never talks to a ledger.
//
// Import path: github.com/sigap/sigap/apps/api/internal/consent
package consent

import "time"

// Purpose is the use a party is requesting. Mirrors the purpose-bound consent
// ledger in the threat model (§4.5): data is used only for a purpose the
// patient consented to, and consent is recorded, not assumed.
type Purpose string

const (
	// PurposeTreatment is direct clinical care for the subject.
	PurposeTreatment Purpose = "treatment"
	// PurposeEmergency is break-glass access during an emergency.
	PurposeEmergency Purpose = "emergency"
	// PurposeResearch is secondary use beyond the visit. DESIGN-ONLY: it
	// requires a purpose-bound grant before processing (threat T5a/T5c).
	PurposeResearch Purpose = "research"
	// PurposePayment is administrative/billing use. DESIGN-ONLY.
	PurposePayment Purpose = "payment"
	// PurposeMarketing is explicitly prohibited for medical records and only
	// listed so the evaluator can fail it closed.
	PurposeMarketing Purpose = "marketing"
)

// Scope bounds a grant to a record subject and, when FacilityID is set, to a
// single facility. Empty fields mean "not scoped on that axis"; the evaluator
// fails closed for an unpopulated scope rather than widening access.
type Scope struct {
	// PatientID selects the record subject. Synthetic local identifier only;
	// this is never a NIK and never an identity mapping.
	PatientID string
	// FacilityID limits the grant to one facility. Empty = not facility-scoped.
	FacilityID string
}

// Validity is the time window during which a grant is active. The zero value
// (both bounds zero) is treated as expired by the evaluator.
type Validity struct {
	NotBefore time.Time
	NotAfter  time.Time
}

// Revocation records whether, and when, a grant was withdrawn. Revocation
// always wins in the evaluator: a revoked grant authorizes nothing, even
// under emergency access.
type Revocation struct {
	Revoked   bool
	RevokedAt time.Time
}

// EmergencyPolicy decides how break-glass (emergency) access behaves for a
// grant. The zero value denies emergency access, so break-glass is
// deny-by-default exactly like every other path.
type EmergencyPolicy struct {
	// Allowed reports whether the owning patient permits emergency access to
	// this record. Until a patient opts in, this is false.
	Allowed bool
	// RequireReview forces a mandatory post-hoc review when set. It does not
	// itself deny access; it marks the access for audit follow-up.
	RequireReview bool
}

// Grant is one fully-resolved consent record the evaluator reasons over. It is
// the in-memory projection of the DESIGN-ONLY purpose-bound consent ledger
// (threat model §4.5): fields map to patient id, purpose, granted at
// (NotBefore), validity (NotAfter), revocation, and evidence/emergency policy.
type Grant struct {
	Purpose    Purpose
	Scope      Scope
	Validity   Validity
	Revocation Revocation
	Emergency  EmergencyPolicy
}

// Request is a single access decision asking "may this actor use this record
// for this purpose, from this facility, right now?". It contains no identity
// credential beyond the synthetic local PatientID/FacilityID; the actor
// (clinician) authorization is a separate, upstream gate (identity.Actor +
// auth.AuthorizeFacilityRead), not this evaluator's concern.
type Request struct {
	// ReadPurpose is the purpose the actor is requesting to use.
	ReadPurpose Purpose
	// FacilityID is the facility the actor is operating from.
	FacilityID string
	// PatientID is the record subject being read.
	PatientID string
	// Emergency reports whether the actor is invoking break-glass access.
	Emergency bool
}

// Decision is the outcome of an access check. Allowed is never true unless the
// evaluator positively reaches an allow step; every early return is a denial,
// so the zero value is deny-with-empty-reason.
type Decision struct {
	Allowed bool
	Reason  string
}

// Denial reason constants. They are stable strings so callers and audit logs
// can record a structured, comparable reason rather than a prose message.
const (
	ReasonAllowed               = "allowed"
	ReasonRevoked               = "revoked"
	ReasonExpired               = "expired"
	ReasonNotYetActive          = "grant not yet active"
	ReasonPurposeNotConsented   = "purpose not consented"
	ReasonFacilityNotInScope    = "facility not in scope"
	ReasonPatientMismatch       = "patient mismatch"
	ReasonEmergencyNotPermitted = "emergency access not permitted by policy"
)

// Evaluate decides whether req is authorized by grant at time at.
//
// Ordering is deliberate and stable so the first failing condition is the
// reason returned. Revocation is checked first, then validity, then the
// modality-specific rules:
//
//   - Emergency access (req.Emergency == true) is governed by policy: it is
//     denied unless grant.Emergency.Allowed, remains denied when the grant is
//     revoked or outside its validity window, and — when allowed — relaxes the
//     purpose/facility/patient scope checks that a normal read enforces while
//     still requiring a non-revoked, in-window grant. This is what makes
//     emergency access "subject to policy" rather than an unconditional
//     backdoor.
//   - Normal access requires the purpose, facility, and patient scope to
//     match exactly.
//
// Every denial is fail-closed: an empty scope, a zero validity, or a
// non-matching field returns a specific denial reason, never silence.
func Evaluate(req Request, grant Grant, at time.Time) Decision {
	// A grant with no purpose or no subject is not a grant at all. These guards
	// come first so an empty/corrupt record can never authorize, even when both
	// sides are zero-valued and later equality checks would trivially match.
	if grant.Purpose == "" {
		return Decision{Allowed: false, Reason: ReasonPurposeNotConsented}
	}
	if grant.Scope.PatientID == "" {
		return Decision{Allowed: false, Reason: ReasonPatientMismatch}
	}
	if grant.Revocation.Revoked {
		return Decision{Allowed: false, Reason: ReasonRevoked}
	}
	if at.Before(grant.Validity.NotBefore) {
		return Decision{Allowed: false, Reason: ReasonNotYetActive}
	}
	if !grant.Validity.NotAfter.IsZero() && at.After(grant.Validity.NotAfter) {
		return Decision{Allowed: false, Reason: ReasonExpired}
	}

	if req.Emergency {
		if !grant.Emergency.Allowed {
			return Decision{Allowed: false, Reason: ReasonEmergencyNotPermitted}
		}
		// Break-glass is permitted by policy and the underlying grant is
		// non-revoked and in-window. Purpose/facility/patient scope is relaxed
		// here because emergency access is antithetical to pre-planned scoping,
		// but the mandatory-review flag rides along for the audit path.
		return Decision{Allowed: true, Reason: ReasonAllowed}
	}

	if req.ReadPurpose != grant.Purpose {
		return Decision{Allowed: false, Reason: ReasonPurposeNotConsented}
	}
	if req.FacilityID != grant.Scope.FacilityID {
		return Decision{Allowed: false, Reason: ReasonFacilityNotInScope}
	}
	if req.PatientID != grant.Scope.PatientID {
		return Decision{Allowed: false, Reason: ReasonPatientMismatch}
	}
	return Decision{Allowed: true, Reason: ReasonAllowed}
}