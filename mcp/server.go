package mcp

import (
	"context"
	"log/slog"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
)

// Server wraps the go-sdk MCP server, providing tool and resource
// registrations that bridge MCP clients to the jniservice gRPC backend.
type Server struct {
	mcp  *gomcp.Server
	conn *grpc.ClientConn
	log  *slog.Logger
}

// NewServer creates the MCP server and registers all tools and resources.
func NewServer(conn *grpc.ClientConn, log *slog.Logger) *Server {
	mcpServer := gomcp.NewServer(
		&gomcp.Implementation{Name: "jnimcp", Version: "0.1.0"},
		&gomcp.ServerOptions{
			Instructions: "MCP server for Android device interaction via JNI. " +
				"Use workflow tools for common operations, call_android_api for any gRPC method, " +
				"or jni_raw for direct JNI access.",
			Logger: log,
		},
	)

	s := &Server{
		mcp:  mcpServer,
		conn: conn,
		log:  log,
	}

	s.registerResources()
	s.registerWorkflowTools()
	s.registerGenericTool()
	s.registerRawJNITool()

	return s
}

// MCPServer returns the underlying go-sdk MCP server for transport wiring.
func (s *Server) MCPServer() *gomcp.Server {
	return s.mcp
}

// Run starts the MCP server with the stdio transport, blocking until the
// context is cancelled or the client disconnects.
func (s *Server) Run(ctx context.Context) error {
	return s.mcp.Run(ctx, &gomcp.StdioTransport{})
}
