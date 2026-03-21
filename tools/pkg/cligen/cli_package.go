package cligen

// CLIPackage describes a single proto package's CLI commands.
type CLIPackage struct {
	GoModule    string
	PackageName string       // e.g., "alarm"
	VarPrefix   string       // e.g., "alarm" (for Go var names)
	Services    []CLIService // non-streaming services
}
