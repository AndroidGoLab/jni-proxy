package cligen

// CLIService describes a gRPC service's CLI subcommands.
type CLIService struct {
	ProtoServiceName string       // e.g., "ManagerService"
	CobraName        string       // e.g., "manager"
	VarName          string       // e.g., "Manager" (for Go var names)
	Short            string       // cobra short description
	Commands         []CLICommand // leaf commands
}
