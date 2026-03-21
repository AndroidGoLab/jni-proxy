package grpcgen

type compositeServerData struct {
	GoModule     string
	JniModule    string
	Entries      []CompositeEntry
	NeedsHandles bool
}
