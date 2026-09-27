package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const maxGRPCRecvMsgSize = 128 * 1024 * 1024

var (
	flagAddr      string
	flagAuthority string
	flagInsecure  bool
	flagTimeout   time.Duration
	flagFormat    string
	flagCert      string
	flagKey       string
	flagCA        string
	flagProfile   string
)

var grpcConn *grpc.ClientConn

var rootCmd = &cobra.Command{
	Use:   "jnicli",
	Short: "CLI for Android API access over gRPC",
	Long:  "jnicli provides command-line access to Android system services via the go-jni gRPC layer.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		switch cmd.Name() {
		case "help", "completion", "list-commands":
			return nil
		}
		// Keep private-IPv4 enforcement on the K80 direct-LAN service route only.
		if flagAuthority == "jniservice" {
			if err := validateDirectLANAddress(flagAddr); err != nil {
				return err
			}
		}

		parentName := ""
		if parent := cmd.Parent(); parent != nil {
			parentName = parent.Name()
		}
		if err := applyDefaultCredentialFlags(cmd.Name(), parentName); err != nil {
			return err
		}

		var opts []grpc.DialOption
		// Allow large messages for binary data transfer (camera photos, videos).
		opts = append(opts,
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxGRPCRecvMsgSize)),
		)

		switch {
		case flagCert != "" && flagKey != "":
			tlsCreds, err := buildMTLSCredentials()
			if err != nil {
				return fmt.Errorf("setting up mTLS: %w", err)
			}
			opts = append(opts, grpc.WithTransportCredentials(tlsCreds))
		case flagInsecure:
			// TLS with InsecureSkipVerify — connects to the server over TLS but
			// skips both server-certificate chain and hostname verification.
			// Required only for explicitly unverified self-signed endpoints.
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(
				&tls.Config{InsecureSkipVerify: true})))
		default:
			// No TLS configuration provided — use plaintext.
			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
		if flagAuthority != "" {
			opts = append(opts, grpc.WithAuthority(flagAuthority))
		}

		conn, err := grpc.NewClient(flagAddr, opts...)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", flagAddr, err)
		}
		grpcConn = conn
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if grpcConn != nil {
			return grpcConn.Close()
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(
		&flagAddr,
		"addr",
		"a",
		"localhost:50051",
		"gRPC server address (jniservice requires a private IPv4 HOST:PORT)",
	)
	rootCmd.PersistentFlags().StringVar(&flagAuthority, "authority", "", "TLS server name and gRPC :authority override")
	rootCmd.PersistentFlags().BoolVar(&flagInsecure, "insecure", false, "skip TLS server certificate chain and hostname verification")
	rootCmd.PersistentFlags().DurationVar(&flagTimeout, "timeout", 10*time.Second, "request timeout")
	rootCmd.PersistentFlags().StringVarP(&flagFormat, "format", "f", "json", "output format (json|text)")
	rootCmd.PersistentFlags().StringVar(&flagCert, "cert", "", "path to client certificate PEM file (auto: ~/.jnicli[/PROFILE]/client.crt)")
	rootCmd.PersistentFlags().StringVar(&flagKey, "key", "", "path to client private key PEM file (auto: ~/.jnicli[/PROFILE]/client.key)")
	rootCmd.PersistentFlags().StringVar(&flagCA, "ca", "", "path to CA certificate PEM file (auto: ~/.jnicli[/PROFILE]/ca.crt)")
	rootCmd.PersistentFlags().StringVar(&flagProfile, "profile", "", "credential profile below ~/.jnicli (for example: pixel8pro)")
}

// validateDirectLANAddress accepts a private IPv4 address with a valid TCP port.
//
// Example: validateDirectLANAddress("192.168.0.242:50051") accepts a phone LAN endpoint.
func validateDirectLANAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("--addr %q must be a private IPv4 HOST:PORT: %w", address, err)
	}

	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.Is4() {
		return fmt.Errorf("--addr host %q must be a literal private IPv4 address", host)
	}
	if ip.IsLoopback() {
		return fmt.Errorf("--addr host %q must not be loopback", host)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("--addr host %q must not be unspecified", host)
	}
	if !ip.IsPrivate() {
		return fmt.Errorf("--addr host %q must be in an RFC 1918 private IPv4 range", host)
	}

	if port == "" {
		return fmt.Errorf("--addr port must be between 1 and 65535")
	}
	for _, digit := range port {
		if digit < '0' || digit > '9' {
			return fmt.Errorf("--addr port %q must be between 1 and 65535", port)
		}
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return fmt.Errorf("--addr port %q must be between 1 and 65535", port)
	}
	return nil
}

func requestContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	return context.WithTimeout(cmd.Context(), flagTimeout)
}

func buildMTLSCredentials() (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(flagCert, flagKey)
	if err != nil {
		return nil, fmt.Errorf("loading client cert/key: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: flagInsecure,
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
