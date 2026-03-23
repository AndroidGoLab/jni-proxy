package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"

	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protogen"
)

func main() {
	specsDir := flag.String("specs", "spec/java", "directory containing per-package YAML specs")
	overlaysDir := flag.String("overlays", "spec/overlays/java", "directory containing per-package overlay YAMLs")
	outputDir := flag.String("output", "proto", "base output directory for .proto files")
	goModule := flag.String("go-module", "github.com/AndroidGoLab/jni", "Go module path for go_package option")
	flag.Parse()

	specs, err := filepath.Glob(filepath.Join(*specsDir, "*.yaml"))
	if err != nil {
		log.Fatalf("glob specs: %v", err)
	}
	if len(specs) == 0 {
		log.Fatalf("no spec files found in %s", *specsDir)
	}

	// Multiple spec files can produce the same proto package (e.g.,
	// notification.yaml and service_notification.yaml both target
	// package "notification"). Accumulate ProtoData per package so
	// all services end up in one .proto file.
	byPackage := make(map[string]*protogen.ProtoData)

	for _, specPath := range specs {
		baseName := strings.TrimSuffix(filepath.Base(specPath), ".yaml")
		overlayPath := filepath.Join(*overlaysDir, baseName+".yaml")

		data, err := protogen.BuildFromSpec(specPath, overlayPath, *goModule)
		if err != nil {
			log.Fatalf("generate %s: %v", baseName, err)
		}

		if existing, ok := byPackage[data.Package]; ok {
			protogen.MergeProtoData(existing, data)
		} else {
			byPackage[data.Package] = data
		}
	}

	for _, data := range byPackage {
		if err := protogen.WriteProto(data, *outputDir); err != nil {
			log.Fatalf("write %s: %v", data.Package, err)
		}
	}
}
