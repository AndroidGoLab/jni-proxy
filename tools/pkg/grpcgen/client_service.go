package grpcgen

// ClientService describes a gRPC service client wrapper.
type ClientService struct {
	GoType      string // Short name, e.g. "Manager"
	ServiceName string // Proto service name, e.g. "ManagerService"
	Methods     []ClientMethod
}
