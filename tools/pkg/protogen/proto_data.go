package protogen

// ProtoData holds all information needed to render a .proto file.
type ProtoData struct {
	Package   string
	GoPackage string
	Services  []ProtoService
	Messages  []ProtoMessage
	Enums     []ProtoEnum
}
