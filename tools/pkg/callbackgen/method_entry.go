package callbackgen

// MethodEntry describes a single abstract method to override.
type MethodEntry struct {
	Name   string       `yaml:"name"`
	Params []ParamEntry `yaml:"params"`
}
