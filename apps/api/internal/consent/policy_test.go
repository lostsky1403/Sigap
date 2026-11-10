package consent

import (
	"testing"
	"time"
)

// Synthetic fixture identifiers. These are fictional and non-routable,
// paralleling the interop contracts' ".pilotsari.example" placeholders; they
// are never real patients, real NIKs, or real facilities.
const (
	fixturePatientID = "p-synthetic-0001"
	fixtureFacilityA = "F-1" // RSUD Pilotsari (fictional)
	fixtureFacilityB = "F-2" // Puskesmas Pilotsari Kota (fictional)
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
		Purpose:  PurposeTreatment,
		Scope:    Scope{PatientID: fixturePatientID, FacilityID: fixtureFacilityA},
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
		ReadPurpose: PurposeTreatment,
		FacilityID:  fixtureFacilityA,
		PatientID:   fixturePatientID,
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
			name: "valid consume grant is allowed",
			req:  validRequest(),
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: true, Reason: ReasonAllowed},
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
			req:  Request{ReadPurpose: PurposeResearch, FacilityID: fixtureFacilityA, PatientID: fixturePatientID},
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonPurposeNotConsented},
		},
		{
			name: "wrong facility is denied",
			req:  Request{ReadPurpose: PurposeTreatment, FacilityID: fixtureFacilityB, PatientID: fixturePatientID},
			g:    validGrant(),
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonFacilityNotInScope},
		},
		{
			name: "emergency granted when policy permits break-glass",
			req:  Request{ReadPurpose: PurposeEmergency, FacilityID: fixtureFacilityB, PatientID: fixturePatientID, Emergency: true},
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
			req:  Request{ReadPurpose: PurposeEmergency, FacilityID: fixtureFacilityB, PatientID: fixturePatientID, Emergency: true},
			g:    validGrant(), // EmergencyPolicy zero value => Allowed == false
			at:   atNow(),
			want: Decision{Allowed: false, Reason: ReasonEmergencyNotPermitted},
		},
		{
			name: "emergency denied when grant is revoked even with policy",
			req:  Request{ReadPurpose: PurposeEmergency, FacilityID: fixtureFacilityB, PatientID: fixturePatientID, Emergency: true},
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
			if got != tc.want {
				t.Fatalf("Evaluate() = %+v, want %+v", got, tc.want)
			}
		})
	}
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