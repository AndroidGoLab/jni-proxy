package magisk

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// TestAndroidServerImportsTrackedBindConfigPackage checks the server source import.
//
// Example: a source import check distinguishes the tracked listener resolver from the excluded package.
func TestAndroidServerImportsTrackedBindConfigPackage(t *testing.T) {
	serverPath := filepath.Join("..", "server.go")
	file, err := parser.ParseFile(token.NewFileSet(), serverPath, nil, 0)
	if err != nil {
		t.Fatalf("parse Android server: %v", err)
	}

	imports := make(map[string]bool, len(file.Imports))
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("decode server import %q: %v", spec.Path.Value, err)
		}
		imports[path] = true
	}

	const trackedResolver = "github.com/AndroidGoLab/jni-proxy/internal/jniservicebindconfig"
	const untrackedResolver = "github.com/AndroidGoLab/jni-proxy/internal/jniserviceconfig"
	if !imports[trackedResolver] {
		t.Errorf("Android server imports %q = false, want the tracked listener resolver", trackedResolver)
	}
	if imports[untrackedResolver] {
		t.Errorf("Android server still imports pre-existing untracked resolver %q", untrackedResolver)
	}
}
