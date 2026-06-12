package grpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/sigap/sigap/apps/api/internal/pb/queueenginepb"

	"github.com/sigap/sigap/apps/api/internal/service"
)

// grpcQueueService is the real implementation of QueueService that talks to the
// Rust engine over gRPC. It provides full "micro-second traceability" by
// surfacing the processing_time from the engine.
type grpcQueueService struct {
	client pb.QueueEngineClient
}

// NewGRPCQueueService dials the Rust engine (e.g. "localhost:50051" or from env).
// For scaffold we use insecure; production should use TLS + auth.
func NewGRPCQueueService(addr string) (service.QueueService, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial rust engine at %s: %w", addr, err)
	}
	// NOTE: conn is intentionally not closed here for scaffold simplicity.
	// In real deployment use a managed connection or grpc.NewClient with proper lifecycle.
	return &grpcQueueService{client: pb.NewQueueEngineClient(conn)}, nil
}

func (g *grpcQueueService) Generate(ctx context.Context, input service.GenerateInput) (service.GenerateResult, error) {
	req := &pb.GenerateQueueRequest{
		FacilityId: input.FacilityID,
		Patient: &pb.PatientInfo{
			FullName: input.Patient.FullName,
			Phone:    input.Patient.Phone,
		},
	}

	// Handle optional fields (proto3 optional -> *string in Go)
	if input.Patient.Gender != "" {
		g := input.Patient.Gender
		req.Patient.Gender = &g
	}
	if input.Patient.DateOfBirth != "" {
		d := input.Patient.DateOfBirth
		req.Patient.DateOfBirth = &d
	}

	resp, err := g.client.GenerateQueueNumber(ctx, req)
	if err != nil {
		// The Rust engine returns clean tonic::Status; Go caller (handler) can map status if needed.
		return service.GenerateResult{}, fmt.Errorf("rust engine error: %w", err)
	}

	return service.GenerateResult{
		TicketID:             resp.TicketId,
		FormattedNumber:      resp.FormattedNumber,
		Status:               resp.Status,
		RegisteredAt:         resp.RegisteredAt,
		EstimatedWaitMinutes: int(resp.EstimatedWaitMinutes),
		ProcessingTime:       fmt.Sprintf("%dµs", resp.ProcessingTimeMicros),
		Signature:            resp.Signature,
	}, nil
}
