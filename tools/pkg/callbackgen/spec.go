package callbackgen

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Spec is the top-level structure of the callbacks YAML spec.
type Spec struct {
	Callbacks []CallbackEntry `yaml:"callbacks"`
}

// LoadSpec reads and parses a callbacks YAML spec file.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return &spec, nil
}
