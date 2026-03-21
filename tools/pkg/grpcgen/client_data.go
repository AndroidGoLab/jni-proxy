package grpcgen

// ClientData holds all information needed to render a gRPC client file.
type ClientData struct {
	Package     string          // Go package name, e.g. "location"
	GoModule    string          // Go module path, e.g. "github.com/AndroidGoLab/jni"
	Services    []ClientService // One per gRPC service in the package
	DataClasses []ClientDataClass
}
