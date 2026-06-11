package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// GenerateInput matches the public contract (validated at handler boundary).
type GenerateInput struct {
	FacilityID string
	Patient    PatientInput
}

type PatientInput struct {
	FullName    string
	Phone       string
	Gender      string // optional "L" or "P"
	DateOfBirth string // optional YYYY-MM-DD
}

// GenerateResult is the successful response payload (comes from Rust gRPC engine).
type GenerateResult struct {
	TicketID             string
	FormattedNumber      string // e.g. "RSK-0042"
	Status               string
	RegisteredAt         string
	EstimatedWaitMinutes int
	ProcessingTimeMicros int64 // from Rust engine, for micro-second traceability
}

// QueueService is the interface for the core queue generation logic.
// The real implementation will eventually delegate to the Rust engine via gRPC.
// For this scaffold we provide an in-memory fake so tests and the endpoint are self-contained.
type QueueService interface {
	Generate(ctx context.Context, input GenerateInput) (GenerateResult, error)
}

// ErrValidation is returned for bad input (mapped to 400 by handler).
var ErrValidation = errors.New("validation error")

// ErrFacilityNotFound etc. can be added later when real logic + DB is wired.

// fakeQueueService is a minimal in-memory implementation for GREEN tests and initial endpoint.
// It always succeeds for valid input and returns a deterministic fake ticket.
// This keeps the chunk small while proving the handler + rate limit + error mapping.
type fakeQueueService struct {
	counter int
}

func NewFakeQueueService() QueueService {
	return &fakeQueueService{}
}

func (f *fakeQueueService) Generate(ctx context.Context, input GenerateInput) (GenerateResult, error) {
	// Very basic validation (handler already did most, this is defense-in-depth)
	if input.FacilityID == "" || input.Patient.FullName == "" || input.Patient.Phone == "" {
		return GenerateResult{}, ErrValidation
	}

	f.counter++
	now := time.Now().UTC().Format(time.RFC3339)

	return GenerateResult{
		TicketID:             fmt.Sprintf("ticket-%d", f.counter),
		FormattedNumber:      "RSK-0001", // placeholder; real logic in Rust engine later
		Status:               "waiting",
		RegisteredAt:         now,
		EstimatedWaitMinutes: 25,
		ProcessingTimeMicros: 123, // dummy for unit tests; real value from gRPC in prod path
	}, nil
}
