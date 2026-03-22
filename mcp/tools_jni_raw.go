package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	handlestorepb "github.com/AndroidGoLab/jni-proxy/proto/handlestore"
	jnirawpb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// jniRawInput is the top-level input for the jni_raw tool.
type jniRawInput struct {
	Operations []jniOp `json:"operations" jsonschema:"Batch of JNI operations to execute sequentially. Each operation result is available to subsequent operations."`
}

// jniOp describes a single JNI operation within a batch.
type jniOp struct {
	Op   string          `json:"op" jsonschema:"Operation name: find_class, get_method_id, get_static_method_id, get_field_id, get_static_field_id, call_method, call_static_method, call_nonvirtual_method, get_field, set_field, get_static_field, set_static_field, new_object, alloc_object, new_string_utf, get_string_utf_chars, get_string_length, new_primitive_array, new_object_array, get_array_length, get_object_array_element, set_object_array_element, get_byte_array_data, exception_check, exception_clear, exception_occurred, get_object_class, is_instance_of, is_same_object, get_version, get_app_context, release_handle, monitor_enter, monitor_exit"`
	Args json.RawMessage `json:"args" jsonschema:"Operation-specific arguments as JSON object"`
}

func (s *Server) registerRawJNITool() {
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "jni_raw",
		Description: "Execute raw JNI operations on the Android device. Accepts a batch " +
			"of operations executed sequentially. ADVANCED: requires jni_raw.* ACL grant on jniservice.",
		Annotations: &gomcp.ToolAnnotations{
			DestructiveHint: nil, // defaults to true
			Title:           "Raw JNI",
		},
	}, s.handleJNIRaw)
}

func (s *Server) handleJNIRaw(
	ctx context.Context,
	req *gomcp.CallToolRequest,
	input jniRawInput,
) (*gomcp.CallToolResult, any, error) {
	if len(input.Operations) == 0 {
		return nil, nil, fmt.Errorf("at least one operation is required")
	}

	jniClient := jnirawpb.NewJNIServiceClient(s.conn)
	hsClient := handlestorepb.NewHandleStoreServiceClient(s.conn)

	results := make([]any, 0, len(input.Operations))
	for i, op := range input.Operations {
		res, err := s.execJNIOp(ctx, jniClient, hsClient, op)
		if err != nil {
			// Return partial results plus the error.
			results = append(results, map[string]any{
				"error": fmt.Sprintf("op[%d] %s: %v", i, op.Op, err),
			})
			return jsonResultWithError(results)
		}
		results = append(results, res)
	}

	return jsonResultRaw(results)
}

// jsonResultRaw marshals the results array as indented JSON.
func jsonResultRaw(v any) (*gomcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", err)
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// jsonResultWithError returns an isError result.
func jsonResultWithError(v any) (*gomcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("marshal result: %w", err)
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}},
		IsError: true,
	}, nil, nil
}

//nolint:cyclop // dispatch table for JNI operations
func (s *Server) execJNIOp(
	ctx context.Context,
	jni jnirawpb.JNIServiceClient,
	hs handlestorepb.HandleStoreServiceClient,
	op jniOp,
) (any, error) {
	switch op.Op {
	// ---- Version ----
	case "get_version":
		resp, err := jni.GetVersion(ctx, &jnirawpb.GetVersionRequest{})
		if err != nil {
			return nil, err
		}
		return map[string]any{"version": resp.GetVersion()}, nil

	// ---- Class ----
	case "find_class":
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.FindClass(ctx, &jnirawpb.FindClassRequest{Name: args.Name})
		if err != nil {
			return nil, err
		}
		return map[string]any{"class_handle": resp.GetClassHandle()}, nil

	// ---- Method/Field ID lookup ----
	case "get_method_id":
		var args struct {
			ClassHandle int64  `json:"class_handle"`
			Name        string `json:"name"`
			Sig         string `json:"sig"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetMethodID(ctx, &jnirawpb.GetMethodIDRequest{
			ClassHandle: args.ClassHandle,
			Name:        args.Name,
			Sig:         args.Sig,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"method_id": resp.GetMethodId()}, nil

	case "get_static_method_id":
		var args struct {
			ClassHandle int64  `json:"class_handle"`
			Name        string `json:"name"`
			Sig         string `json:"sig"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetStaticMethodID(ctx, &jnirawpb.GetStaticMethodIDRequest{
			ClassHandle: args.ClassHandle,
			Name:        args.Name,
			Sig:         args.Sig,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"method_id": resp.GetMethodId()}, nil

	case "get_field_id":
		var args struct {
			ClassHandle int64  `json:"class_handle"`
			Name        string `json:"name"`
			Sig         string `json:"sig"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetFieldID(ctx, &jnirawpb.GetFieldIDRequest{
			ClassHandle: args.ClassHandle,
			Name:        args.Name,
			Sig:         args.Sig,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"field_id": resp.GetFieldId()}, nil

	case "get_static_field_id":
		var args struct {
			ClassHandle int64  `json:"class_handle"`
			Name        string `json:"name"`
			Sig         string `json:"sig"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetStaticFieldID(ctx, &jnirawpb.GetStaticFieldIDRequest{
			ClassHandle: args.ClassHandle,
			Name:        args.Name,
			Sig:         args.Sig,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"field_id": resp.GetFieldId()}, nil

	// ---- Method calls ----
	case "call_method":
		var args struct {
			ObjectHandle int64      `json:"object_handle"`
			MethodID     int64      `json:"method_id"`
			ReturnType   string     `json:"return_type"`
			Args         []jvalJSON `json:"args"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		pbArgs, err := jvalSliceToPB(args.Args)
		if err != nil {
			return nil, err
		}
		resp, err := jni.CallMethod(ctx, &jnirawpb.CallMethodRequest{
			ObjectHandle: args.ObjectHandle,
			MethodId:     args.MethodID,
			ReturnType:   parseJType(args.ReturnType),
			Args:         pbArgs,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": jvalToJSON(resp.GetResult())}, nil

	case "call_static_method":
		var args struct {
			ClassHandle int64      `json:"class_handle"`
			MethodID    int64      `json:"method_id"`
			ReturnType  string     `json:"return_type"`
			Args        []jvalJSON `json:"args"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		pbArgs, err := jvalSliceToPB(args.Args)
		if err != nil {
			return nil, err
		}
		resp, err := jni.CallStaticMethod(ctx, &jnirawpb.CallStaticMethodRequest{
			ClassHandle: args.ClassHandle,
			MethodId:    args.MethodID,
			ReturnType:  parseJType(args.ReturnType),
			Args:        pbArgs,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": jvalToJSON(resp.GetResult())}, nil

	case "call_nonvirtual_method":
		var args struct {
			ObjectHandle int64      `json:"object_handle"`
			ClassHandle  int64      `json:"class_handle"`
			MethodID     int64      `json:"method_id"`
			ReturnType   string     `json:"return_type"`
			Args         []jvalJSON `json:"args"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		pbArgs, err := jvalSliceToPB(args.Args)
		if err != nil {
			return nil, err
		}
		resp, err := jni.CallNonvirtualMethod(ctx, &jnirawpb.CallNonvirtualMethodRequest{
			ObjectHandle: args.ObjectHandle,
			ClassHandle:  args.ClassHandle,
			MethodId:     args.MethodID,
			ReturnType:   parseJType(args.ReturnType),
			Args:         pbArgs,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": jvalToJSON(resp.GetResult())}, nil

	// ---- Field access ----
	case "get_field":
		var args struct {
			ObjectHandle int64  `json:"object_handle"`
			FieldID      int64  `json:"field_id"`
			FieldType    string `json:"field_type"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetField(ctx, &jnirawpb.GetFieldValueRequest{
			ObjectHandle: args.ObjectHandle,
			FieldId:      args.FieldID,
			FieldType:    parseJType(args.FieldType),
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": jvalToJSON(resp.GetResult())}, nil

	case "set_field":
		var args struct {
			ObjectHandle int64    `json:"object_handle"`
			FieldID      int64    `json:"field_id"`
			Value        jvalJSON `json:"value"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		pbVal, err := args.Value.toPB()
		if err != nil {
			return nil, err
		}
		_, err = jni.SetField(ctx, &jnirawpb.SetFieldValueRequest{
			ObjectHandle: args.ObjectHandle,
			FieldId:      args.FieldID,
			Value:        pbVal,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil

	case "get_static_field":
		var args struct {
			ClassHandle int64  `json:"class_handle"`
			FieldID     int64  `json:"field_id"`
			FieldType   string `json:"field_type"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetStaticField(ctx, &jnirawpb.GetStaticFieldValueRequest{
			ClassHandle: args.ClassHandle,
			FieldId:     args.FieldID,
			FieldType:   parseJType(args.FieldType),
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": jvalToJSON(resp.GetResult())}, nil

	case "set_static_field":
		var args struct {
			ClassHandle int64    `json:"class_handle"`
			FieldID     int64    `json:"field_id"`
			Value       jvalJSON `json:"value"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		pbVal, err := args.Value.toPB()
		if err != nil {
			return nil, err
		}
		_, err = jni.SetStaticField(ctx, &jnirawpb.SetStaticFieldValueRequest{
			ClassHandle: args.ClassHandle,
			FieldId:     args.FieldID,
			Value:       pbVal,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil

	// ---- Object ----
	case "new_object":
		var args struct {
			ClassHandle int64      `json:"class_handle"`
			MethodID    int64      `json:"method_id"`
			Args        []jvalJSON `json:"args"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		pbArgs, err := jvalSliceToPB(args.Args)
		if err != nil {
			return nil, err
		}
		resp, err := jni.NewObject(ctx, &jnirawpb.NewObjectRequest{
			ClassHandle: args.ClassHandle,
			MethodId:    args.MethodID,
			Args:        pbArgs,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"object_handle": resp.GetObjectHandle()}, nil

	case "alloc_object":
		var args struct {
			ClassHandle int64 `json:"class_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.AllocObject(ctx, &jnirawpb.AllocObjectRequest{
			ClassHandle: args.ClassHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"object_handle": resp.GetObjectHandle()}, nil

	case "get_object_class":
		var args struct {
			ObjectHandle int64 `json:"object_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetObjectClass(ctx, &jnirawpb.GetObjectClassRequest{
			ObjectHandle: args.ObjectHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"class_handle": resp.GetClassHandle()}, nil

	case "is_instance_of":
		var args struct {
			ObjectHandle int64 `json:"object_handle"`
			ClassHandle  int64 `json:"class_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.IsInstanceOf(ctx, &jnirawpb.IsInstanceOfRequest{
			ObjectHandle: args.ObjectHandle,
			ClassHandle:  args.ClassHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": resp.GetResult()}, nil

	case "is_same_object":
		var args struct {
			Object1 int64 `json:"object1"`
			Object2 int64 `json:"object2"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.IsSameObject(ctx, &jnirawpb.IsSameObjectRequest{
			Object1: args.Object1,
			Object2: args.Object2,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": resp.GetResult()}, nil

	// ---- String ----
	case "new_string_utf":
		var args struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.NewStringUTF(ctx, &jnirawpb.NewStringUTFRequest{Value: args.Value})
		if err != nil {
			return nil, err
		}
		return map[string]any{"string_handle": resp.GetStringHandle()}, nil

	case "get_string_utf_chars":
		var args struct {
			StringHandle int64 `json:"string_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetStringUTFChars(ctx, &jnirawpb.GetStringUTFCharsRequest{
			StringHandle: args.StringHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"value": resp.GetValue()}, nil

	case "get_string_length":
		var args struct {
			StringHandle int64 `json:"string_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetStringLength(ctx, &jnirawpb.GetStringLengthRequest{
			StringHandle: args.StringHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"length": resp.GetLength()}, nil

	// ---- Array ----
	case "new_primitive_array":
		var args struct {
			ElementType string `json:"element_type"`
			Length      int32  `json:"length"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.NewPrimitiveArray(ctx, &jnirawpb.NewPrimitiveArrayRequest{
			ElementType: parseJType(args.ElementType),
			Length:      args.Length,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"array_handle": resp.GetArrayHandle()}, nil

	case "new_object_array":
		var args struct {
			Length      int32 `json:"length"`
			ClassHandle int64 `json:"class_handle"`
			InitElement int64 `json:"init_element"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.NewObjectArray(ctx, &jnirawpb.NewObjectArrayRequest{
			Length:      args.Length,
			ClassHandle: args.ClassHandle,
			InitElement: args.InitElement,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"array_handle": resp.GetArrayHandle()}, nil

	case "get_array_length":
		var args struct {
			ArrayHandle int64 `json:"array_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetArrayLength(ctx, &jnirawpb.GetArrayLengthRequest{
			ArrayHandle: args.ArrayHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"length": resp.GetLength()}, nil

	case "get_object_array_element":
		var args struct {
			ArrayHandle int64 `json:"array_handle"`
			Index       int32 `json:"index"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetObjectArrayElement(ctx, &jnirawpb.GetObjectArrayElementRequest{
			ArrayHandle: args.ArrayHandle,
			Index:       args.Index,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"element_handle": resp.GetElementHandle()}, nil

	case "set_object_array_element":
		var args struct {
			ArrayHandle   int64 `json:"array_handle"`
			Index         int32 `json:"index"`
			ElementHandle int64 `json:"element_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		_, err := jni.SetObjectArrayElement(ctx, &jnirawpb.SetObjectArrayElementRequest{
			ArrayHandle:   args.ArrayHandle,
			Index:         args.Index,
			ElementHandle: args.ElementHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil

	case "get_byte_array_data":
		var args struct {
			ArrayHandle int64 `json:"array_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		resp, err := jni.GetByteArrayData(ctx, &jnirawpb.GetByteArrayDataRequest{
			ArrayHandle: args.ArrayHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"data_base64": base64.StdEncoding.EncodeToString(resp.GetData()),
			"length":      len(resp.GetData()),
		}, nil

	// ---- Exception ----
	case "exception_check":
		resp, err := jni.ExceptionCheck(ctx, &jnirawpb.ExceptionCheckRequest{})
		if err != nil {
			return nil, err
		}
		return map[string]any{"has_exception": resp.GetHasException()}, nil

	case "exception_clear":
		_, err := jni.ExceptionClear(ctx, &jnirawpb.ExceptionClearRequest{})
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil

	case "exception_occurred":
		resp, err := jni.ExceptionOccurred(ctx, &jnirawpb.ExceptionOccurredRequest{})
		if err != nil {
			return nil, err
		}
		return map[string]any{"throwable_handle": resp.GetThrowableHandle()}, nil

	// ---- App context ----
	case "get_app_context":
		resp, err := jni.GetAppContext(ctx, &jnirawpb.GetAppContextRequest{})
		if err != nil {
			return nil, err
		}
		return map[string]any{"context_handle": resp.GetContextHandle()}, nil

	// ---- Handle release (via HandleStoreService) ----
	case "release_handle":
		var args struct {
			Handle int64 `json:"handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		_, err := hs.ReleaseHandle(ctx, &handlestorepb.ReleaseHandleRequest{
			Handle: args.Handle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil

	// ---- Monitor ----
	case "monitor_enter":
		var args struct {
			ObjectHandle int64 `json:"object_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		_, err := jni.MonitorEnter(ctx, &jnirawpb.MonitorEnterRequest{
			ObjectHandle: args.ObjectHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil

	case "monitor_exit":
		var args struct {
			ObjectHandle int64 `json:"object_handle"`
		}
		if err := json.Unmarshal(op.Args, &args); err != nil {
			return nil, fmt.Errorf("unmarshal args: %w", err)
		}
		_, err := jni.MonitorExit(ctx, &jnirawpb.MonitorExitRequest{
			ObjectHandle: args.ObjectHandle,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil

	default:
		return nil, fmt.Errorf("unknown JNI operation: %q", op.Op)
	}
}

// ---------------------------------------------------------------------------
// JValue JSON helpers
// ---------------------------------------------------------------------------

// jvalJSON is a JSON-friendly representation of a JNI value.
// Exactly one field should be set.
type jvalJSON struct {
	Z *bool    `json:"z,omitempty"` // boolean
	B *int32   `json:"b,omitempty"` // byte
	C *uint32  `json:"c,omitempty"` // char
	S *int32   `json:"s,omitempty"` // short
	I *int32   `json:"i,omitempty"` // int
	J *int64   `json:"j,omitempty"` // long
	F *float32 `json:"f,omitempty"` // float
	D *float64 `json:"d,omitempty"` // double
	L *int64   `json:"l,omitempty"` // object handle
}

func (v jvalJSON) toPB() (*jnirawpb.JValue, error) {
	switch {
	case v.Z != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_Z{Z: *v.Z}}, nil
	case v.B != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_B{B: *v.B}}, nil
	case v.C != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_C{C: *v.C}}, nil
	case v.S != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_S{S: *v.S}}, nil
	case v.I != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_I{I: *v.I}}, nil
	case v.J != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_J{J: *v.J}}, nil
	case v.F != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_F{F: *v.F}}, nil
	case v.D != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_D{D: *v.D}}, nil
	case v.L != nil:
		return &jnirawpb.JValue{Value: &jnirawpb.JValue_L{L: *v.L}}, nil
	default:
		return &jnirawpb.JValue{}, nil
	}
}

func jvalSliceToPB(vals []jvalJSON) ([]*jnirawpb.JValue, error) {
	out := make([]*jnirawpb.JValue, len(vals))
	for i, v := range vals {
		pb, err := v.toPB()
		if err != nil {
			return nil, fmt.Errorf("arg[%d]: %w", i, err)
		}
		out[i] = pb
	}
	return out, nil
}

func jvalToJSON(v *jnirawpb.JValue) any {
	if v == nil {
		return nil
	}
	switch val := v.Value.(type) {
	case *jnirawpb.JValue_Z:
		return map[string]any{"z": val.Z}
	case *jnirawpb.JValue_B:
		return map[string]any{"b": val.B}
	case *jnirawpb.JValue_C:
		return map[string]any{"c": val.C}
	case *jnirawpb.JValue_S:
		return map[string]any{"s": val.S}
	case *jnirawpb.JValue_I:
		return map[string]any{"i": val.I}
	case *jnirawpb.JValue_J:
		return map[string]any{"j": val.J}
	case *jnirawpb.JValue_F:
		return map[string]any{"f": val.F}
	case *jnirawpb.JValue_D:
		return map[string]any{"d": val.D}
	case *jnirawpb.JValue_L:
		return map[string]any{"l": val.L}
	default:
		return nil
	}
}

// parseJType converts a string like "INT", "OBJECT", etc. to the proto enum.
func parseJType(s string) jnirawpb.JType {
	if v, ok := jnirawpb.JType_value[s]; ok {
		return jnirawpb.JType(v)
	}
	return jnirawpb.JType_VOID
}
