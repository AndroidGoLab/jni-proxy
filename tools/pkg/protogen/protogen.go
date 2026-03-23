package protogen

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndroidGoLab/jni/tools/pkg/javagen"
)

// Generate loads a Java API spec and overlay, merges them, builds proto
// data structures, and writes a .proto file to the output directory.
func Generate(specPath, overlayPath, outputDir, goModule string) error {
	data, err := BuildFromSpec(specPath, overlayPath, goModule)
	if err != nil {
		return err
	}

	return WriteProto(data, outputDir)
}

// BuildFromSpec loads a spec/overlay pair, merges them, and returns proto data.
func BuildFromSpec(specPath, overlayPath, goModule string) (*ProtoData, error) {
	spec, err := javagen.LoadSpec(specPath)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}

	overlay, err := javagen.LoadOverlay(overlayPath)
	if err != nil {
		return nil, fmt.Errorf("load overlay: %w", err)
	}

	merged, err := javagen.Merge(spec, overlay)
	if err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}

	return BuildProtoData(merged, goModule), nil
}

// MergeProtoData combines two ProtoData for the same package.
// Identical messages (same name and field fingerprint) are deduplicated.
// Colliding messages (same name, different fields) are renamed with a
// class prefix from the service that owns them.
func MergeProtoData(dst, src *ProtoData) {
	seenMsg := make(map[string]string, len(dst.Messages)) // name -> fingerprint
	for _, m := range dst.Messages {
		seenMsg[m.Name] = messageFingerprint(m)
	}

	for _, m := range src.Messages {
		fp := messageFingerprint(m)
		if existingFP, exists := seenMsg[m.Name]; exists {
			if existingFP == fp {
				continue // identical — safe to share
			}
			// Collision: rename src message. Find the service that references it
			// and update its RPC input/output types.
			newName := findUniqueMessageName(m.Name, src.Services, seenMsg)
			for si := range src.Services {
				updateServiceRPCMessageName(&src.Services[si], m.Name, newName)
			}
			m.Name = newName
		}
		seenMsg[m.Name] = fp
		dst.Messages = append(dst.Messages, m)
	}

	// Merge services: if dst already has a service with the same name,
	// append new (non-duplicate) RPCs to it; otherwise add the service.
	for _, srcSvc := range src.Services {
		dstIdx := -1
		for i := range dst.Services {
			if dst.Services[i].Name == srcSvc.Name {
				dstIdx = i
				break
			}
		}
		if dstIdx < 0 {
			dst.Services = append(dst.Services, srcSvc)
			continue
		}
		seenRPC := make(map[string]bool, len(dst.Services[dstIdx].RPCs))
		for _, rpc := range dst.Services[dstIdx].RPCs {
			seenRPC[rpc.Name] = true
		}
		for _, rpc := range srcSvc.RPCs {
			if seenRPC[rpc.Name] {
				continue
			}
			seenRPC[rpc.Name] = true
			dst.Services[dstIdx].RPCs = append(dst.Services[dstIdx].RPCs, rpc)
		}
	}
	dst.Enums = append(dst.Enums, src.Enums...)
}

// findUniqueMessageName generates a unique message name by prefixing with
// the service name that references it.
func findUniqueMessageName(msgName string, services []ProtoService, seen map[string]string) string {
	// Find which service references this message.
	for _, svc := range services {
		for _, rpc := range svc.RPCs {
			if rpc.InputType == msgName || rpc.OutputType == msgName {
				candidate := svc.Name[:len(svc.Name)-len("Service")] + msgName
				if _, exists := seen[candidate]; !exists {
					return candidate
				}
			}
		}
	}
	// Fallback: append a suffix.
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", msgName, i)
		if _, exists := seen[candidate]; !exists {
			return candidate
		}
	}
}

// WriteProto writes a ProtoData to the appropriate .proto file.
func WriteProto(data *ProtoData, outputDir string) error {
	pkgDir := filepath.Join(outputDir, data.Package)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", pkgDir, err)
	}

	// Use only the last path segment as the filename (e.g., "health/connect" → "connect.proto").
	baseName := filepath.Base(data.Package)
	outputPath := filepath.Join(pkgDir, baseName+".proto")
	if err := renderProto(data, outputPath); err != nil {
		return fmt.Errorf("render proto: %w", err)
	}

	return nil
}
