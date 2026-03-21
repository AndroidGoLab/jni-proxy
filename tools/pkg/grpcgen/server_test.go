package grpcgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndroidGoLab/jni/tools/pkg/javagen"
	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protogen"
)

// findJNIRepoRoot locates the jni repo root by walking up from cwd to find
// the go.work file, then looking for the sibling jni directory that contains
// the spec/ directory. Falls back to ../jni relative to the jni-proxy root.
func findJNIRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	// Walk up to find go.mod (jni-proxy root).
	proxyRoot := dir
	for {
		if _, err := os.Stat(filepath.Join(proxyRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(proxyRoot)
		if parent == proxyRoot {
			t.Fatal("could not find jni-proxy root (no go.mod found)")
		}
		proxyRoot = parent
	}
	// The jni repo is expected as a sibling directory.
	jniRoot := filepath.Join(filepath.Dir(proxyRoot), "jni")
	if _, err := os.Stat(filepath.Join(jniRoot, "spec")); err != nil {
		t.Fatalf("jni repo not found at %s (need spec/ directory)", jniRoot)
	}
	return jniRoot
}

func TestBuildServerData_Location(t *testing.T) {
	root := findJNIRepoRoot(t)
	specPath := filepath.Join(root, "spec", "java", "location.yaml")
	overlayPath := filepath.Join(root, "spec", "overlays", "java", "location.yaml")

	spec, err := javagen.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	overlay, err := javagen.LoadOverlay(overlayPath)
	if err != nil {
		t.Fatalf("load overlay: %v", err)
	}
	merged, err := javagen.Merge(spec, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	goModule := "github.com/AndroidGoLab/jni"
	protoData := protogen.BuildProtoData(merged, goModule)
	data := buildServerData(merged, goModule, goModule, protoData, emptyGoNames())

	if data.Package != "location" {
		t.Errorf("Package = %q, want %q", data.Package, "location")
	}
	if data.GoModule != "github.com/AndroidGoLab/jni" {
		t.Errorf("GoModule = %q, want %q", data.GoModule, "github.com/AndroidGoLab/jni")
	}
	if len(data.Services) != 1 {
		t.Fatalf("Services count = %d, want 1", len(data.Services))
	}

	svc := data.Services[0]
	if svc.GoType != "Manager" {
		t.Errorf("Service GoType = %q, want %q", svc.GoType, "Manager")
	}
	if svc.ServiceName != "ManagerService" {
		t.Errorf("Service ServiceName = %q, want %q", svc.ServiceName, "ManagerService")
	}
	if !svc.Close {
		t.Error("Service Close should be true")
	}

	// Check that methods include GetLastKnownLocation and IsProviderEnabled.
	// Note: unexported spec methods like getProvidersRaw are skipped.
	methodNames := make(map[string]bool)
	for _, m := range svc.Methods {
		methodNames[m.GoName] = true
	}
	for _, want := range []string{"GetLastKnownLocation", "IsProviderEnabled"} {
		if !methodNames[want] {
			t.Errorf("missing method %q in service", want)
		}
	}
	// Verify unexported spec methods are excluded.
	for _, unwanted := range []string{"GetProvidersRaw"} {
		if methodNames[unwanted] {
			t.Errorf("unexported spec method %q should not be in service", unwanted)
		}
	}
}

func TestBuildServerData_LocationMethod_Object(t *testing.T) {
	root := findJNIRepoRoot(t)
	specPath := filepath.Join(root, "spec", "java", "location.yaml")
	overlayPath := filepath.Join(root, "spec", "overlays", "java", "location.yaml")

	spec, err := javagen.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	overlay, err := javagen.LoadOverlay(overlayPath)
	if err != nil {
		t.Fatalf("load overlay: %v", err)
	}
	merged, err := javagen.Merge(spec, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	goModule := "github.com/AndroidGoLab/jni"
	protoData := protogen.BuildProtoData(merged, goModule)
	data := buildServerData(merged, goModule, goModule, protoData, emptyGoNames())

	// Find GetLastKnownLocation method.
	var m *ServerMethod
	for i := range data.Services[0].Methods {
		if data.Services[0].Methods[i].GoName == "GetLastKnownLocation" {
			m = &data.Services[0].Methods[i]
			break
		}
	}
	if m == nil {
		t.Fatal("GetLastKnownLocation method not found")
	}

	// The new spec has no data_class kind; GetLastKnownLocation returns an
	// android.location.Location object, which is treated as a plain object.
	if m.ReturnKind != "object" {
		t.Errorf("ReturnKind = %q, want %q", m.ReturnKind, "object")
	}
	if m.DataClass != "" {
		t.Errorf("DataClass = %q, want %q", m.DataClass, "")
	}
	if !m.HasResult {
		t.Error("HasResult should be true")
	}
	if !m.HasError {
		t.Error("HasError should be true")
	}
}

func TestRenderServerToString_Location(t *testing.T) {
	root := findJNIRepoRoot(t)
	specPath := filepath.Join(root, "spec", "java", "location.yaml")
	overlayPath := filepath.Join(root, "spec", "overlays", "java", "location.yaml")

	spec, err := javagen.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	overlay, err := javagen.LoadOverlay(overlayPath)
	if err != nil {
		t.Fatalf("load overlay: %v", err)
	}
	merged, err := javagen.Merge(spec, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	goModule := "github.com/AndroidGoLab/jni"
	protoData := protogen.BuildProtoData(merged, goModule)
	data := buildServerData(merged, goModule, goModule, protoData, emptyGoNames())
	output, err := renderServerToString(data)
	if err != nil {
		t.Fatalf("renderServerToString: %v", err)
	}

	// Verify key patterns.
	checks := []struct {
		name    string
		pattern string
	}{
		{"generated header", "Code generated by grpcgen. DO NOT EDIT."},
		{"package", "package location"},
		{"jni import", `"github.com/AndroidGoLab/jni"`},
		{"jnipkg import", `jnipkg "github.com/AndroidGoLab/jni/location"`},
		{"pb import", `pb "github.com/AndroidGoLab/jni/proto/location"`},
		{"struct type", "type ManagerServer struct"},
		{"unimplemented embed", "pb.UnimplementedManagerServiceServer"},
		{"method sig", "func (s *ManagerServer) GetLastKnownLocation"},
		{"new manager", "jnipkg.NewManager(s.Ctx)"},
		{"defer close", "defer mgr.Close()"},
		{"proto response", "&pb.GetLastKnownLocationResponse{"},
		{"bool method", "func (s *ManagerServer) IsProviderEnabled"},
		{"bool result", "Result: result"},
		{"error return", "status.Errorf(codes.Internal,"},
	}
	for _, c := range checks {
		if !strings.Contains(output, c.pattern) {
			t.Errorf("%s: output missing pattern %q", c.name, c.pattern)
		}
	}
}

func TestGenerateServer_AllRealSpecs(t *testing.T) {
	root := findJNIRepoRoot(t)
	specsDir := filepath.Join(root, "spec", "java")
	overlaysDir := filepath.Join(root, "spec", "overlays", "java")
	outputDir := t.TempDir()
	goModule := "github.com/AndroidGoLab/jni"

	specFiles, err := filepath.Glob(filepath.Join(specsDir, "*.yaml"))
	if err != nil {
		t.Fatalf("glob specs: %v", err)
	}
	if len(specFiles) < 40 {
		t.Fatalf("expected at least 40 spec files, found %d", len(specFiles))
	}

	var generated, skipped int
	var failed []string
	for _, specPath := range specFiles {
		baseName := strings.TrimSuffix(filepath.Base(specPath), ".yaml")
		overlayPath := filepath.Join(overlaysDir, baseName+".yaml")

		if _, err := GenerateServer(specPath, overlayPath, outputDir, goModule, goModule, ""); err != nil {
			t.Errorf("GenerateServer %s: %v", baseName, err)
			failed = append(failed, baseName)
			continue
		}

		// Load spec to determine package name.
		spec, loadErr := javagen.LoadSpec(specPath)
		if loadErr != nil {
			t.Errorf("%s: load spec: %v", baseName, loadErr)
			failed = append(failed, baseName)
			continue
		}

		serverPath := filepath.Join(outputDir, "grpc", "server", spec.Package, "server.go")
		if _, err := os.Stat(serverPath); err != nil {
			// Some packages don't generate servers (no system_service/context_method).
			skipped++
			continue
		}

		data, err := os.ReadFile(serverPath)
		if err != nil {
			t.Errorf("%s: read server.go: %v", baseName, err)
			failed = append(failed, baseName)
			continue
		}

		content := string(data)
		if !strings.Contains(content, "Code generated by grpcgen. DO NOT EDIT.") {
			t.Errorf("%s: missing generated header", baseName)
			failed = append(failed, baseName)
		}
		if !strings.Contains(content, "pb.Unimplemented") {
			t.Errorf("%s: missing Unimplemented embed", baseName)
			failed = append(failed, baseName)
		}
		generated++
	}

	t.Logf("processed %d specs: %d generated, %d skipped (no service), %d failures",
		len(specFiles), generated, skipped, len(failed))
	if len(failed) > 0 {
		t.Errorf("failed specs: %s", strings.Join(failed, ", "))
	}
	if generated < 8 {
		t.Errorf("expected at least 8 generated servers, got %d", generated)
	}
}

func TestRenderServerToString_SeparateModules(t *testing.T) {
	root := findJNIRepoRoot(t)
	specPath := filepath.Join(root, "spec", "java", "location.yaml")
	overlayPath := filepath.Join(root, "spec", "overlays", "java", "location.yaml")

	spec, err := javagen.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	overlay, err := javagen.LoadOverlay(overlayPath)
	if err != nil {
		t.Fatalf("load overlay: %v", err)
	}
	merged, err := javagen.Merge(spec, overlay)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	goModule := "github.com/AndroidGoLab/jni-proxy"
	jniModule := "github.com/AndroidGoLab/jni"
	protoData := protogen.BuildProtoData(merged, goModule)
	data := buildServerData(merged, goModule, jniModule, protoData, emptyGoNames())
	output, err := renderServerToString(data)
	if err != nil {
		t.Fatalf("renderServerToString: %v", err)
	}

	// app must import from jni module, not proxy module.
	if strings.Contains(output, `"github.com/AndroidGoLab/jni-proxy/app"`) {
		t.Error("app import incorrectly uses proxy module")
	}
	if !strings.Contains(output, `"github.com/AndroidGoLab/jni/app"`) {
		t.Error("app import missing from jni module")
	}

	// proto and handlestore must import from proxy module.
	if !strings.Contains(output, `"github.com/AndroidGoLab/jni-proxy/proto/location"`) {
		t.Error("proto import should use proxy module")
	}
	if !strings.Contains(output, `"github.com/AndroidGoLab/jni-proxy/handlestore"`) {
		t.Error("handlestore import should use proxy module")
	}

	// jnipkg must import from jni module (via GoImport).
	if !strings.Contains(output, `jnipkg "github.com/AndroidGoLab/jni/location"`) {
		t.Error("jnipkg import should use jni module")
	}
}

func TestExportName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"getProviders", "GetProviders"},
		{"GetLastKnown", "GetLastKnown"},
		{"", ""},
		{"a", "A"},
	}
	for _, tt := range tests {
		got := exportName(tt.input)
		if got != tt.want {
			t.Errorf("exportName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHasContextConstructor(t *testing.T) {
	tests := []struct {
		name   string
		cls    javagen.MergedClass
		expect bool
	}{
		{
			name:   "system_service",
			cls:    javagen.MergedClass{Obtain: "system_service"},
			expect: true,
		},
		{
			name:   "context_method",
			cls:    javagen.MergedClass{Obtain: "context_method"},
			expect: false,
		},
		{
			name:   "constructor",
			cls:    javagen.MergedClass{Obtain: "constructor"},
			expect: false,
		},
		{
			name:   "no obtain",
			cls:    javagen.MergedClass{Obtain: ""},
			expect: false,
		},
		{
			name:   "data_class",
			cls:    javagen.MergedClass{Kind: "data_class", Obtain: "system_service"},
			expect: false,
		},
		{
			name:   "builder",
			cls:    javagen.MergedClass{Kind: "builder"},
			expect: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasContextConstructor(tt.cls)
			if got != tt.expect {
				t.Errorf("hasContextConstructor() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestBuildCallArgs(t *testing.T) {
	tests := []struct {
		name        string
		params      []javagen.MergedParam
		want        string
		wantHandles bool
	}{
		{
			name:   "empty",
			params: nil,
			want:   "",
		},
		{
			name: "string param",
			params: []javagen.MergedParam{
				{GoName: "provider", GoType: "string", IsString: true},
			},
			want: "req.GetProvider()",
		},
		{
			name: "bool param",
			params: []javagen.MergedParam{
				{GoName: "enabledOnly", GoType: "bool", IsBool: true},
			},
			want: "req.GetEnabledOnly()",
		},
		{
			name: "object param",
			params: []javagen.MergedParam{
				{GoName: "listener", GoType: "*jni.Object", IsObject: true},
			},
			want:        "s.Handles.Get(req.GetListener())",
			wantHandles: true,
		},
		{
			name: "mixed params",
			params: []javagen.MergedParam{
				{GoName: "provider", GoType: "string", IsString: true},
				{GoName: "minTimeMs", GoType: "int64"},
				{GoName: "listener", GoType: "*jni.Object", IsObject: true},
			},
			want:        "req.GetProvider(), req.GetMinTimeMs(), s.Handles.Get(req.GetListener())",
			wantHandles: true,
		},
		{
			name: "android.content.Context auto-substituted",
			params: []javagen.MergedParam{
				{GoName: "arg0", GoType: "*jni.Object", IsObject: true, JavaType: "android.content.Context"},
			},
			want:        "s.Ctx.Obj",
			wantHandles: false,
		},
		{
			name: "context with other params",
			params: []javagen.MergedParam{
				{GoName: "arg0", GoType: "*jni.Object", IsObject: true, JavaType: "android.content.Context"},
				{GoName: "arg1", GoType: "string", IsString: true},
			},
			want:        "s.Ctx.Obj, req.GetArg1()",
			wantHandles: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotHandles := buildCallArgs(tt.params)
			if got != tt.want {
				t.Errorf("buildCallArgs() = %q, want %q", got, tt.want)
			}
			if gotHandles != tt.wantHandles {
				t.Errorf("buildCallArgs() needsHandles = %v, want %v", gotHandles, tt.wantHandles)
			}
		})
	}
}

func TestIsExported(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"exported", "Manager", true},
		{"unexported", "manager", false},
		{"empty", "", false},
		{"uppercase single", "A", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExported(tt.input)
			if got != tt.expect {
				t.Errorf("isExported(%q) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestProtoGoFieldName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"SSID", "Ssid"},
		{"BSSID", "Bssid"},
		{"RSSI", "Rssi"},
		{"IPAddress", "IpAddress"},
		{"Latitude", "Latitude"},
		{"Longitude", "Longitude"},
		{"LinkSpeed", "LinkSpeed"},
		{"Provider", "Provider"},
		{"Time", "Time"},
		{"Cn0DbHz", "Cn0dbHz"},
		{"UsedInFix", "UsedInFix"},
	}
	for _, tt := range tests {
		got := protoGoFieldName(tt.input)
		if got != tt.want {
			t.Errorf("protoGoFieldName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildServerMethod_ObjectNoError(t *testing.T) {
	// When a method returns an object (not a data class) and has no error,
	// the template uses the {{else if $m.HasResult}} branch which emits
	// Result: {{$m.ResultExpr}}. ResultExpr must be set to "result".
	m := javagen.MergedMethod{
		GoName:     "GetHandle",
		ReturnKind: javagen.ReturnObject,
		GoReturn:   "*jni.Object",
		Returns:    "android.some.Unknown",
		Error:      false,
	}
	sm := buildServerMethod(
		m,
		map[string]bool{},            // no data classes
		map[string]string{},           // no java->data class mapping
		map[string][]javagen.MergedField{}, // no data class fields
		map[string]rpcInfo{},          // no proto RPC lookup
		"TestService",
		emptyGoNames(),
	)
	if sm.ReturnKind != "object" {
		t.Errorf("ReturnKind = %q, want %q", sm.ReturnKind, "object")
	}
	if !sm.HasResult {
		t.Error("HasResult should be true")
	}
	if sm.HasError {
		t.Error("HasError should be false")
	}
	if sm.ResultExpr != "result" {
		t.Errorf("ResultExpr = %q, want %q", sm.ResultExpr, "result")
	}
}

func TestConvertPrimitiveExpr(t *testing.T) {
	tests := []struct {
		goType string
		want   string
	}{
		{"int32", "result"},
		{"int64", "result"},
		{"float32", "result"},
		{"bool", "result"},
		{"int16", "int32(result)"},
		{"byte", "uint32(result)"},
	}
	for _, tt := range tests {
		got := convertPrimitiveExpr(tt.goType, "result")
		if got != tt.want {
			t.Errorf("convertPrimitiveExpr(%q, result) = %q, want %q", tt.goType, got, tt.want)
		}
	}
}
