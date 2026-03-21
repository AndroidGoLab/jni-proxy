package grpcgen

// ServerDataClass describes a data class used for result conversion.
type ServerDataClass struct {
	GoType string
	Fields []ServerDataClassField
}
