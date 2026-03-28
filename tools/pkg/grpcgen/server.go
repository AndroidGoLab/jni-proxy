package grpcgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndroidGoLab/jni/tools/pkg/javagen"
	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protogen"
	"github.com/AndroidGoLab/jni-proxy/tools/pkg/protoscan"
)

// GenerateServer loads a Java API spec and overlay, merges them, builds server
// data structures, and writes a gRPC server implementation file.
// It returns composite entries for each service generated (for use in the
// composite server registration file).
func GenerateServer(
	specPath string,
	overlayPath string,
	outputDir string,
	goModule string,
	jniModule string,
	protoDir string,
) ([]CompositeEntry, error) {
	data, err := BuildServerFromSpec(specPath, overlayPath, goModule, jniModule, protoDir)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	entries := EntriesFromServerData(data)
	if err := WriteServer(data, outputDir); err != nil {
		return nil, err
	}
	return entries, nil
}

// BuildServerFromSpec loads a spec/overlay pair and returns server data.
// Returns nil if the spec produces no services.
func BuildServerFromSpec(specPath, overlayPath, goModule, jniModule, protoDir string) (*ServerData, error) {
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

	protoData := protogen.BuildProtoData(merged, goModule)
	goNames := protoscan.Scan(filepath.Join(protoDir, merged.Package))

	data := buildServerData(merged, goModule, jniModule, protoData, goNames)
	if len(data.Services) == 0 {
		return nil, nil
	}
	return data, nil
}

// BuildServerFromMergedProto builds server data using pre-merged ProtoData.
func BuildServerFromMergedProto(specPath, overlayPath, goModule, jniModule string, mergedProto *protogen.ProtoData, goNames protoscan.GoNames) (*ServerData, error) {
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

	data := buildServerData(merged, goModule, jniModule, mergedProto, goNames)
	if len(data.Services) == 0 {
		return nil, nil
	}
	return data, nil
}

// MergeServerData combines two ServerData for the same package.
func MergeServerData(dst, src *ServerData) {
	seenSvc := make(map[string]bool, len(dst.Services))
	for _, s := range dst.Services {
		seenSvc[s.GoType] = true
	}
	for _, s := range src.Services {
		if seenSvc[s.GoType] {
			continue
		}
		seenSvc[s.GoType] = true
		dst.Services = append(dst.Services, s)
		if s.NeedsHandles {
			dst.NeedsHandles = true
		}
	}
	if src.NeedsJNI {
		dst.NeedsJNI = true
	}

	seenDC := make(map[string]bool, len(dst.DataClasses))
	for _, dc := range dst.DataClasses {
		seenDC[dc.GoType] = true
	}
	for _, dc := range src.DataClasses {
		if seenDC[dc.GoType] {
			continue
		}
		seenDC[dc.GoType] = true
		dst.DataClasses = append(dst.DataClasses, dc)
	}

	// In mixed packages (system_service + constructor), constructor services
	// cause message name collisions (handle vs no-handle). Remove them.
	hasSystemService := false
	for _, svc := range dst.Services {
		if svc.InstantiationKind == "system_service" {
			hasSystemService = true
			break
		}
	}
	if hasSystemService {
		var filtered []ServerService
		for _, svc := range dst.Services {
			if svc.InstantiationKind == "constructor" {
				continue
			}
			filtered = append(filtered, svc)
		}
		dst.Services = filtered
	}

	// Recalculate flags after filtering.
	dst.NeedsJNI = false
	dst.NeedsHandles = false
	for _, svc := range dst.Services {
		if svc.NeedsHandles {
			dst.NeedsHandles = true
		}
		for _, m := range svc.Methods {
			if m.IsConstructor || m.ReturnKind == "data_class" || m.ReturnKind == "object" {
				dst.NeedsJNI = true
			}
		}
	}

	assignImportAliases(dst)
}

// assignImportAliases assigns unique import aliases to services based on
// their GoImport paths. When all services share the same path, they all
// get "jnipkg". When paths differ, each unique path gets a numbered alias.
func assignImportAliases(data *ServerData) {
	paths := make(map[string]string) // GoImport → alias
	counter := 0
	for i := range data.Services {
		svc := &data.Services[i]
		if alias, ok := paths[svc.GoImport]; ok {
			svc.ImportAlias = alias
			continue
		}
		switch counter {
		case 0:
			svc.ImportAlias = "jnipkg"
		default:
			svc.ImportAlias = fmt.Sprintf("jnipkg%d", counter+1)
		}
		paths[svc.GoImport] = svc.ImportAlias
		counter++
	}
}

// EntriesFromServerData extracts CompositeEntry records from a ServerData.
func EntriesFromServerData(data *ServerData) []CompositeEntry {
	var entries []CompositeEntry
	for _, svc := range data.Services {
		entries = append(entries, CompositeEntry{
			Package:      data.Package,
			GoType:       svc.GoType,
			ServiceName:  svc.ServiceName,
			NeedsHandles: svc.NeedsHandles,
		})
	}
	return entries
}

// WriteServer writes a ServerData to its package directory.
func WriteServer(data *ServerData, outputDir string) error {
	pkgDir := filepath.Join(outputDir, "grpc", "server", data.Package)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", pkgDir, err)
	}

	outputPath := filepath.Join(pkgDir, "server.go")
	return renderServer(data, outputPath)
}

// buildServerData converts a MergedSpec into server template data.
func buildServerData(
	merged *javagen.MergedSpec,
	goModule string,
	jniModule string,
	protoData *protogen.ProtoData,
	goNames protoscan.GoNames,
) *ServerData {
	data := &ServerData{
		Package:   merged.Package,
		GoModule:  goModule,
		JniModule: jniModule,
		GoImport:  merged.GoImport,
	}

	// Build maps from Java class name to data class info.
	// Only include exported data classes (those whose GoType starts with uppercase),
	// because the Extract function for unexported types is not accessible from
	// the grpcserver package.
	javaClassToDataClass := make(map[string]string)
	dataClassMap := make(map[string]bool)
	dcFieldMap := make(map[string][]javagen.MergedField)
	for _, dc := range merged.DataClasses {
		if !isExported(dc.GoType) {
			continue
		}
		javaClassToDataClass[dc.JavaClass] = dc.GoType
		dataClassMap[dc.GoType] = true
		dcFieldMap[dc.GoType] = dc.Fields
	}

	// Build data class info for result conversion.
	for _, dc := range merged.DataClasses {
		if !isExported(dc.GoType) {
			continue
		}
		sdc := ServerDataClass{GoType: dc.GoType}
		for _, f := range dc.Fields {
			sdc.Fields = append(sdc.Fields, ServerDataClassField{
				GoName:    f.GoName,
				ProtoName: f.GoName,
				GoType:    f.GoType,
			})
		}
		data.DataClasses = append(data.DataClasses, sdc)
	}

	// Build RPC lookup from protogen data (handles collision renames and
	// cross-class message name disambiguation).
	protoRPCLookup := make(map[string]rpcInfo)
	for _, ps := range protoData.Services {
		for _, rpc := range ps.RPCs {
			info := rpcInfo{
				Name:       rpc.Name,
				InputType:  rpc.InputType,
				OutputType: rpc.OutputType,
			}
			key := ps.Name + "/" + strings.ToLower(rpc.Name)
			protoRPCLookup[key] = info
			if rpc.OriginalName != "" {
				origKey := ps.Name + "/" + strings.ToLower(rpc.OriginalName)
				protoRPCLookup[origKey] = info
			}
		}
	}

	// Build services from eligible classes (same criteria as protogen).
	for _, cls := range merged.Classes {
		if !protogen.IsServerEligible(cls) || !isExported(cls.GoType) {
			continue
		}

		rawServiceName := exportName(cls.GoType) + "Service"
		serviceName := goNames.ResolveService(rawServiceName)
		// The protogen service name uses the same convention.
		protoServiceName := rawServiceName

		svc := ServerService{
			GoType:      cls.GoType,
			ServiceName: serviceName,
			GoImport:    merged.GoImport,
			Obtain:      cls.Obtain,
			Close:       cls.Close,
		}

		switch cls.Obtain {
		case "system_service":
			svc.InstantiationKind = "system_service"
		case "constructor":
			svc.InstantiationKind = "constructor"
			svc.NeedsHandles = true
			data.NeedsHandles = true
		}

		// For constructor classes, try to find a constructor RPC in
		// protoRPCLookup and synthesize a ServerMethod with IsConstructor.
		// If no constructor RPC exists, skip the class entirely — the
		// server has no way to instantiate the object.
		if cls.Obtain == "constructor" {
			ctorRPCName := "New" + exportName(cls.GoType)
			ctorLookupKey := protoServiceName + "/" + strings.ToLower(ctorRPCName)
			info, found := protoRPCLookup[ctorLookupKey]
			if !found {
				continue
			}
			callArgs, _ := buildCallArgs(cls.ConstructorParams)
			ctorMethod := ServerMethod{
				GoName:        info.Name,
				SpecGoName:    ctorRPCName,
				RequestType:   goNames.ResolveMessage(info.InputType),
				ResponseType:  goNames.ResolveMessage(info.OutputType),
				CallArgs:      callArgs,
				ReturnKind:    "object",
				HasError:      true,
				HasResult:     true,
				GoReturnType:  "*" + cls.GoType,
				NeedsHandles:  true,
				IsConstructor: true,
				ResultExpr:    "result",
			}
			svc.Methods = append(svc.Methods, ctorMethod)
			data.NeedsJNI = true
		}

		seenMethod := make(map[string]bool)
		for _, m := range cls.Methods {
			if !isExported(m.GoName) {
				continue
			}
			sm := buildServerMethod(m, dataClassMap, javaClassToDataClass, dcFieldMap, protoRPCLookup, protoServiceName, goNames)
			if seenMethod[sm.GoName] {
				continue
			}
			seenMethod[sm.GoName] = true
			if sm.ReturnKind == "data_class" || sm.ReturnKind == "object" {
				data.NeedsJNI = true
			}
			if sm.NeedsHandles {
				svc.NeedsHandles = true
				data.NeedsHandles = true
			}
			svc.Methods = append(svc.Methods, sm)
		}

		if len(svc.Methods) == 0 {
			continue
		}
		data.Services = append(data.Services, svc)
	}

	// Remove constructor services whose Go types don't exist in the jni package.
	var validServices []ServerService
	for _, svc := range data.Services {
		if svc.InstantiationKind == "constructor" {
			if !goTypeExistsInPackage(svc.GoImport, svc.GoType) {
				continue
			}
		}
		validServices = append(validServices, svc)
	}
	data.Services = validServices

	// Recalculate NeedsJNI/NeedsHandles after filtering.
	data.NeedsJNI = false
	data.NeedsHandles = false
	for _, svc := range data.Services {
		if svc.NeedsHandles {
			data.NeedsHandles = true
		}
		for _, m := range svc.Methods {
			if m.IsConstructor || m.ReturnKind == "data_class" || m.ReturnKind == "object" {
				data.NeedsJNI = true
			}
		}
	}

	assignImportAliases(data)
	return data
}

// goTypeExistsInPackage checks if a Go type struct is defined in the
// package at the given import path. It tries multiple resolution
// strategies: GOPATH, go.work sibling directories, and module cache.
func goTypeExistsInPackage(goImport, goType string) bool {
	// Check for both the type definition and the constructor function.
	typePattern := "type " + goType + " struct"
	ctorPattern := "func New" + goType + "("

	// Strategy 1: GOPATH/src resolution.
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(os.Getenv("HOME"), "go")
	}
	candidates := []string{
		filepath.Join(gopath, "src", strings.ReplaceAll(goImport, "/", string(filepath.Separator))),
	}

	// Strategy 2: resolve via go.work — check sibling directories.
	// If the import is "github.com/Org/mod/pkg", try "../mod/pkg" relative
	// to the current working directory.
	parts := strings.SplitN(goImport, "/", 4) // host/org/mod/pkg...
	if len(parts) >= 3 {
		modName := parts[2] // e.g., "jni"
		subPath := ""
		if len(parts) >= 4 {
			subPath = parts[3]
		}
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates,
				filepath.Join(cwd, "..", modName, subPath),
				filepath.Join(filepath.Dir(cwd), modName, subPath),
			)
		}
	}

	foundType := false
	foundCtor := false
	for _, dir := range candidates {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			continue
		}
		for _, f := range files {
			content, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			s := string(content)
			if strings.Contains(s, typePattern) {
				foundType = true
			}
			if strings.Contains(s, ctorPattern) {
				foundCtor = true
			}
		}
	}
	return foundType && foundCtor
}

// buildServerMethod converts a MergedMethod to a ServerMethod with
// pre-rendered template expressions.
func buildServerMethod(
	m javagen.MergedMethod,
	dataClassNames map[string]bool,
	javaClassToDataMsg map[string]string,
	dcFieldMap map[string][]javagen.MergedField,
	protoRPCLookup map[string]rpcInfo,
	protoServiceName string,
	goNames protoscan.GoNames,
) ServerMethod {
	rawName := exportName(m.GoName)
	goName := rawName

	// Look up the actual proto RPC info (handles collision renames).
	lookupKey := protoServiceName + "/" + strings.ToLower(rawName)
	info, found := protoRPCLookup[lookupKey]
	if found {
		goName = info.Name
	}
	goName = goNames.ResolveRPC(goName)

	// Use the actual proto message types when available, which may
	// include a class prefix for disambiguation. Then resolve through
	// protoscan to handle protoc naming quirks (e.g., P2p -> P2P).
	reqType := goName + "Request"
	respType := goName + "Response"
	if found {
		reqType = info.InputType
		respType = info.OutputType
	}
	reqType = goNames.ResolveMessage(reqType)
	respType = goNames.ResolveMessage(respType)

	sm := ServerMethod{
		GoName:       goName,
		SpecGoName:   m.GoName,
		RequestType:  reqType,
		ResponseType: respType,
		HasError:     m.HasError,
		HasResult:    m.ReturnKind != javagen.ReturnVoid,
		GoReturnType: m.GoReturn,
	}

	// Build call arguments expression.
	callArgs, argsNeedHandles := buildCallArgs(m.Params)
	sm.CallArgs = callArgs

	// Determine return kind and result expression.
	switch m.ReturnKind {
	case javagen.ReturnVoid:
		sm.ReturnKind = "void"
	case javagen.ReturnString:
		sm.ReturnKind = "string"
		sm.ResultExpr = "result"
	case javagen.ReturnBool:
		sm.ReturnKind = "bool"
		sm.ResultExpr = "result"
	case javagen.ReturnPrimitive:
		sm.ReturnKind = "primitive"
		sm.ResultExpr = convertPrimitiveExpr(m.GoReturn, "result")
	case javagen.ReturnObject:
		if dcName, ok := javaClassToDataMsg[m.Returns]; ok {
			sm.ReturnKind = "data_class"
			sm.DataClass = dcName
			// Use "extracted" variable name since the template extracts
			// from the JNI object into a Go struct via VM.Do.
			sm.DataClassConversion = buildDataClassConversion(dcName, dcFieldMap[dcName], "extracted")
		} else {
			sm.ReturnKind = "object"
			sm.ResultExpr = "result"
			argsNeedHandles = true
		}
	}

	sm.NeedsHandles = argsNeedHandles

	return sm
}

// buildCallArgs generates the Go expression for the call arguments and
// reports whether any argument requires the handle store.
func buildCallArgs(params []javagen.MergedParam) (string, bool) {
	if len(params) == 0 {
		return "", false
	}
	needsHandles := false
	var parts []string
	for _, p := range params {
		switch {
		case p.JavaType == "android.content.Context":
			// The server's own Context is used for android.content.Context
			// parameters, so the client does not need to supply a handle.
			parts = append(parts, "s.Ctx.Obj")
		case p.IsObject && !p.IsString:
			// Object handles are looked up in the HandleStore.
			protoGetter := fmt.Sprintf("req.Get%s()", exportName(p.GoName))
			parts = append(parts, fmt.Sprintf("s.Handles.Get(%s)", protoGetter))
			needsHandles = true
		default:
			// Strings, bools, and primitives use the proto getter.
			protoGetter := fmt.Sprintf("req.Get%s()", exportName(p.GoName))
			parts = append(parts, convertProtoToGo(p.GoType, protoGetter))
		}
	}
	return strings.Join(parts, ", "), needsHandles
}

// convertProtoToGo wraps a proto getter expression with a type cast if needed.
func convertProtoToGo(goType, expr string) string {
	switch goType {
	case "string", "bool", "int32", "int64", "float32", "float64":
		return expr
	case "int8":
		return fmt.Sprintf("int8(%s)", expr)
	case "int16":
		return fmt.Sprintf("int16(%s)", expr)
	case "uint16":
		return fmt.Sprintf("uint16(%s)", expr)
	case "int":
		return fmt.Sprintf("int(%s)", expr)
	case "byte":
		return fmt.Sprintf("byte(%s)", expr)
	default:
		return expr
	}
}

// convertPrimitiveExpr returns the expression to convert a Go primitive to the
// proto field type.
func convertPrimitiveExpr(goType, varName string) string {
	switch goType {
	case "int32", "int64", "float32", "float64", "bool", "string":
		return varName
	case "int":
		return fmt.Sprintf("int32(%s)", varName)
	case "int8":
		return fmt.Sprintf("uint32(%s)", varName)
	case "int16", "uint16":
		return fmt.Sprintf("int32(%s)", varName)
	case "byte":
		return fmt.Sprintf("uint32(%s)", varName)
	default:
		return varName
	}
}

// buildDataClassConversion renders the Go expression that converts a javagen
// data class struct to a proto message pointer.
func buildDataClassConversion(
	dcName string,
	fields []javagen.MergedField,
	varName string,
) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "&pb.%s{\n", dcName)
	for _, f := range fields {
		protoFieldName := protoGoFieldName(f.GoName)
		protoType := protoGoType(f.CallSuffix, f.GoType)
		valueExpr := dataClassFieldExpr(f.GoType, protoType, varName+"."+f.GoName)
		fmt.Fprintf(&buf, "\t\t%s: %s,\n", protoFieldName, valueExpr)
	}
	fmt.Fprintf(&buf, "\t}")
	return buf.String()
}

// protoGoType returns the Go type of a proto field given the JNI call suffix
// and the javagen Go type.
func protoGoType(callSuffix, goType string) string {
	switch callSuffix {
	case "Boolean":
		return "bool"
	case "Byte":
		return "uint32"
	case "Short", "Int":
		return "int32"
	case "Long":
		return "int64"
	case "Float":
		return "float32"
	case "Double":
		return "float64"
	case "Object":
		if goType == "string" {
			return "string"
		}
		return "int64" // object handle
	default:
		return "int32"
	}
}

// dataClassFieldExpr returns the Go expression to assign a javagen field value
// to a proto message field, with type casting if needed.
func dataClassFieldExpr(
	goType string,
	protoType string,
	expr string,
) string {
	if goType == protoType {
		return expr
	}
	// Need type cast.
	return fmt.Sprintf("%s(%s)", protoType, expr)
}

// protoGoFieldName converts a javagen Go field name to the corresponding
// protobuf-generated Go struct field name. The path is:
//
//	GoName -> toSnakeCase -> protobuf CamelCase
//
// Examples: "SSID" -> "ssid" -> "Ssid", "IPAddress" -> "ip_address" -> "IpAddress"
func protoGoFieldName(goName string) string {
	snake := toSnakeCase(goName)
	return protoCamelCase(snake)
}

// toSnakeCase converts a PascalCase or camelCase string to snake_case.
func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := rune(s[i-1])
				switch {
				case prev >= 'a' && prev <= 'z':
					b.WriteByte('_')
				case prev >= 'A' && prev <= 'Z' && i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z':
					b.WriteByte('_')
				}
			}
			b.WriteRune(r - 'A' + 'a')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// protoCamelCase converts a snake_case string to the CamelCase used by
// protoc-gen-go. Each underscore-separated word gets its first letter
// uppercased; the rest stay lowercase.
func protoCamelCase(s string) string {
	var buf strings.Builder
	upper := true
	for _, r := range s {
		if r == '_' {
			upper = true
			continue
		}
		if upper && r >= 'a' && r <= 'z' {
			buf.WriteRune(r - 'a' + 'A')
		} else {
			buf.WriteRune(r)
		}
		upper = false
	}
	return buf.String()
}


// isExported reports whether a Go name is exported (starts with uppercase).
func isExported(name string) bool {
	if name == "" {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}

// exportName capitalizes the first letter of a Go name (as protoc-gen-go does).
func exportName(s string) string {
	if s == "" {
		return ""
	}
	first := s[0]
	if first >= 'a' && first <= 'z' {
		return string(first-'a'+'A') + s[1:]
	}
	return s
}
