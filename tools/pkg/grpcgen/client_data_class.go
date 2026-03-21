package grpcgen

// ClientDataClass describes a data class used for result conversion in the client.
type ClientDataClass struct {
	GoType string
	Fields []ClientDataClassField
}
