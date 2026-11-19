package consent

import (
	"testing"
	"time"
)

// Synthetic fixture identifiers. These are fictional and non-routable,
// paralleling the interop contracts' ".pilotsari.example" placeholders; they
// are never real patients, real NIKs, or real facilities.
const (
	fixturePatientID     = "p-synthetic-0001"
	fixtureActorA        = "actor-synthetic-0001" // dr. Sari (fictional)
	fixtureActorB        = "actor-synthetic-0002" // dr. Bayu (fictional)
	fixtureFacilityA     = "F-1"                  // RSUD Pilotsari (fictional)
	fixtureFacilityB     = "F-2"                  // Puskesmas Pilotsari Kota (fictional)
	fixtureOrganizationA = "O-1"                  // Dinas Kesehatan Kab. Pilotsari (fictional)
	fixtureOrganizationB = "O-2"                  // Yayasan Sehat Nusantara (fictional)
)

// validWindow returns a validity window covering the test "now".
func validWindow() Validity {
	return Validity{
		NotBefore: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
	}
}

func validGrant() Grant {
	return Grant{
		ActorID:        fixtureActorA,
		Purpose:        PurposeTreatment,
		ResourceCategory: ResourceClinicalRecord,
		Scope: Scope{
			PatientID:     fixturePatientID,
			FacilityID:    fixtureFacilityA,
			OrganizationID: fixtureOrganizationA,
		},
		Validity: validWindow(),
	}
}

// atNow is the wall-clock instant every case evaluates against, chosen inside
// the valid window.
func atNow() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

// validRequest is the normal read request that validGrant should authorize.
func validRequest() Request {
	return Request{
		ActorID:         fixtureActorA,
		ReadPurpose:     PurposeTreatment,
		ResourceCategory: ResourceClinicalRecord,
		FacilityID:      fixtureFacilityA,
		OrganizationID:  fixtureOrganizationA,
		PatientID:       fixturePatientID,
	}
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		g    Grant
		at   time.Time
		want Decision
	}{
		{
			name: "valid treatment grant is allowed",
			req:  validRequest(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: true, Reason: ReasonAllowed},
		},
		{
			name: "wrong actor is denied",
			req: func() Request {
				req := validRequest()
				req.ActorID = fixtureActorB
				return req
			}(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonActorNotAuthorized},
		},
		{
			name: "out-of-scope facility is denied",
			req: func() Request {
				req := validRequest()
				req.FacilityID = fixtureFacilityB
				return req
			}(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonFacilityNotInScope},
		},
		{
			name: "out-of-scope organization is denied",
			req: func() Request {
				req := validRequest()
				req.OrganizationID = fixtureOrganizationB
				return req
			}(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonOrganizationNotInScope},
		},
		{
			name: "uncategorized resource is denied",
			req: func() Request {
				req := validRequest()
				req.ResourceCategory = ""
				return req
			}(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonResourceNotPermitted},
		},
		{
			name: "unpermitted resource category is denied",
			req: func() Request {
				req := validRequest()
				req.ResourceCategory = ResourceMedicationHistory
				return req
			}(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonResourceNotPermitted},
		},
		{
			name: "expired grant is denied",
			req:  validRequest(),
			g:    validGrant(),
			at:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			want: Decision{Allowed: false, Reason: ReasonExpired},
		},
		{
			name: "revoked grant is denied",
			req:  validRequest(),
			g: func() Grant {
				g := validGrant()
				g.Revocation = Revocation{
					Revoked:   true,
					RevokedAt: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
				}
				return g
			}(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonRevoked},
		},
		{
			name: "out-of-scope purpose is denied",
			req: func() Request {
				req := validRequest()
				req.ReadPurpose = PurposeResearch
				return req
			}(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonPurposeNotConsented},
		},
		{
			name: "wrong patient is denied",
			req: func() Request {
				req := validRequest()
				req.PatientID = "p-synthetic-9999"
				return req
			}(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonPatientMismatch},
		},
		{
			name: "emergency granted when policy permits break-glass",
			req: func() Request {
				req := validRequest()
				req.Emergency = true
				req.FacilityID = fixtureFacilityB
				req.OrganizationID = fixtureOrganizationB
				req.ReadPurpose = PurposeEmergency
				return req
			}(),
			g: func() Grant {
				g := validGrant()
				g.Emergency = EmergencyPolicy{Allowed: true, RequireReview: true}
				return g
			}(),
			at:   atNow(),
			want: Decision{Allowed: true, Reason: ReasonAllowed},
		},
		{
			name: "emergency denied when policy does not permit break-glass",
			req: func() Request {
				req := validRequest()
				req.Emergency = true
				req.FacilityID = fixtureFacilityB
				req.OrganizationID = fixtureOrganizationB
				req.ReadPurpose = PurposeEmergency
				return req
			}(),
			g:    validGrant(), // EmergencyPolicy zero value => Allowed == false
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonEmergencyNotPermitted},
		},
		{
			name: "emergency denied when actor not named on grant",
			req: func() Request {
				req := validRequest()
				req.ActorID = fixtureActorB
				req.Emergency = true
				req.FacilityID = fixtureFacilityB
				req.OrganizationID = fixtureOrganizationB
				req.ReadPurpose = PurposeEmergency
				return req
			}(),
			g: func() Grant {
				g := validGrant()
				g.Emergency = EmergencyPolicy{Allowed: true, RequireReview: true}
				return g
			}(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonActorNotAuthorized},
		},
		{
			name: "emergency denied when grant is revoked even with policy",
			req: func() Request {
				req := validRequest()
				req.Emergency = true
				req.FacilityID = fixtureFacilityB
				req.OrganizationID = fixtureOrganizationB
				req.ReadPurpose = PurposeEmergency
				return req
			}(),
			g: func() Grant {
				g := validGrant()
				g.Emergency = EmergencyPolicy{Allowed: true, RequireReview: true}
				g.Revocation = Revocation{Revoked: true, RevokedAt: atNow().Add(-time.Hour)}
				return g
			}(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonRevoked},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.req, tc.g, tc.at)
			if got.Allowed != tc.want.Allowed || got.Reason != tc.want.Reason {
				t.Fatalf("Evaluate() = {Allowed %v, Reason %q}, want {Allowed %v, Reason %q}",
					got.Allowed, got.Reason, tc.want.Allowed, tc.want.Reason)
			}
		})
	}
}

// TestEvaluateAudited verifies that every decision — allow or deny — carries a
// complete AuditEvent: the request (actor, subject, facility, organization,
// purpose, resource category), the evaluation instant, and the outcome.
func TestEvaluateAudited(t *testing.T) {
	t.Run("allow records full audit", func(t *testing.T) {
		got := Evaluate(validRequest(), validGrant(), atNow())
		if !got.Allowed {
			t.Fatalf("expected allow, got %+v", got)
		}
		want := AuditEvent{
			At:              atNow(),
			ActorID:         fixtureActorA,
			SubjectID:       fixturePatientID,
			FacilityID:      fixtureFacilityA,
			OrganizationID:  fixtureOrganizationA,
			Purpose:         PurposeTreatment,
			ResourceCategory: ResourceClinicalRecord,
			Allowed:         true,
			Reason:          ReasonAllowed,
		}
		if got.Audit != want {
			t.Fatalf("audit = %+v, want %+v", got.Audit, want)
		}
	})

	t.Run("deny records full audit", func(t *testing.T) {
		req := validRequest()
		req.ActorID = fixtureActorB
		got := Evaluate(req, validGrant(), atNow())
		if got.Allowed {
			t.Fatalf("expected deny, got %+v", got)
		}
		if got.Audit.ActorID != fixtureActorB {
			t.Fatalf("audit actor = %q, want %q", got.Audit.ActorID, fixtureActorB)
		}
		if got.Audit.Allowed {
			t.Fatalf("audit Allowed = true for a denial")
		}
		if got.Audit.Reason != ReasonActorNotAuthorized {
			t.Fatalf("audit reason = %q, want %q", got.Audit.Reason, ReasonActorNotAuthorized)
		}
		if got.Audit.SubjectID != fixturePatientID || got.Audit.FacilityID != fixtureFacilityA ||
			got.Audit.OrganizationID != fixtureOrganizationA || got.Audit.Purpose != PurposeTreatment ||
			got.Audit.ResourceCategory != ResourceClinicalRecord {
			t.Fatalf("denial audit lost request context: %+v", got.Audit)
		}
		if got.Audit.At != atNow() {
			t.Fatalf("audit At = %v, want %v", got.Audit.At, atNow())
		}
	})
}

// TestEvaluateDenyByDefault guards the fail-closed invariant: a zero Grant and
// a zero Request must deny, so a missing/corrupt record never authorizes.
func TestEvaluateDenyByDefault(t *testing.T) {
	got := Evaluate(Request{}, Grant{}, atNow())
	if got.Allowed {
		t.Fatalf("zero request/grant must deny, got allow: %+v", got)
	}
	// The empty-purpose guard fires before any other check, so the deterministic
	// reason for a fully zero grant is "purpose not consented".
	if got.Reason != ReasonPurposeNotConsented {
		t.Fatalf("unexpected zero-value reason %q", got.Reason)
	}
}