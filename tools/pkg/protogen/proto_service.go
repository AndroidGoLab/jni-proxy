package protogen

// ProtoService describes a gRPC service generated from a Java class.
type ProtoService struct {
	Name string
	RPCs []ProtoRPC
}
