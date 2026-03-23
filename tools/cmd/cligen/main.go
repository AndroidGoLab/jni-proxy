package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"

	"github.com/AndroidGoLab/jni-proxy/tools/pkg/cligen"
	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protogen"
)

func main() {
	specsDir := flag.String("specs", "spec/java", "directory containing per-package YAML specs")
	overlaysDir := flag.String("overlays", "spec/overlays/java", "directory containing overlays")
	outputDir := flag.String("output", "cmd/jnicli", "output directory for generated CLI files")
	goModule := flag.String("go-module", "github.com/AndroidGoLab/jni", "Go module path")
	protoDir := flag.String("proto-dir", "proto", "directory containing compiled proto Go stubs")
	flag.Parse()

	specs, err := filepath.Glob(filepath.Join(*specsDir, "*.yaml"))
	if err != nil {
		log.Fatalf("glob specs: %v", err)
	}
	if len(specs) == 0 {
		log.Fatalf("no spec files found in %s", *specsDir)
	}

	// Accumulate ProtoData per package so multiple specs targeting the
	// same package are merged (same approach as protogen cmd).
	byPackage := make(map[string]*protogen.ProtoData)

	for _, specPath := range specs {
		baseName := strings.TrimSuffix(filepath.Base(specPath), ".yaml")
		overlayPath := filepath.Join(*overlaysDir, baseName+".yaml")

		data, err := protogen.BuildFromSpec(specPath, overlayPath, *goModule)
		if err != nil {
			log.Fatalf("build %s: %v", baseName, err)
		}

		if existing, ok := byPackage[data.Package]; ok {
			protogen.MergeProtoData(existing, data)
		} else {
			byPackage[data.Package] = data
		}
	}

	for _, data := range byPackage {
		if len(data.Services) == 0 {
			continue
		}
		if err := cligen.GenerateFromProtoData(data, *outputDir, *goModule, *protoDir); err != nil {
			log.Fatalf("generate %s: %v", data.Package, err)
		}
	}
}
