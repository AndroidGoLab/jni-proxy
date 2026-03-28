package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"

	"github.com/AndroidGoLab/jni-proxy/tools/pkg/grpcgen"
	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protogen"
	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protoscan"
)

type specEntry struct {
	specPath    string
	overlayPath string
}

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

	// Pass 1: Build merged ProtoData per package.
	// This ensures message collision renames are visible when building
	// server/client data in pass 2.
	protoByPkg := make(map[string]*protogen.ProtoData)
	specsByPkg := make(map[string][]specEntry)

	for _, specPath := range specs {
		baseName := strings.TrimSuffix(filepath.Base(specPath), ".yaml")
		overlayPath := filepath.Join(*overlaysDir, baseName+".yaml")

		pd, err := protogen.BuildFromSpec(specPath, overlayPath, *goModule)
		if err != nil {
			log.Fatalf("build proto %s: %v", baseName, err)
		}

		if existing, ok := protoByPkg[pd.Package]; ok {
			protogen.MergeProtoData(existing, pd)
		} else {
			protoByPkg[pd.Package] = pd
		}
		specsByPkg[pd.Package] = append(specsByPkg[pd.Package], specEntry{specPath, overlayPath})
	}

	// Pass 2: Build server and client data per spec, using the merged
	// ProtoData for RPC lookups (so message collision renames are reflected).
	serverByPkg := make(map[string]*grpcgen.ServerData)
	clientByPkg := make(map[string]*grpcgen.ClientData)

	for pkg, entries := range specsByPkg {
		mergedProto := protoByPkg[pkg]
		goNames := protoscan.Scan(filepath.Join(*protoDir, pkg))

		for _, e := range entries {
			srvData, err := grpcgen.BuildServerFromMergedProto(e.specPath, e.overlayPath, *goModule, *jniModule, mergedProto, goNames)
			if err != nil {
				log.Fatalf("build server %s: %v", e.specPath, err)
			}
			if srvData != nil {
				if existing, ok := serverByPkg[pkg]; ok {
					grpcgen.MergeServerData(existing, srvData)
				} else {
					serverByPkg[pkg] = srvData
				}
			}

			cliData, err := grpcgen.BuildClientFromMergedProto(e.specPath, e.overlayPath, *goModule, mergedProto, goNames)
			if err != nil {
				log.Fatalf("build client %s: %v", e.specPath, err)
			}
			if cliData != nil {
				if existing, ok := clientByPkg[pkg]; ok {
					grpcgen.MergeClientData(existing, cliData)
				} else {
					clientByPkg[pkg] = cliData
				}
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
