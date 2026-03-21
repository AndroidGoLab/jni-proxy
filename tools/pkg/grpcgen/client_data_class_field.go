package grpcgen

// ClientDataClassField describes a field in a data class for client-side conversion.
type ClientDataClassField struct {
	GoName    string // Field name in the Go struct
	ProtoName string // Field name in the proto message
	GoType    string // Go type
	ProtoType string // Proto Go type (may differ, e.g. int32 vs int16)
}
