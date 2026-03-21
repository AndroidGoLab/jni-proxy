package grpcgen

// ServerData holds all information needed to render a gRPC server file.
type ServerData struct {
	Package      string // Go package name, e.g. "location"
	GoModule     string // Go module path for proxy code, e.g. "github.com/AndroidGoLab/jni-proxy"
	JniModule    string // Go module path for the JNI bindings, e.g. "github.com/AndroidGoLab/jni"
	GoImport     string // Go import of the javagen package, e.g. "github.com/AndroidGoLab/jni/location"
	NeedsJNI     bool   // Whether the generated code needs to import the jni package
	NeedsHandles bool   // Whether the generated code needs to import the grpcserver handles package
	Services     []ServerService
	DataClasses  []ServerDataClass
}
