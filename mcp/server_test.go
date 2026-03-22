package mcp_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	mcpserver "github.com/AndroidGoLab/jni-proxy/mcp"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestServerToolsAndResources(t *testing.T) {
	// Create dummy gRPC conn (not used for tool/resource listing).
	conn, err := grpc.NewClient("passthrough:///dummy",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	// Create our MCP server wrapper.
	log := slog.Default()
	srv := mcpserver.NewServer(conn, log)

	// Create in-memory transports. Server must be connected first.
	clientTransport, serverTransport := gomcp.NewInMemoryTransports()

	ctx := context.Background()

	// Connect the MCP server to the server-side transport.
	serverSession, err := srv.MCPServer().Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	// Create and connect the MCP client to the client-side transport.
	client := gomcp.NewClient(
		&gomcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	// --- List tools ---
	toolsResult, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)

	// 52 workflow + 1 generic (call_android_api) + 1 raw (jni_raw) = 54
	require.Len(t, toolsResult.Tools, 54,
		"expected 54 tools (52 workflow + 1 generic + 1 raw)")

	toolNames := make(map[string]bool, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		toolNames[tool.Name] = true
	}
	require.True(t, toolNames["get_battery_status"],
		"workflow tool get_battery_status must be registered")
	require.True(t, toolNames["call_android_api"],
		"generic tool call_android_api must be registered")
	require.True(t, toolNames["jni_raw"],
		"raw JNI tool jni_raw must be registered")

	// --- List resources ---
	resourcesResult, err := clientSession.ListResources(ctx, nil)
	require.NoError(t, err)
	require.Len(t, resourcesResult.Resources, 1,
		"expected exactly 1 static resource (jni://services)")
	require.Equal(t, "jni://services", resourcesResult.Resources[0].URI)

	// --- Read the services resource ---
	readResult, err := clientSession.ReadResource(ctx,
		&gomcp.ReadResourceParams{URI: "jni://services"})
	require.NoError(t, err)
	require.Len(t, readResult.Contents, 1, "expected 1 content block")
	require.NotEmpty(t, readResult.Contents[0].Text,
		"services resource must return non-empty text")

	// The text should be valid JSON (an array of service names).
	var services []string
	err = json.Unmarshal([]byte(readResult.Contents[0].Text), &services)
	require.NoError(t, err, "services resource must return valid JSON array")
	require.Greater(t, len(services), 0,
		"services list must contain at least one entry")
}
