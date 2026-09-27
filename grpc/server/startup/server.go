// Package startup exposes the fixed startup action through the existing authenticated gRPC server.
package startup

import (
	"context"

	pb "github.com/AndroidGoLab/jni-proxy/proto/startup"
	"github.com/AndroidGoLab/jni-proxy/startup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server delegates startup lifecycle to the application controller.
//
// Example: register this service on the server with the normal mTLS and ACL interceptors.
type Server struct {
	pb.UnimplementedStartupServiceServer
	Controller *startup.Controller
}

// Configure persists opt-in and an optional replacement key without returning key material.
//
// Example: Enabled false cancels pending retries without disabling current fixed-performance mode.
func (s *Server) Configure(
	ctx context.Context,
	req *pb.ConfigureRequest,
) (*pb.Status, error) {
	if err := s.Controller.Configure(ctx, req.GetPrivateKeyPem(), req.GetEnabled()); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return s.snapshot(), nil
}

// Apply starts a new asynchronous discovery-and-command attempt using the configured key.
//
// Example: GetStatus subsequently reports the actual shell exit result.
func (s *Server) Apply(
	ctx context.Context,
	_ *pb.ApplyRequest,
) (*pb.Status, error) {
	if err := s.Controller.Apply(ctx); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return s.snapshot(), nil
}

// GetStatus reports configuration and last execution without modifying either.
//
// Example: a waiting state is expected while trusted Wi-Fi or wireless debugging is unavailable.
func (s *Server) GetStatus(
	_ context.Context,
	_ *pb.GetStatusRequest,
) (*pb.Status, error) {
	return s.snapshot(), nil
}

// snapshot maps the credential-free domain status to its public protocol representation.
//
// Example: last_exit_code stays absent until a shell command actually returns an exit frame.
func (s *Server) snapshot() *pb.Status {
	state := s.Controller.Status()
	result := &pb.Status{Enabled: state.Enabled, Configured: state.Configured, State: string(state.State), Generation: state.Generation, LocalAdbPort: uint32(state.Port), Attempts: uint32(state.Attempts), LastRunUnixMilli: state.LastRunUnixMilli, LastError: state.LastError}
	if state.HasResult {
		exitCode := int32(state.LastExitCode)
		result.LastExitCode = &exitCode
	}
	return result
}
