package grpcgen

// ClientMethod describes a single client RPC wrapper method.
type ClientMethod struct {
	GoName       string // Exported Go method name on the client, e.g. "GetLastKnownLocation"
	RequestType  string // Proto request message type
	ResponseType string // Proto response message type
	// Params to accept in the Go API.
	Params []ClientParam
	// Return type information.
	ReturnKind   string // "void", "string", "bool", "primitive", "data_class", "object"
	GoReturnType string // Go type of the result, e.g. "bool", "string", "int32"
	DataClass    string // If returning a data class, its Go type
	HasError     bool   // Whether the underlying method returns error
	HasResult    bool   // Whether the method returns a non-void value
	// Pre-rendered expression for extracting the result from the response.
	ResultExpr string // e.g. "resp.Result", "resp.GetResult()"
}
