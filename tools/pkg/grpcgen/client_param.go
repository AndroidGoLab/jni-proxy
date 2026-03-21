package grpcgen

// ClientParam describes a parameter in the client's Go API.
type ClientParam struct {
	GoName    string // Go parameter name
	GoType    string // Go type
	ProtoName string // Corresponding proto field name (exported)
	IsObject  bool   // Whether this is an object handle (int64 in proto)
}
