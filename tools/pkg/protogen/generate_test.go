package protogen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndroidGoLab/jni/spec"
	"github.com/AndroidGoLab/jni/tools/pkg/javagen"
)

// TestGenerate_AllRealSpecs is an E2E integration test that loads all spec/java/*.yaml
// files and verifies protogen generates valid .proto files for each one.
func TestGenerate_AllRealSpecs(t *testing.T) {
	entries, err := spec.Java.ReadDir("java")
	if err != nil {
		t.Fatalf("reading embedded spec dir: %v", err)
	}
	if len(entries) < 30 {
		t.Fatalf("expected at least 30 spec files, found %d", len(entries))
	}

	tmpDir := t.TempDir()
	outputDir := t.TempDir()
	goModule := "github.com/AndroidGoLab/jni"

	var failed []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		baseName := strings.TrimSuffix(entry.Name(), ".yaml")

		// Write embedded spec data to temp file.
		specData, err := spec.Java.ReadFile("java/" + entry.Name())
		if err != nil {
			t.Errorf("%s: read embedded spec: %v", baseName, err)
			failed = append(failed, baseName)
			continue
		}
		specFile := filepath.Join(tmpDir, entry.Name())
		if err := os.WriteFile(specFile, specData, 0644); err != nil {
			t.Fatalf("write temp spec %s: %v", baseName, err)
		}

		// Write embedded overlay data to temp file (may not exist).
		overlayFile := filepath.Join(tmpDir, "overlay_"+entry.Name())
		overlayData, overlayErr := spec.Overlays.ReadFile("overlays/java/" + entry.Name())
		if overlayErr != nil {
			if err := os.WriteFile(overlayFile, []byte("{}"), 0644); err != nil {
				t.Fatalf("write temp overlay %s: %v", baseName, err)
			}
		} else {
			if err := os.WriteFile(overlayFile, overlayData, 0644); err != nil {
				t.Fatalf("write temp overlay %s: %v", baseName, err)
			}
		}

		// Load the spec to determine the actual package name, which may differ
		// from the YAML filename (e.g. connectivity.yaml has package "net").
		sp, parseErr := javagen.ParseSpec(specData)
		if parseErr != nil {
			t.Errorf("%s: parse spec: %v", baseName, parseErr)
			failed = append(failed, baseName)
			continue
		}
		pkgName := sp.Package

		if err := Generate(specFile, overlayFile, outputDir, goModule); err != nil {
			t.Errorf("Generate %s: %v", baseName, err)
			failed = append(failed, baseName)
			continue
		}

		protoPath := filepath.Join(outputDir, pkgName, pkgName+".proto")
		data, err := os.ReadFile(protoPath)
		if err != nil {
			t.Errorf("%s (pkg=%s): proto file not created at %s: %v", baseName, pkgName, protoPath, err)
			failed = append(failed, baseName)
			continue
		}

		content := string(data)
		if !strings.Contains(content, `syntax = "proto3";`) {
			t.Errorf("%s: missing proto3 syntax declaration", baseName)
			failed = append(failed, baseName)
		}
	}

	t.Logf("processed %d spec files, %d failures", len(entries), len(failed))
	if len(failed) > 0 {
		t.Errorf("failed specs: %s", strings.Join(failed, ", "))
	}
}
