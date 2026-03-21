package cligen

// CLIFlag describes a cobra flag derived from a proto request field.
type CLIFlag struct {
	CobraName  string // e.g., "carrier-frequency" (kebab-case)
	ProtoField string // e.g., "CarrierFrequency" (PascalCase, for req.Field = v)
	CobraType  string // e.g., "Int64" (for Flags().Get<Type> / Flags().<Type>)
	GoType     string // e.g., "int64"
	Default    string // e.g., "0", `""`, "false"
	ProtoType  string // e.g., "int64" (proto field Go type)
}
