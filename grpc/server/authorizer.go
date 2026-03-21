package server

import "context"

// Authorizer checks whether a gRPC call is allowed.
type Authorizer interface {
	Authorize(ctx context.Context, fullMethod string) error
}
