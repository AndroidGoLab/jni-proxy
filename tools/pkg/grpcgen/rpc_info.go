package grpcgen

// rpcInfo holds the proto RPC name and its actual message type names,
// which may differ from the default "Name+Request"/"Name+Response"
// when cross-class message collisions required disambiguation.
type rpcInfo struct {
	Name       string
	InputType  string
	OutputType string
}
