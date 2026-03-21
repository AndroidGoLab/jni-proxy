package server

import (
	"context"
	"strings"

	"github.com/AndroidGoLab/jni-proxy/grpc/server/acl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// ACLAuth checks client identity from mTLS peer cert and verifies
// method permissions against the ACL store.
type ACLAuth struct {
	Store *acl.Store
}

// Authorize extracts the client CN from the TLS peer certificate and
// checks the ACL store for a matching method grant. The Register RPC
// is always allowed (unauthenticated enrollment), and all AuthService
// RPCs are allowed for any authenticated client.
func (a ACLAuth) Authorize(ctx context.Context, fullMethod string) error {
	// Register RPC is always allowed (unauthenticated enrollment).
	if fullMethod == "/auth.AuthService/Register" {
		return nil
	}

	// Extract client CN from TLS peer cert.
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "no peer info")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
		return status.Error(codes.Unauthenticated, "no client certificate")
	}
	clientID := tlsInfo.State.PeerCertificates[0].Subject.CommonName

	// AuthService RPCs are allowed for any authenticated client.
	if strings.HasPrefix(fullMethod, "/auth.AuthService/") {
		return nil
	}

	// Check ACL store.
	allowed, err := a.Store.IsMethodAllowed(clientID, fullMethod)
	if err != nil {
		return status.Errorf(codes.Internal, "acl check: %v", err)
	}
	if !allowed {
		return status.Errorf(codes.PermissionDenied,
			"client %q not authorized for %s", clientID, fullMethod)
	}
	return nil
}

// UnaryAuthInterceptor returns a gRPC unary interceptor that checks
// authorization before handling each request.
func UnaryAuthInterceptor(auth Authorizer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := auth.Authorize(ctx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor returns a gRPC stream interceptor that checks
// authorization before handling each stream.
func StreamAuthInterceptor(auth Authorizer) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := auth.Authorize(ss.Context(), info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}
