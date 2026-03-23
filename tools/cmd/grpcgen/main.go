package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"

	"github.com/AndroidGoLab/jni-proxy/tools/pkg/grpcgen"
)

func main() {
	specsDir := flag.String("specs", "spec/java", "directory containing per-package YAML specs")
	overlaysDir := flag.String("overlays", "spec/overlays/java", "directory containing overlays")
	outputDir := flag.String("output", ".", "base output directory")
	goModule := flag.String("go-module", "github.com/AndroidGoLab/jni", "Go module path for generated proxy code")
	jniModule := flag.String("jni-module", "github.com/AndroidGoLab/jni", "Go module path for the JNI bindings (app package)")
	protoDir := flag.String("proto-dir", "proto", "directory containing compiled proto Go stubs")
	flag.Parse()

	specs, err := filepath.Glob(filepath.Join(*specsDir, "*.yaml"))
	if err != nil {
		log.Fatalf("glob specs: %v", err)
	}
	if len(specs) == 0 {
		log.Fatalf("no spec files found in %s", *specsDir)
	}

	// Accumulate server and client data per package so multiple specs
	// targeting the same package are merged instead of overwriting.
	serverByPkg := make(map[string]*grpcgen.ServerData)
	clientByPkg := make(map[string]*grpcgen.ClientData)

	for _, specPath := range specs {
		baseName := strings.TrimSuffix(filepath.Base(specPath), ".yaml")
		overlayPath := filepath.Join(*overlaysDir, baseName+".yaml")

		srvData, err := grpcgen.BuildServerFromSpec(specPath, overlayPath, *goModule, *jniModule, *protoDir)
		if err != nil {
			log.Fatalf("build server %s: %v", baseName, err)
		}
		if srvData != nil {
			if existing, ok := serverByPkg[srvData.Package]; ok {
				grpcgen.MergeServerData(existing, srvData)
			} else {
				serverByPkg[srvData.Package] = srvData
			}
		}

		cliData, err := grpcgen.BuildClientFromSpec(specPath, overlayPath, *goModule, *protoDir)
		if err != nil {
			log.Fatalf("build client %s: %v", baseName, err)
		}
		if cliData != nil {
			if existing, ok := clientByPkg[cliData.Package]; ok {
				grpcgen.MergeClientData(existing, cliData)
			} else {
				clientByPkg[cliData.Package] = cliData
			}
		}
	}

	// Write accumulated data and collect composite entries.
	var entries []grpcgen.CompositeEntry

	for _, data := range serverByPkg {
		if err := grpcgen.WriteServer(data, *outputDir); err != nil {
			log.Fatalf("write server %s: %v", data.Package, err)
		}
		entries = append(entries, grpcgen.EntriesFromServerData(data)...)
	}

	for _, data := range clientByPkg {
		if err := grpcgen.WriteClient(data, *outputDir); err != nil {
			log.Fatalf("write client %s: %v", data.Package, err)
		}
	}

	if err := grpcgen.GenerateCompositeServer(entries, *outputDir, *goModule, *jniModule); err != nil {
		log.Fatalf("generate composite server: %v", err)
	}
	if err := grpcgen.GenerateCompositeClient(entries, *outputDir, *goModule); err != nil {
		log.Fatalf("generate composite client: %v", err)
	}
}
