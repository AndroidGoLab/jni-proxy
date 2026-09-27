package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
)

func TestCredentialPathsUsePersistentJNICLIProfileDirectory(t *testing.T) {
	got := credentialPathsFor("/home/tester", "pixel8pro")
	want := credentialPaths{
		cert: "/home/tester/.jnicli/pixel8pro/client.crt",
		key:  "/home/tester/.jnicli/pixel8pro/client.key",
		ca:   "/home/tester/.jnicli/pixel8pro/ca.crt",
	}

	if got != want {
		t.Fatalf("credentialPathsFor() = %#v, want %#v", got, want)
	}
}

func TestCredentialPathsDefaultProfileUsesPersistentRoot(t *testing.T) {
	got := credentialPathsFor("/home/tester", "")
	want := credentialPaths{
		cert: "/home/tester/.jnicli/client.crt",
		key:  "/home/tester/.jnicli/client.key",
		ca:   "/home/tester/.jnicli/ca.crt",
	}

	if got != want {
		t.Fatalf("credentialPathsFor() = %#v, want %#v", got, want)
	}
}

func TestValidateCredentialProfileRejectsPathTraversal(t *testing.T) {
	for _, profile := range []string{"../other", "nested/profile", ".", ".."} {
		if err := validateCredentialProfile(profile); err == nil {
			t.Errorf("validateCredentialProfile(%q) accepted a path", profile)
		}
	}

	if err := validateCredentialProfile("pixel6a"); err != nil {
		t.Fatalf("validateCredentialProfile(pixel6a) = %v", err)
	}
}

// TestInsecureModeDoesNotRequireClientCredentials checks the empty-profile setup.
//
// Example: with `--insecure` and no profile selected, credential setup needs no client certificate.
func TestInsecureModeDoesNotRequireClientCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	oldCert, oldKey, oldCA, oldProfile, oldInsecure := flagCert, flagKey, flagCA, flagProfile, flagInsecure
	t.Cleanup(func() {
		flagCert, flagKey, flagCA, flagProfile, flagInsecure = oldCert, oldKey, oldCA, oldProfile, oldInsecure
	})
	flagCert, flagKey, flagCA = "", "", ""
	flagProfile = ""
	flagInsecure = true

	if err := applyDefaultCredentialFlags("get-version", "jni"); err != nil {
		t.Fatalf("applyDefaultCredentialFlags: %v", err)
	}
	if flagCert != "" || flagKey != "" || flagCA != "" {
		t.Fatalf("insecure mode selected credentials without a profile: cert=%q key=%q ca=%q", flagCert, flagKey, flagCA)
	}
}

// TestJNICLIUsesAuthorityForVerifiedTLS checks that the CLI keeps dialing the
// supplied address while using --authority for HTTP/2 and TLS verification.
//
// Example: a phone LAN IP can remain --addr while jniservice is --authority.
func TestJNICLIUsesAuthorityForVerifiedTLS(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewUnstartedServer(http.HandlerFunc(
		// Record HTTP/2 :authority without implementing an unrelated RPC.
		func(w http.ResponseWriter, r *http.Request) {
			requests <- r.Host
			w.Header().Set("Content-Type", "application/grpc")
			w.Header().Set("Grpc-Status", "12")
			w.WriteHeader(http.StatusOK)
		},
	))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	hasExpectedDNSName := false
	for _, name := range server.Certificate().DNSNames {
		if name == "example.com" {
			hasExpectedDNSName = true
			break
		}
	}
	if !hasExpectedDNSName {
		t.Fatalf("test TLS certificate DNS names %v do not include example.com", server.Certificate().DNSNames)
	}
	if len(server.TLS.Certificates) != 1 {
		t.Fatalf("test TLS server has %d certificates, want 1", len(server.TLS.Certificates))
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})
	keyDER, err := x509.MarshalPKCS8PrivateKey(server.TLS.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatalf("marshal test TLS key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	profileDir := t.TempDir()
	paths := credentialPaths{
		cert: filepath.Join(profileDir, "client.crt"),
		key:  filepath.Join(profileDir, "client.key"),
		ca:   filepath.Join(profileDir, "ca.crt"),
	}
	for path, contents := range map[string][]byte{
		paths.cert: certificatePEM,
		paths.key:  keyPEM,
		paths.ca:   certificatePEM,
	} {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatalf("write test credential %s: %v", filepath.Base(path), err)
		}
	}

	authorityFlag := rootCmd.PersistentFlags().Lookup("authority")
	if authorityFlag == nil {
		t.Fatal("root command does not define --authority")
	}
	oldAuthority := authorityFlag.Value.String()
	oldAddr, oldCert, oldKey, oldCA, oldInsecure, oldConn := flagAddr, flagCert, flagKey, flagCA, flagInsecure, grpcConn
	t.Cleanup(func() {
		if err := authorityFlag.Value.Set(oldAuthority); err != nil {
			t.Errorf("restore --authority: %v", err)
		}
		flagAddr, flagCert, flagKey, flagCA, flagInsecure, grpcConn = oldAddr, oldCert, oldKey, oldCA, oldInsecure, oldConn
	})
	flagAddr = server.Listener.Addr().String()
	flagCert, flagKey, flagCA = paths.cert, paths.key, paths.ca
	flagInsecure = false

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := invokeJNICLIWithAuthorityForTest(ctx, "example.com", paths); err == nil {
		t.Fatal("capture request unexpectedly succeeded against a server without the requested RPC")
	}
	select {
	case got := <-requests:
		if got != "example.com" {
			t.Fatalf("HTTP/2 :authority = %q, want example.com", got)
		}
	default:
		t.Fatal("verified jnicli call did not reach the TLS server")
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = invokeJNICLIWithAuthorityForTest(ctx, "wrong.example", paths)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "certificate") {
		t.Fatalf("wrong TLS authority error = %v, want certificate verification failure", err)
	}
	select {
	case got := <-requests:
		t.Fatalf("TLS-mismatched authority reached HTTP/2 handler with :authority %q", got)
	default:
	}
}

// invokeJNICLIWithAuthorityForTest creates a connection through the production
// persistent pre-run and sends one request to exercise its TLS authority.
//
// Example: invoke it with example.com against a server certificate for that DNS name.
func invokeJNICLIWithAuthorityForTest(
	ctx context.Context,
	authority string,
	paths credentialPaths,
) error {
	authorityFlag := rootCmd.PersistentFlags().Lookup("authority")
	if authorityFlag == nil {
		return errors.New("root command does not define --authority")
	}
	if err := authorityFlag.Value.Set(authority); err != nil {
		return fmt.Errorf("set --authority: %w", err)
	}
	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		return err
	}
	conn := grpcConn
	grpcConn = nil
	defer conn.Close()

	return conn.Invoke(ctx, "/test.Service/Method", &emptypb.Empty{}, &emptypb.Empty{})
}

// TestValidateDirectLANAddress accepts RFC 1918 IPv4 host-port endpoints only.
//
// Example: a phone at 192.168.0.242:50051 is a valid direct-LAN target.
func TestValidateDirectLANAddress(t *testing.T) {
	for _, address := range []string{
		"10.2.3.4:1",
		"172.16.0.1:50051",
		"172.31.255.254:65535",
		"192.168.0.242:50051",
	} {
		if err := validateDirectLANAddress(address); err != nil {
			t.Errorf("validateDirectLANAddress(%q) = %v, want nil", address, err)
		}
	}

	for _, address := range []string{
		"localhost:50051",
		"127.0.0.1:50051",
		"0.0.0.0:50051",
		"192.0.2.1:50051",
		"8.8.8.8:50051",
		"[::1]:50051",
		"192.168.0.1:0",
		"192.168.0.1:65536",
		"192.168.0.1",
	} {
		if err := validateDirectLANAddress(address); err == nil {
			t.Errorf("validateDirectLANAddress(%q) = nil, want rejection", address)
		}
	}
}

// TestPersistentPreRunRejectsUnsafeJniserviceAddress validates the live CLI path.
//
// Example: --authority jniservice must reject a documentation-network --addr.
func TestPersistentPreRunRejectsUnsafeJniserviceAddress(t *testing.T) {
	oldAddr, oldAuthority, oldConn := flagAddr, flagAuthority, grpcConn
	t.Cleanup(func() {
		flagAddr, flagAuthority, grpcConn = oldAddr, oldAuthority, oldConn
	})
	flagAddr = "192.0.2.1:50051"
	flagAuthority = "jniservice"
	grpcConn = nil

	err := rootCmd.PersistentPreRunE(rootCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "private IPv4") {
		t.Fatalf("PersistentPreRunE error = %v, want private IPv4 rejection", err)
	}
	if grpcConn != nil {
		t.Fatal("PersistentPreRunE created a connection for a rejected address")
	}
}

// TestPersistentPreRunAllowsGenericExplicitAddresses preserves non-K80 jnicli targets.
//
// Example: `--addr localhost:50051` remains valid without the jniservice authority.
func TestPersistentPreRunAllowsGenericExplicitAddresses(t *testing.T) {
	addrFlag := rootCmd.PersistentFlags().Lookup("addr")
	if addrFlag == nil {
		t.Fatal("root command does not define --addr")
	}

	oldAddr, oldAuthority := flagAddr, flagAuthority
	oldCert, oldKey, oldCA, oldProfile, oldInsecure := flagCert, flagKey, flagCA, flagProfile, flagInsecure
	oldConn, oldAddrChanged := grpcConn, addrFlag.Changed
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() {
		if grpcConn != nil && grpcConn != oldConn {
			if err := grpcConn.Close(); err != nil {
				t.Errorf("close test gRPC connection: %v", err)
			}
		}
		flagAddr, flagAuthority = oldAddr, oldAuthority
		flagCert, flagKey, flagCA, flagProfile, flagInsecure = oldCert, oldKey, oldCA, oldProfile, oldInsecure
		grpcConn, addrFlag.Changed = oldConn, oldAddrChanged
	})
	flagCert, flagKey, flagCA, flagProfile = "", "", "", ""
	flagInsecure = false
	addrFlag.Changed = true

	for _, authority := range []string{"", "example.com"} {
		for _, address := range []string{"127.0.0.1:50051", "localhost:50051", "[::1]:50051"} {
			flagAddr, flagAuthority = address, authority
			grpcConn = nil

			if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
				t.Errorf("PersistentPreRunE(--addr %q, --authority %q) = %v, want nil", address, authority, err)
				continue
			}
			if grpcConn == nil {
				t.Errorf("PersistentPreRunE(--addr %q, --authority %q) created no connection", address, authority)
				continue
			}
			if err := grpcConn.Close(); err != nil {
				t.Errorf("close connection for --addr %q, --authority %q: %v", address, authority, err)
			}
			grpcConn = nil
		}
	}
}
