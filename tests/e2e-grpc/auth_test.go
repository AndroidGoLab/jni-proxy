//go:build !android

package e2e_grpc_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func skipIfNoMTLS(t *testing.T) {
	t.Helper()
	if os.Getenv("JNICTL_E2E_ADDR") == "" {
		t.Skip("JNICTL_E2E_ADDR not set")
	}
	if os.Getenv("JNICTL_E2E_MTLS") != "1" {
		t.Skip("JNICTL_E2E_MTLS not set; skipping mTLS tests")
	}
}

// runJnicliAuth runs jnicli with mTLS cert flags against the live server.
func runJnicliAuth(t *testing.T, certDir string, args ...string) string {
	t.Helper()
	addr := os.Getenv("JNICTL_E2E_ADDR")
	fullArgs := []string{
		"--addr", addr,
		"--insecure",
		"--cert", filepath.Join(certDir, "client.crt"),
		"--key", filepath.Join(certDir, "client.key"),
		"--ca", filepath.Join(certDir, "ca.crt"),
	}
	fullArgs = append(fullArgs, args...)

	cmd := jnicliCommand(fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jnicli %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// runJnicliAuthExpectError runs jnicli with mTLS and expects failure.
func runJnicliAuthExpectError(t *testing.T, certDir string, args ...string) string {
	t.Helper()
	addr := os.Getenv("JNICTL_E2E_ADDR")
	fullArgs := []string{
		"--addr", addr,
		"--insecure",
		"--cert", filepath.Join(certDir, "client.crt"),
		"--key", filepath.Join(certDir, "client.key"),
		"--ca", filepath.Join(certDir, "ca.crt"),
	}
	fullArgs = append(fullArgs, args...)

	cmd := jnicliCommand(fullArgs...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error from jnicli %s but got success:\n%s", strings.Join(args, " "), out)
	}
	return string(out)
}

// runAdmin runs jniserviceadmin with the test DB. Supports host mode
// (JNISERVICEADMIN_BIN) and ADB mode (JNISERVICEADMIN_ADB_BIN).
func runAdmin(t *testing.T, dbPath string, args ...string) string {
	t.Helper()

	if adminBin := os.Getenv("JNISERVICEADMIN_BIN"); adminBin != "" {
		fullArgs := append([]string{"--db", dbPath}, args...)
		cmd := exec.Command(adminBin, fullArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("jniserviceadmin %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	if adbAdmin := os.Getenv("JNISERVICEADMIN_ADB_BIN"); adbAdmin != "" {
		adbEnv := os.Getenv("ADB")
		if adbEnv == "" {
			adbEnv = "adb"
		}
		adbParts := strings.Fields(adbEnv)

		// Write a script to the device to avoid adb shell quoting issues
		// with glob characters like /* in method patterns.
		cmdStr := adbAdmin + " --db " + dbPath
		for _, a := range args {
			cmdStr += " " + shellescape(a)
		}
		scriptFile, err := os.CreateTemp("", "admin-*.sh")
		if err != nil {
			t.Fatalf("creating admin script: %v", err)
		}
		defer func() { _ = os.Remove(scriptFile.Name()) }()
		if _, err := scriptFile.WriteString(cmdStr); err != nil {
			t.Fatalf("writing admin script: %v", err)
		}
		_ = scriptFile.Close()

		pushArgs := append(adbParts[1:], "push", scriptFile.Name(), "/data/local/tmp/e2e-admin.sh")
		pushCmd := exec.Command(adbParts[0], pushArgs...)
		if out, err := pushCmd.CombinedOutput(); err != nil {
			t.Fatalf("pushing admin script: %v\n%s", err, out)
		}

		runArgs := append(adbParts[1:], "shell", "sh", "/data/local/tmp/e2e-admin.sh")
		cmd := exec.Command(adbParts[0], runArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("adb jniserviceadmin %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	t.Skip("neither JNISERVICEADMIN_BIN nor JNISERVICEADMIN_ADB_BIN is set")
	return ""
}

// shellescape wraps a string in single quotes for safe shell execution.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func TestE2E_Auth_RegisterAndDenied(t *testing.T) {
	skipIfNoMTLS(t)
	certDir := t.TempDir()

	// 1. Register a new client.
	addr := os.Getenv("JNICTL_E2E_ADDR")
	cmd := jnicliCommand(
		"--addr", addr, "--insecure",
		"auth", "register",
		"--cn", "test-e2e-client",
		"--key-out", filepath.Join(certDir, "client.key"),
		"--cert-out", filepath.Join(certDir, "client.crt"),
		"--ca-out", filepath.Join(certDir, "ca.crt"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("register: %v\n%s", err, out)
	}
	t.Logf("Registered: %s", strings.TrimSpace(string(out)))

	// Verify cert files exist.
	for _, f := range []string{"client.key", "client.crt", "ca.crt"} {
		if _, err := os.Stat(filepath.Join(certDir, f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}

	// 2. Try to call FindClass — should be denied (no grants).
	errOut := runJnicliAuthExpectError(t, certDir, "jni", "class", "find", "--name", "java/lang/String")
	if !strings.Contains(errOut, "PermissionDenied") {
		t.Errorf("expected PermissionDenied, got: %s", errOut)
	}
	t.Log("FindClass correctly denied without grant")
}

func TestE2E_Auth_GrantAndAllow(t *testing.T) {
	skipIfNoMTLS(t)

	dbPath := os.Getenv("JNISERVICE_DB")
	if dbPath == "" {
		dbPath = "/data/adb/jniservice/data/acl.db"
	}

	certDir := t.TempDir()

	// 1. Register.
	addr := os.Getenv("JNICTL_E2E_ADDR")
	cmd := jnicliCommand(
		"--addr", addr, "--insecure",
		"auth", "register",
		"--cn", "test-grant-client",
		"--key-out", filepath.Join(certDir, "client.key"),
		"--cert-out", filepath.Join(certDir, "client.crt"),
		"--ca-out", filepath.Join(certDir, "ca.crt"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("register: %v\n%s", err, out)
	}

	// 2. Admin grants access.
	runAdmin(t, dbPath, "grants", "approve", "test-grant-client", "/jni_raw.JNIService/*")
	t.Log("Admin granted /jni_raw.JNIService/* to test-grant-client")

	// 3. Also grant handlestore (needed for handle release in tests).
	runAdmin(t, dbPath, "grants", "approve", "test-grant-client", "/handlestore.HandleStoreService/*")

	// 4. Now FindClass should succeed.
	result := runJnicliAuth(t, certDir, "jni", "class", "find", "--name", "java/lang/String")
	resp := parseJSON(t, result)
	handle := getInt64Field(t, resp, "classHandle")
	if handle <= 0 {
		t.Errorf("expected classHandle > 0, got %d", handle)
	}
	t.Logf("FindClass succeeded after grant: classHandle=%d", handle)
}

func TestE2E_Auth_ListPermissions(t *testing.T) {
	skipIfNoMTLS(t)

	dbPath := os.Getenv("JNISERVICE_DB")
	if dbPath == "" {
		dbPath = "/data/adb/jniservice/data/acl.db"
	}

	certDir := t.TempDir()

	// Register.
	addr := os.Getenv("JNICTL_E2E_ADDR")
	cmd := jnicliCommand(
		"--addr", addr, "--insecure",
		"auth", "register",
		"--cn", "test-list-client",
		"--key-out", filepath.Join(certDir, "client.key"),
		"--cert-out", filepath.Join(certDir, "client.crt"),
		"--ca-out", filepath.Join(certDir, "ca.crt"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("register: %v\n%s", err, out)
	}

	// Grant some methods.
	runAdmin(t, dbPath, "grants", "approve", "test-list-client", "/jni_raw.JNIService/FindClass")

	// List permissions via jnicli.
	result := runJnicliAuth(t, certDir, "auth", "list-permissions")
	if !strings.Contains(result, "FindClass") {
		t.Errorf("expected FindClass in permissions list, got: %s", result)
	}
	t.Logf("Permissions: %s", strings.TrimSpace(result))
}
