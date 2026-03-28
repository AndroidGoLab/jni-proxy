// Package cligen generates cobra CLI commands from Java API YAML specs.
// It produces Go source files for cmd/jnicli that call proto-generated
// gRPC stubs directly, covering the full Android API surface.
package cligen

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndroidGoLab/jni/tools/pkg/javagen"
	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protogen"
	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protoscan"
)

// Generate loads a Java API spec and overlay, builds proto data, converts
// it to CLI data structures, and writes a cobra command file.
// protoDir is the base directory containing compiled proto Go stubs.
func Generate(
	specPath string,
	overlayPath string,
	outputDir string,
	goModule string,
	protoDir string,
) error {
	spec, err := javagen.LoadSpec(specPath)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	overlay, err := javagen.LoadOverlay(overlayPath)
	if err != nil {
		return fmt.Errorf("load overlay: %w", err)
	}

	merged, err := javagen.Merge(spec, overlay)
	if err != nil {
		return fmt.Errorf("merge: %w", err)
	}

	protoData := protogen.BuildProtoData(merged, goModule)
	if len(protoData.Services) == 0 {
		return nil
	}

	return GenerateFromProtoData(protoData, outputDir, goModule, protoDir)
}

// GenerateFromProtoData generates CLI code from pre-built proto data.
func GenerateFromProtoData(
	protoData *protogen.ProtoData,
	outputDir string,
	goModule string,
	protoDir string,
) error {
	// Resolve proto names to actual Go names by scanning compiled proto stubs.
	goNames := protoscan.Scan(filepath.Join(protoDir, protoData.Package))

	cliPkg := buildCLIPackage(protoData, goModule, goNames)
	if cliPkg == nil {
		return nil
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outputDir, err)
	}

	outputPath := filepath.Join(outputDir, protoData.Package+".go")
	if err := renderPackage(cliPkg, outputPath); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	return nil
}
