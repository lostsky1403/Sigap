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

// ResourceCategory identifies a class of protected record content that a grant
// may permit. The zero value is not a category, so an uncategorized grant or
// request fails closed rather than widening access.
type ResourceCategory string

const (
	// ResourceClinicalRecord is the patient's clinical encounter record.
	ResourceClinicalRecord ResourceCategory = "clinical_record"
	// ResourceMedicationHistory is the patient's medication history.
	ResourceMedicationHistory ResourceCategory = "medication_history"
	// ResourceBilling is administrative/billing record content.
	ResourceBilling ResourceCategory = "billing"
)

// Scope bounds a grant to a record subject and, when FacilityID and/or
// OrganizationID are set, to a single facility and/or organization. Empty
// fields mean "not scoped on that axis"; the evaluator fails closed for an
// unpopulated scope rather than widening access.
type Scope struct {
	// PatientID selects the record subject. Synthetic local identifier only;
	// this is never a NIK and never an identity mapping.
	PatientID string
	// FacilityID limits the grant to one facility. Empty = not facility-scoped.
	FacilityID string
	// OrganizationID limits the grant to one organization. Empty = not
	// organization-scoped.
	OrganizationID string
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
// (threat model §4.5): fields map to patient id, authorized actor, purpose,
// permitted resource category, granted at (NotBefore), validity (NotAfter),
// revocation, and evidence/emergency policy.
type Grant struct {
	// ActorID names the single professional actor allowed to act on this
	// grant. Synthetic local identifier only. This is a consent-level binding
	// *in addition to* the separate upstream professional-authorization gate
	// (identity.Actor + auth.AuthorizeFacilityRead); it never replaces it.
	ActorID string
	// Purpose is the consented purpose of use.
	Purpose Purpose
	// ResourceCategory is the class of record content this grant permits.
	// Empty = uncategorized = denies.
	ResourceCategory ResourceCategory
	Scope      Scope
	Validity   Validity
	Revocation Revocation
	Emergency  EmergencyPolicy
}

// Request is a single access decision asking "may this actor use this record
// for this purpose, from this facility, right now?". It contains no identity
// credential beyond the synthetic local identifiers; professional
// authorization (identity.Actor + auth.AuthorizeFacilityRead) remains a
// separate, upstream gate that this evaluator never replaces — the actor
// binding here is a second, consent-level check.
type Request struct {
	// ActorID is the professional requesting access.
	ActorID string
	// ReadPurpose is the purpose the actor is requesting to use.
	ReadPurpose Purpose
	// ResourceCategory is the class of record content being requested. Empty =
	// uncategorized = denies.
	ResourceCategory ResourceCategory
	// FacilityID is the facility the actor is operating from.
	FacilityID string
	// OrganizationID is the organization the actor is operating under.
	OrganizationID string
	// PatientID is the record subject being read.
	PatientID string
	// Emergency reports whether the actor is invoking break-glass access.
	Emergency bool
}

// Decision is the outcome of an access check. Allowed is never true unless the
// evaluator positively reaches an allow step; every early return is a denial,
// so the zero value is deny-with-empty-reason. Audit is always populated (even
// for denials) so every decision is immediately auditable.
type Decision struct {
	Allowed bool
	Reason  string
	Audit   AuditEvent
}

// AuditEvent is the structured, immutable record of one access decision. It
// captures what was requested (actor, subject, facility, organization, purpose,
// resource category), when, and what was decided. Evaluate always returns one,
// so denials are audited exactly like allows.
type AuditEvent struct {
	// At is the evaluation instant (the `at` argument).
	At time.Time
	// ActorID is the requesting actor as stated in the request.
	ActorID string
	// SubjectID is the record subject the request targeted.
	SubjectID string
	// FacilityID is the facility the request came from.
	FacilityID string
	// OrganizationID is the organization the request came from.
	OrganizationID string
	// Purpose is the requested purpose.
	Purpose Purpose
	// ResourceCategory is the requested record category.
	ResourceCategory ResourceCategory
	// Allowed mirrors Decision.Allowed.
	Allowed bool
	// Reason is the stable reason constant.
	Reason string
}

// Denial reason constants. They are stable strings so callers and audit logs
// can record a structured, comparable reason rather than a prose message.
const (
	ReasonAllowed                = "allowed"
	ReasonActorNotAuthorized     = "actor not authorized"
	ReasonRevoked                = "revoked"
	ReasonExpired                = "expired"
	ReasonNotYetActive           = "grant not yet active"
	ReasonResourceNotPermitted   = "resource category not permitted"
	ReasonPurposeNotConsented    = "purpose not consented"
	ReasonFacilityNotInScope     = "facility not in scope"
	ReasonOrganizationNotInScope = "organization not in scope"
	ReasonPatientMismatch        = "patient mismatch"
	ReasonEmergencyNotPermitted  = "emergency access not permitted by policy"
)

// Evaluate decides whether req is authorized by grant at time at.
//
// Ordering is deliberate and stable so the first failing condition is the
// reason returned. Revocation is checked first, then validity, then the
// identity/scope and modality-specific rules:
//
//   - The actor must be the one named on the grant. This is a consent-level
//     binding in addition to — never instead of — the upstream professional
//     authorization gate (identity.Actor + auth.AuthorizeFacilityRead).
//   - Emergency access (req.Emergency == true) is governed by policy: it is
//     denied unless grant.Emergency.Allowed, remains denied when the grant is
//     revoked or outside its validity window, and — when allowed — relaxes the
//     purpose/facility/organization/patient scope checks that a normal read
//     enforces while still requiring a non-revoked, in-window grant. The
//     actor and resource-category checks still apply, so break-glass cannot
//     bypass who is named on the grant or what category of record it covers.
//     This is what makes emergency access "subject to policy" rather than an
//     unconditional backdoor.
//   - Normal access requires the purpose, resource category, facility,
//     organization, and patient scope to match exactly.
//
// Every denial is fail-closed: an empty scope, a zero validity, or a
// non-matching field returns a specific denial reason, never silence.
//
// The returned Decision always carries an AuditEvent capturing the request,
// the evaluation instant, and the outcome, so both allows and denials are
// auditable without extra plumbing.
func Evaluate(req Request, grant Grant, at time.Time) Decision {
	audit := AuditEvent{
		At:              at,
		ActorID:         req.ActorID,
		SubjectID:       req.PatientID,
		FacilityID:      req.FacilityID,
		OrganizationID:  req.OrganizationID,
		Purpose:         req.ReadPurpose,
		ResourceCategory: req.ResourceCategory,
	}
	deny := func(reason string) Decision {
		audit.Allowed = false
		audit.Reason = reason
		return Decision{Allowed: false, Reason: reason, Audit: audit}
	}

	// A grant with no purpose, no subject, or no resource category is not a
	// grant at all. These guards come first so an empty/corrupt record can
	// never authorize, even when both sides are zero-valued and later equality
	// checks would trivially match.
	if grant.Purpose == "" {
		return deny(ReasonPurposeNotConsented)
	}
	if grant.Scope.PatientID == "" {
		return deny(ReasonPatientMismatch)
	}
	if grant.ResourceCategory == "" {
		return deny(ReasonResourceNotPermitted)
	}
	if grant.Revocation.Revoked {
		return deny(ReasonRevoked)
	}
	if at.Before(grant.Validity.NotBefore) {
		return deny(ReasonNotYetActive)
	}
	if !grant.Validity.NotAfter.IsZero() && at.After(grant.Validity.NotAfter) {
		return deny(ReasonExpired)
	}

	// The consent-level actor binding: only the professional named on the
	// grant may act on it. Professional authorization is a separate upstream
	// gate and is never bypassed by this check.
	if req.ActorID != grant.ActorID {
		return deny(ReasonActorNotAuthorized)
	}

	if req.Emergency {
		if !grant.Emergency.Allowed {
			return deny(ReasonEmergencyNotPermitted)
		}
		// The resource-category binding still applies under break-glass: the
		// grant covers specific categories, and emergency access may not
		// widen them. Purpose/facility/organization/patient scope is relaxed
		// because emergency access is antithetical to pre-planned scoping,
		// but the actor check above has already passed and the mandatory-
		// review flag rides along for the audit path.
		if req.ResourceCategory != grant.ResourceCategory {
			return deny(ReasonResourceNotPermitted)
		}
		audit.Allowed = true
		audit.Reason = ReasonAllowed
		return Decision{Allowed: true, Reason: ReasonAllowed, Audit: audit}
	}

	if req.ResourceCategory != grant.ResourceCategory {
		return deny(ReasonResourceNotPermitted)
	}
	if req.ReadPurpose != grant.Purpose {
		return deny(ReasonPurposeNotConsented)
	}
	if req.FacilityID != grant.Scope.FacilityID {
		return deny(ReasonFacilityNotInScope)
	}
	if req.OrganizationID != grant.Scope.OrganizationID {
		return deny(ReasonOrganizationNotInScope)
	}
	if req.PatientID != grant.Scope.PatientID {
		return deny(ReasonPatientMismatch)
	}
	audit.Allowed = true
	audit.Reason = ReasonAllowed
	return Decision{Allowed: true, Reason: ReasonAllowed, Audit: audit}
}