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
	FormattedNumber      string `json:"formatted_number"`
	Status               string
	RegisteredAt         string `json:"registered_at"`
	EstimatedWaitMinutes int    `json:"estimated_wait_minutes"`
	ProcessingTime       string `json:"processing_time"` // e.g. "45µs" from Rust engine micro-second measurement
	Signature            string `json:"signature"`       // SHA-256 hex from Rust engine for immutable Health Wallet proof
}

// QueueService is the interface for the core queue generation logic.
// The real implementation will eventually delegate to the Rust engine via gRPC.
// For this scaffold we provide an in-memory fake so tests and the endpoint are self-contained.
type QueueService interface {
	Generate(ctx context.Context, input GenerateInput) (GenerateResult, error)
	// Probe checks whether the backing service/engine is reachable.
	// Returns nil when healthy; a non-nil error signals unavailability.
	Probe(ctx context.Context) error
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
		FormattedNumber:      "RSK-0001",
		Status:               "waiting",
		RegisteredAt:         now,
		EstimatedWaitMinutes: 25,
		ProcessingTime:       "123µs", // dummy for unit tests; real value comes from Rust gRPC
		Signature:            "a1b2c3d4e5f67890123456789abcdef0123456789abcdef0123456789abcdef01", // demo immutable sig
	}, nil
}

func (f *fakeQueueService) Probe(ctx context.Context) error {
	return nil
}
