package cligen

// CLICommand describes a single RPC as a cobra leaf command.
type CLICommand struct {
	RPCName         string    // e.g., "Cancel"
	CobraName       string    // e.g., "cancel"
	VarName         string    // e.g., "Cancel" (for Go var names)
	Short           string    // cobra short description
	RequestType     string    // e.g., "CancelRequest"
	Flags           []CLIFlag // request message fields → flags
	ServerStreaming bool
	ClientStreaming bool
}
