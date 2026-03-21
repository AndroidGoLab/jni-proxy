package grpcgen

// ServerMethod describes a single RPC method implementation.
type ServerMethod struct {
	GoName       string // Go method name on the gRPC service interface, e.g. "GetLastKnownLocation"
	SpecGoName   string // Go method name in the javagen code, e.g. "GetLastKnownLocation" or "getProvidersRaw"
	RequestType  string // Proto request message type, e.g. "GetLastKnownLocationRequest"
	ResponseType string // Proto response message type, e.g. "GetLastKnownLocationResponse"
	CallArgs     string // Pre-rendered Go arguments, e.g. "req.GetProvider()"
	ReturnKind   string // "void", "string", "bool", "object", "primitive", "data_class"
	DataClass    string // If returning a data class, its Go type, e.g. "Location"
	HasError     bool   // Whether the javagen method returns error
	HasResult    bool   // Whether the javagen method returns a non-void value
	GoReturnType string // Go type of the return value, e.g. "bool", "string", "*jni.Object"
	NeedsHandles bool   // Whether any param or return uses object handles
	// Pre-rendered conversion expression for result assignment in proto response.
	ResultExpr string // e.g. "result", "int32(result)"
	// Pre-rendered data class conversion for the response.
	DataClassConversion string // e.g. "&pb.Location{\n\tLatitude: result.Latitude,\n...}"
}
