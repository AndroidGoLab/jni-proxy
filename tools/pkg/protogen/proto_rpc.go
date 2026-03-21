package protogen

// ProtoRPC describes a single RPC method in a gRPC service.
type ProtoRPC struct {
	Name            string
	OriginalName    string // Name before collision renaming (empty if unchanged).
	InputType       string
	OutputType      string
	ClientStreaming bool
	ServerStreaming bool
	Comment         string
}
