package grpcgen

// clientRenderData is the combined data passed to the client template.
type clientRenderData struct {
	Package      string
	GoModule     string
	Services     []ClientService
	DataClasses  []ClientDataClass
	DataClassMap map[string][]ClientDataClassField
}
