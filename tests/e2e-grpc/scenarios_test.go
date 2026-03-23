//go:build !android

package e2e_grpc_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestScenario_GPSCommandsExist(t *testing.T) {
	assertCommandExists(t, "location", "manager", "get-last-known-location")
	assertCommandExists(t, "location", "manager", "is-provider-enabled")
}

func TestScenario_WiFiCommandsExist(t *testing.T) {
	assertCommandExists(t, "wifi", "manager", "get-connection-info")
}

func TestScenario_CameraCommandsExist(t *testing.T) {
	assertCommandExists(t, "camera", "manager", "set-torch-mode")
}

func TestScenario_RecorderCommandsExist(t *testing.T) {
	t.Skip("recorder subcommand removed (MediaRecorder is not a system_service)")
}

func TestScenario_NotificationCommandsExist(t *testing.T) {
	assertCommandExists(t, "notification", "manager", "are-notifications-enabled")
	assertCommandExists(t, "notification", "manager", "create-notification-channel")
	assertCommandExists(t, "notification", "manager", "cancel1")
}

func TestScenario_DeviceInfoCommandsExist(t *testing.T) {
	t.Skip("build subcommand removed (Build is not a system_service)")
}

func TestScenario_BluetoothCommandsExist(t *testing.T) {
	assertCommandExists(t, "bluetooth", "manager", "get-adapter")
}

func TestScenario_RawJNICommandsExist(t *testing.T) {
	assertCommandExists(t, "jni", "class", "find")
	assertCommandExists(t, "jni", "method", "get-static-id")
	assertCommandExists(t, "jni", "method", "call-static")
	assertCommandExists(t, "jni", "string", "new")
	assertCommandExists(t, "jni", "string", "get")
}

// assertCommandExists verifies a jnicli subcommand exists by checking its help output.
func assertCommandExists(t *testing.T, args ...string) {
	t.Helper()
	fullArgs := make([]string, len(args)+1)
	copy(fullArgs, args)
	fullArgs[len(args)] = "--help"
	cmd := exec.Command("go", append([]string{"run", jnicliBin}, fullArgs...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("command jnicli %s: %v\n%s", strings.Join(args, " "), err, out)
		return
	}
	// Verify it's a real command (has "Usage:" section), not just a group
	if !strings.Contains(string(out), "Usage:") {
		t.Errorf("command jnicli %s produced no Usage section", strings.Join(args, " "))
	}
}
