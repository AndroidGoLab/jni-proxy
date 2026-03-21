package callbackgen

// templateData holds the context passed to the Java adapter template.
type templateData struct {
	Package string
	Entry   CallbackEntry
}
