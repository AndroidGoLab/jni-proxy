package mcp

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndroidGoLab/jni-proxy/grpc/server/certauth"
	pb "github.com/AndroidGoLab/jni-proxy/proto/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// AutoEnroll registers with jniservice if no certs exist in configDir.
// It connects using TLS with InsecureSkipVerify (no client cert needed for Register).
// Returns paths to the cert, key, and CA files.
func AutoEnroll(ctx context.Context, addr, configDir string) (certPath, keyPath, caPath string, err error) {
	certPath = filepath.Join(configDir, "client.crt")
	keyPath = filepath.Join(configDir, "client.key")
	caPath = filepath.Join(configDir, "ca.crt")

	// If all three files already exist, return their paths.
	if fileExists(certPath) && fileExists(keyPath) && fileExists(caPath) {
		return certPath, keyPath, caPath, nil
	}

	// Create the config directory if it doesn't exist.
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", "", "", fmt.Errorf("creating config dir: %w", err)
	}

	// Generate a random suffix for the CN.
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return "", "", "", fmt.Errorf("generating random bytes: %w", err)
	}
	cn := "jnimcp-" + hex.EncodeToString(randBytes)

	// Generate EC P-256 keypair and CSR.
	csrPEM, keyPEM, err := certauth.GenerateCSR(cn)
	if err != nil {
		return "", "", "", fmt.Errorf("generating CSR: %w", err)
	}

	// Connect to jniservice without client cert but with TLS (InsecureSkipVerify
	// because jniservice uses a self-signed CA).
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(
			&tls.Config{InsecureSkipVerify: true},
		)),
	)
	if err != nil {
		return "", "", "", fmt.Errorf("connecting to jniservice for enrollment: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Call Register RPC.
	client := pb.NewAuthServiceClient(conn)
	resp, err := client.Register(ctx, &pb.RegisterRequest{
		CsrPem: string(csrPEM),
	})
	if err != nil {
		return "", "", "", fmt.Errorf("register RPC: %w", err)
	}

	// Save the returned client cert, CA cert, and the generated private key.
	if err := os.WriteFile(certPath, []byte(resp.GetClientCertPem()), 0600); err != nil {
		return "", "", "", fmt.Errorf("writing client cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return "", "", "", fmt.Errorf("writing client key: %w", err)
	}
	if err := os.WriteFile(caPath, []byte(resp.GetCaCertPem()), 0644); err != nil {
		return "", "", "", fmt.Errorf("writing CA cert: %w", err)
	}

	return certPath, keyPath, caPath, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
