package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestFakeQueueService_TicketIDIsUUID guards the contract the check-in handler
// depends on: TicketID is written into uuid columns, so a synthetic string
// ("ticket-1") is rejected by the database and strands the appointment.
func TestFakeQueueService_TicketIDIsUUID(t *testing.T) {
	svc := NewFakeQueueService()

	res, err := svc.Generate(context.Background(), GenerateInput{
		FacilityID: "00000000-0000-0000-0000-000000000001",
		Patient:    PatientInput{FullName: "Budi Santoso", Phone: "081234567890"},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := uuid.Parse(res.TicketID); err != nil {
		t.Fatalf("expected a UUID ticket id, got %q: %v", res.TicketID, err)
	}
}
