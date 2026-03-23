package grpcgen

// ServerService describes a gRPC service backed by a javagen-generated class.
type ServerService struct {
	GoType       string // javagen type name, e.g. "Manager", "Adapter"
	ServiceName  string // proto service name, e.g. "ManagerService"
	GoImport     string // Go import path for the javagen package, e.g. "github.com/AndroidGoLab/jni/location"
	ImportAlias  string // import alias for the javagen package, e.g. "jnipkg" or "jnipkg2"
	Obtain       string // how the manager is obtained: "system_service", etc.
	Close        bool   // whether manager has Close()
	Methods      []ServerMethod
	NeedsHandles bool // whether any method uses object handles
}
