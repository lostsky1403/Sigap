package identity

import (
	"testing"

	"github.com/google/uuid"
)

// citizenAccountID mirrors the synthetic citizen seeded in
// packages/db/seed/regional_tenancy.sql (subject local-patient-p1).
const citizenAccountID = "00000000-0000-0000-0000-00000000c307"

func TestActor_PatientAccountID(t *testing.T) {
	citizen := Actor{
		UserID:    "local-patient-p1",
		Type:      ActorUser,
		AppUserID: citizenAccountID,
	}

	facilityID := uuid.New()

	tests := []struct {
		name  string
		actor Actor
		want  string
	}{
		{"citizen with no staff grants owns their account", citizen, citizenAccountID},
		{"unauthenticated zero actor", Actor{}, ""},
		{"dev actor never owns even with app user id", Actor{UserID: "dev", Type: ActorDev, IsDev: true, AppUserID: citizenAccountID}, ""},
		{"staff with flat permission key fails closed", Actor{UserID: "op", Type: ActorUser, AppUserID: citizenAccountID, Permissions: []string{"appointment.read"}}, ""},
		{"staff with facility grant fails closed", Actor{UserID: "admin", Type: ActorUser, AppUserID: citizenAccountID, FacilityGrants: []FacilityGrant{{Key: "appointment.manage", FacilityID: &facilityID}}}, ""},
		{"zero-scope assignment fails closed", Actor{UserID: "zero", Type: ActorUser, AppUserID: citizenAccountID, Permissions: []string{"facility.read"}, FacilityGrants: []FacilityGrant{{Key: "facility.read"}}}, ""},
		{"authenticated but unmapped subject fails closed", Actor{UserID: "no-map", Type: ActorUser}, ""},
		{"system actor fails closed", Actor{UserID: "svc", Type: ActorSystem, AppUserID: citizenAccountID}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.actor.PatientAccountID(); got != tt.want {
				t.Errorf("PatientAccountID() = %q, want %q", got, tt.want)
			}
		})
	}
}
