//go:build !android

package e2e_grpc_test

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var (
	testCertFile string
	testKeyFile  string
	testCAFile   string
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	addr := os.Getenv("JNICTL_E2E_ADDR")
	if addr == "" {
		// No address set: individual tests will skip via skipIfNoEmulator.
		return m.Run()
	}

	certDir, err := os.MkdirTemp("", "e2e-certs-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E setup: creating temp dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(certDir) }()

	testCertFile = filepath.Join(certDir, "client.crt")
	testKeyFile = filepath.Join(certDir, "client.key")
	testCAFile = filepath.Join(certDir, "ca.crt")

	// Use a randomized CN to avoid UNIQUE constraint collisions with
	// previous test runs that may not have cleaned up.
	cn := fmt.Sprintf("e2e-test-%d", rand.Int63())

	// Register a client certificate. The Register RPC is exempted from
	// mTLS auth, so --insecure (TLS with skip-verify, no client cert) works.
	regCmd := jnicliCommand(
		"--addr", addr, "--insecure",
		"auth", "register",
		"--cn", cn,
		"--cert-out", testCertFile,
		"--key-out", testKeyFile,
		"--ca-out", testCAFile,
	)
	out, err := regCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E setup: register failed: %v\n%s\n", err, out)
		return 1
	}
	fmt.Fprintf(os.Stderr, "E2E setup: registered client %q\n", cn)

	// Grant the test client full access via the admin tool.
	if err := grantTestPermissions(cn); err != nil {
		fmt.Fprintf(os.Stderr, "E2E setup: granting permissions: %v\n", err)
		return 1
	}

	return m.Run()
}

// grantTestPermissions grants "/*" to the test client using
// jniserviceadmin. Supports two modes:
//
//   - Host mode: JNISERVICEADMIN_BIN is set to a host binary and
//     JNISERVICE_DB is a host-accessible path. The admin tool is run
//     directly on the host.
//
//   - ADB mode: JNISERVICEADMIN_ADB_BIN is set to the device-side path
//     of jniserviceadmin (e.g. /data/adb/jniservice/dataadmin). The
//     admin tool is invoked via "adb shell" on the device. JNISERVICE_DB
//     defaults to the device-side path.
func grantTestPermissions(cn string) error {
	// Try host mode first.
	if adminBin := os.Getenv("JNISERVICEADMIN_BIN"); adminBin != "" {
		return grantViaHostAdmin(adminBin, cn)
	}

	// Fall back to adb mode.
	if adbAdmin := os.Getenv("JNISERVICEADMIN_ADB_BIN"); adbAdmin != "" {
		return grantViaADB(adbAdmin, cn)
	}

	return fmt.Errorf("neither JNISERVICEADMIN_BIN nor JNISERVICEADMIN_ADB_BIN is set; cannot grant permissions")
}

func grantViaHostAdmin(adminBin, cn string) error {
	dbPath := os.Getenv("JNISERVICE_DB")
	if dbPath == "" {
		dbPath = "/data/adb/jniservice/data/acl.db"
	}

	cmd := exec.Command(adminBin, "--db", dbPath, "grants", "approve", cn, "/*")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("jniserviceadmin grants approve: %w\n%s", err, out)
	}
	fmt.Fprintf(os.Stderr, "E2E setup: granted /* to %q (host mode)\n", cn)
	return nil
}

func grantViaADB(adbAdminPath, cn string) error {
	dbPath := os.Getenv("JNISERVICE_DB")
	if dbPath == "" {
		dbPath = "/data/adb/jniservice/data/acl.db"
	}

	adbEnv := os.Getenv("ADB")
	if adbEnv == "" {
		adbEnv = "adb"
	}
	// ADB env may contain flags (e.g., "adb -s 192.168.0.159:5555").
	adbParts := strings.Fields(adbEnv)

	// Write a grant script to the device to avoid shell glob expansion of /*.
	scriptContent := fmt.Sprintf(`%s --db %s grants approve %s "/*"`,
		adbAdminPath, dbPath, cn)
	scriptFile, err := os.CreateTemp("", "grant-*.sh")
	if err != nil {
		return fmt.Errorf("creating grant script: %w", err)
	}
	defer func() { _ = os.Remove(scriptFile.Name()) }()
	if _, err := scriptFile.WriteString(scriptContent); err != nil {
		return fmt.Errorf("writing grant script: %w", err)
	}
	_ = scriptFile.Close()

	// Push script to device.
	pushArgs := make([]string, 0, len(adbParts)-1+3)
	pushArgs = append(pushArgs, adbParts[1:]...)
	pushArgs = append(pushArgs, "push", scriptFile.Name(), "/data/adb/jniservice/e2e-grant.sh")
	pushCmd := exec.Command(adbParts[0], pushArgs...)
	if out, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pushing grant script: %w\n%s", err, out)
	}

	// Run script on device. Use "su root" if not already running as root,
	// falling back to direct execution (adb root gives us uid 0).
	runArgs := make([]string, 0, len(adbParts)-1+4)
	runArgs = append(runArgs, adbParts[1:]...)
	runArgs = append(runArgs, "shell", "sh", "/data/adb/jniservice/e2e-grant.sh")
	cmd := exec.Command(adbParts[0], runArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("adb shell jniserviceadmin: %w\n%s", err, out)
	}
	fmt.Fprintf(os.Stderr, "E2E setup: granted /* to %q (adb mode)\n", cn)
	return nil
}
