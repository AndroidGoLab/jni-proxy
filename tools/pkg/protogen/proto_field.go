package protogen

// ProtoField describes a single field in a protobuf message.
type ProtoField struct {
	Type     string
	Name     string
	Number   int
	Repeated bool
	Optional bool
	Comment  string
}
