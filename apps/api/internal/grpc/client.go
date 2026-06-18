package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/sigap/sigap/apps/api/internal/pb/queueenginepb"

	"github.com/sigap/sigap/apps/api/internal/service"
)

// grpcQueueService is the real implementation of QueueService that talks to the
// Rust engine over gRPC. It provides full "micro-second traceability" by
// surfacing the processing_time from the engine.
type grpcQueueService struct {
	client pb.QueueEngineClient
	conn   *grpc.ClientConn
}

// NewGRPCQueueService dials the Rust engine (e.g. "localhost:50051" or from env).
// Supports TLS via SIGAP_GRPC_TLS env var; defaults to insecure for local dev
// with a loud warning. Production MUST set SIGAP_GRPC_TLS=true and provide certs.
func NewGRPCQueueService(addr string) (service.QueueService, error) {
	var creds credentials.TransportCredentials
	if os.Getenv("SIGAP_GRPC_TLS") == "true" {
		// Production TLS: load system CA certs or custom cert paths.
		// TODO: add client cert + CA path flags when mTLS is wired.
		var err error
		creds, err = credentials.NewClientTLSFromFile("/etc/sigap/certs/ca.crt", "")
		if err != nil {
			return nil, fmt.Errorf("load TLS credentials: %w", err)
		}
	} else {
		slog.Warn("gRPC using insecure transport (dev only); set SIGAP_GRPC_TLS=true in production",
			"addr", addr)
		creds = insecure.NewCredentials()
	}
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial rust engine at %s: %w", addr, err)
	}
	// NOTE: conn is intentionally not closed here for scaffold simplicity.
	// In real deployment use a managed connection or grpc.NewClient with proper lifecycle.
	return &grpcQueueService{client: pb.NewQueueEngineClient(conn), conn: conn}, nil
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

// Probe checks gRPC connectivity by verifying the connection state and
// performing a lightweight grpc.Ping (if supported) or state check.
func (g *grpcQueueService) Probe(ctx context.Context) error {
	if g.conn == nil {
		return fmt.Errorf("grpc connection not initialized")
	}
	// Short timeout probe; if connection state is Ready, treat as healthy.
	state := g.conn.GetState()
	if state == connectivity.Ready {
		return nil
	}
	// Try to wait for a transient state to resolve within a short window.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if !g.conn.WaitForStateChange(ctx, state) {
		return fmt.Errorf("grpc connection state %s (timeout)", state)
	}
	if g.conn.GetState() != connectivity.Ready {
		return fmt.Errorf("grpc connection state %s", g.conn.GetState())
	}
	return nil
}
