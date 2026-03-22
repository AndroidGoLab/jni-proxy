package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

type callAndroidAPIInput struct {
	Service string          `json:"service" jsonschema:"Service key (e.g., 'battery/ManagerService'). Use jni://services resource to list available services."`
	Method  string          `json:"method" jsonschema:"RPC method name (e.g., 'GetIntProperty'). Use jni://services/{service} resource to list methods."`
	Params  json.RawMessage `json:"params,omitempty" jsonschema:"JSON object with method parameters (e.g., {\"arg0\": 4})"`
}

func (s *Server) registerGenericTool() {
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "call_android_api",
		Description: "Invoke any Android system service method via gRPC. " +
			"Use the jni://services resource to discover available services and " +
			"jni://services/{service} to list methods. " +
			"Parameters are passed as a JSON object with positional keys (arg0, arg1, etc.).",
		Annotations: &gomcp.ToolAnnotations{
			Title: "Call Android API",
		},
	}, s.handleCallAndroidAPI)
}

func (s *Server) handleCallAndroidAPI(
	ctx context.Context,
	req *gomcp.CallToolRequest,
	input callAndroidAPIInput,
) (*gomcp.CallToolResult, any, error) {
	// Validate inputs.
	if input.Service == "" {
		return nil, nil, fmt.Errorf("service is required")
	}
	if input.Method == "" {
		return nil, nil, fmt.Errorf("method is required")
	}

	// Convert service key "battery/ManagerService" to proto full service
	// name "battery.ManagerService".
	protoService := serviceKeyToProtoName(input.Service)

	// Look up the service descriptor in the global proto registry.
	svcDesc, err := findServiceDescriptor(protoService)
	if err != nil {
		return nil, nil, fmt.Errorf("service %q: %w", input.Service, err)
	}

	// Find the method descriptor.
	methodDesc := svcDesc.Methods().ByName(protoreflect.Name(input.Method))
	if methodDesc == nil {
		var available []string
		for i := 0; i < svcDesc.Methods().Len(); i++ {
			available = append(available, string(svcDesc.Methods().Get(i).Name()))
		}
		return nil, nil, fmt.Errorf(
			"method %q not found in service %q; available methods: %s",
			input.Method, input.Service, strings.Join(available, ", "),
		)
	}

	// Build the request message from the user's JSON params.
	reqMsg := dynamicpb.NewMessage(methodDesc.Input())
	if len(input.Params) > 0 {
		if err := protojson.Unmarshal(input.Params, reqMsg); err != nil {
			return nil, nil, fmt.Errorf("unmarshal params into %s: %w", methodDesc.Input().FullName(), err)
		}
	}

	// Build the gRPC full method path: /{package}.{Service}/{Method}
	fullMethod := fmt.Sprintf("/%s/%s", svcDesc.FullName(), methodDesc.Name())

	// Invoke the gRPC method.
	respMsg := dynamicpb.NewMessage(methodDesc.Output())
	if err := s.conn.Invoke(ctx, fullMethod, reqMsg, respMsg, grpc.StaticMethod()); err != nil {
		return nil, nil, fmt.Errorf("gRPC invoke %s: %w", fullMethod, err)
	}

	// Marshal response back to JSON.
	marshaler := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: true,
	}
	respJSON, err := marshaler.Marshal(respMsg)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal response: %w", err)
	}

	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(respJSON)}},
	}, nil, nil
}

// serviceKeyToProtoName converts a service registry key like
// "battery/ManagerService" to the proto full service name
// "battery.ManagerService".
func serviceKeyToProtoName(key string) string {
	idx := strings.Index(key, "/")
	if idx < 0 {
		return key
	}
	return key[:idx] + "." + key[idx+1:]
}

// findServiceDescriptor looks up a service by its proto full name in the
// global file registry.
func findServiceDescriptor(fullName string) (protoreflect.ServiceDescriptor, error) {
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(fullName))
	if err != nil {
		return nil, fmt.Errorf("proto service %q not found in registry: %w", fullName, err)
	}
	svcDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is a %T, not a service descriptor", fullName, desc)
	}
	return svcDesc, nil
}
