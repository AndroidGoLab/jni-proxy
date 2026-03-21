package grpcgen

// ServerService describes a gRPC service backed by a javagen-generated class.
type ServerService struct {
	GoType       string // javagen type name, e.g. "Manager", "Adapter"
	ServiceName  string // proto service name, e.g. "ManagerService"
	Obtain       string // how the manager is obtained: "system_service", etc.
	Close        bool   // whether manager has Close()
	Methods      []ServerMethod
	NeedsHandles bool // whether any method uses object handles
}
