package callbackgen

// CallbackEntry describes a single abstract class or interface
// that needs a generated Java adapter.
type CallbackEntry struct {
	// Class is the fully-qualified Java class or interface name.
	Class string `yaml:"class"`

	// Adapter is the generated adapter class name (unqualified).
	Adapter string `yaml:"adapter"`

	// Interface indicates whether the base type is a Java interface
	// (uses "implements") rather than a class (uses "extends").
	Interface bool `yaml:"interface"`

	// Methods lists the abstract methods to override.
	Methods []MethodEntry `yaml:"methods"`
}
