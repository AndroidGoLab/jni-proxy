package protogen

// ProtoData holds all information needed to render a .proto file.
type ProtoData struct {
	Package      string
	ProtoPackage string // Package with '/' replaced by '.' for proto syntax
	GoPackage    string
	Services     []ProtoService
	Messages     []ProtoMessage
	Enums        []ProtoEnum
}
