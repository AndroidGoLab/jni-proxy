package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	mcpserver "github.com/AndroidGoLab/jni-proxy/mcp"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const maxGRPCRecvMsgSize = 128 * 1024 * 1024

var (
	flagAddr      string
	flagTransport string
	flagHTTPPort  string
	flagHTTPAddr  string
	flagCert      string
	flagKey       string
	flagCA        string
	flagConfigDir string
	flagInsecure  bool
)

var rootCmd = &cobra.Command{
	Use:   "jnimcp",
	Short: "MCP server for Android device interaction via JNI",
	Long: "jnimcp exposes Android system services over the Model Context Protocol (MCP), " +
		"bridging MCP clients to the jniservice gRPC backend.",
	RunE: run,
}

func init() {
	rootCmd.Flags().StringVar(&flagAddr, "addr", "localhost:50051", "jniservice gRPC address")
	rootCmd.Flags().StringVar(&flagTransport, "transport", "stdio", "MCP transport: stdio or http")
	rootCmd.Flags().StringVar(&flagHTTPPort, "http-port", "8080", "HTTP listen port (when transport=http)")
	rootCmd.Flags().StringVar(&flagHTTPAddr, "http-addr", "127.0.0.1", "HTTP listen address (when transport=http)")
	rootCmd.Flags().StringVar(&flagCert, "cert", "", "client certificate PEM file (for mTLS)")
	rootCmd.Flags().StringVar(&flagKey, "key", "", "client private key PEM file (for mTLS)")
	rootCmd.Flags().StringVar(&flagCA, "ca", "", "CA certificate PEM file (for mTLS)")
	rootCmd.Flags().StringVar(&flagConfigDir, "config-dir", "", "certificate storage directory (used by auto-enrollment, default ~/.config/jnimcp)")
	rootCmd.Flags().BoolVar(&flagInsecure, "insecure", false, "skip TLS server certificate verification")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, _ []string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	conn, err := dialGRPC()
	if err != nil {
		return fmt.Errorf("connecting to jniservice: %w", err)
	}
	defer conn.Close()

	srv := mcpserver.NewServer(conn, log)

	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer cancel()

	switch flagTransport {
	case "stdio":
		log.Info("starting MCP server", "transport", "stdio", "grpc_addr", flagAddr)
		return srv.Run(ctx)

	case "http":
		listenAddr := fmt.Sprintf("%s:%s", flagHTTPAddr, flagHTTPPort)
		log.Info("starting MCP server", "transport", "http", "listen", listenAddr, "grpc_addr", flagAddr)

		handler := gomcp.NewStreamableHTTPHandler(
			func(_ *http.Request) *gomcp.Server { return srv.MCPServer() },
			&gomcp.StreamableHTTPOptions{
				SessionTimeout: 30 * time.Minute,
				Logger:         log,
			},
		)

		httpServer := &http.Server{
			Addr:    listenAddr,
			Handler: handler,
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- httpServer.ListenAndServe()
		}()

		select {
		case <-ctx.Done():
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			return httpServer.Shutdown(shutdownCtx)
		case err := <-errCh:
			return err
		}

	default:
		return fmt.Errorf("unknown transport %q (expected stdio or http)", flagTransport)
	}
}

func dialGRPC() (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	opts = append(opts,
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxGRPCRecvMsgSize)),
	)

	switch {
	case flagCert != "" && flagKey != "":
		tlsCreds, err := buildMTLSCredentials()
		if err != nil {
			return nil, fmt.Errorf("setting up mTLS: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(tlsCreds))
	case flagInsecure:
		// TLS with InsecureSkipVerify — connects over TLS but does not verify
		// the server certificate. Required for self-signed CA (jniservice
		// generates its own CA). This is NOT plaintext.
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(
			&tls.Config{InsecureSkipVerify: true})))
	default:
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return grpc.NewClient(flagAddr, opts...)
}

func buildMTLSCredentials() (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(flagCert, flagKey)
	if err != nil {
		return nil, fmt.Errorf("loading client cert/key: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	if flagCA != "" {
		caPEM, err := os.ReadFile(flagCA)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", flagCA)
		}
		tlsCfg.RootCAs = caPool
	}

	return credentials.NewTLS(tlsCfg), nil
}
