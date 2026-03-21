//go:build !android

package e2e_grpc_test

import (
	"strings"
	"testing"
)

// TestE2E_Services tests all high-level Android service commands via jnicli.
// Each subtest group exercises a service that was verified during the manual
// E2E run. Tests are organised into three categories:
//
//   - Working: expect exit-code 0 (and optionally validate JSON fields).
//   - ExpectedError: expect a non-zero exit-code with a specific error string.
//   - Unimplemented: expect a non-zero exit-code containing "Unimplemented".

// ---------- helpers ----------

// runSuccess runs jnicli and asserts exit-code 0.
func runSuccess(t *testing.T, args ...string) string {
	t.Helper()
	return runLiveJnicli(t, args...)
}

// runExpectErr runs jnicli and asserts non-zero exit-code.
// Returns combined output for further assertions.
func runExpectErr(t *testing.T, args ...string) string {
	t.Helper()
	return runLiveJnicliExpectError(t, args...)
}

// assertContains fails the test if substr is not found in s.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, s)
	}
}

// assertJSONField parses JSON output and checks a top-level key exists.
func assertJSONField(t *testing.T, out, field string) {
	t.Helper()
	resp := parseJSON(t, out)
	if _, ok := resp[field]; !ok {
		t.Errorf("expected JSON field %q in response: %v", field, resp)
	}
}

// ---------- Working services ----------

func TestE2E_Services_Accounts(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetAccounts", func(t *testing.T) {
		runSuccess(t, "accounts", "account-manager", "get-accounts")
	})
}

func TestE2E_Services_Admin(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsDeviceIdAttestationSupported", func(t *testing.T) {
		runSuccess(t, "admin", "device-policy-manager", "is-device-id-attestation-supported")
	})
}

func TestE2E_Services_Alarm(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetNextAlarmClock", func(t *testing.T) {
		runSuccess(t, "alarm", "manager", "get-next-alarm-clock")
	})
}

func TestE2E_Services_AudioManager(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetRingerMode", func(t *testing.T) {
		out := runSuccess(t, "audiomanager", "audio-manager", "get-ringer-mode")
		assertJSONField(t, out, "result")
	})
	t.Run("GetStreamMaxVolume", func(t *testing.T) {
		out := runSuccess(t, "audiomanager", "audio-manager", "get-stream-max-volume", "--arg0", "3")
		assertJSONField(t, out, "result")
	})
}

func TestE2E_Services_Battery(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsCharging", func(t *testing.T) {
		runSuccess(t, "battery", "manager", "is-charging")
	})
	t.Run("GetIntProperty", func(t *testing.T) {
		out := runSuccess(t, "battery", "manager", "get-int-property", "--arg0", "4")
		resp := parseJSON(t, out)
		capacity := getInt64Field(t, resp, "result")
		if capacity < 0 || capacity > 100 {
			t.Errorf("expected battery capacity 0-100, got %d", capacity)
		}
		t.Logf("battery capacity: %d%%", capacity)
	})
}

func TestE2E_Services_Biometric(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("CanAuthenticate0", func(t *testing.T) {
		runSuccess(t, "biometric", "manager", "can-authenticate0")
	})
}

func TestE2E_Services_Camera(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetCameraIdList", func(t *testing.T) {
		runSuccess(t, "camera", "manager", "get-camera-id-list")
	})
	t.Run("GetTorchStrengthLevel", func(t *testing.T) {
		runSuccess(t, "camera", "manager", "get-torch-strength-level", "--arg0", "0")
	})
}

func TestE2E_Services_Clipboard(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("HasPrimaryClip", func(t *testing.T) {
		runSuccess(t, "clipboard", "manager", "has-primary-clip")
	})
	t.Run("GetPrimaryClip", func(t *testing.T) {
		runSuccess(t, "clipboard", "manager", "get-primary-clip")
	})
}

func TestE2E_Services_Companion(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetMyAssociations", func(t *testing.T) {
		runSuccess(t, "companion", "device-manager", "get-my-associations")
	})
}

func TestE2E_Services_Device(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("Info", func(t *testing.T) {
		out := runSuccess(t, "device", "info")
		resp := parseJSON(t, out)
		for _, field := range []string{"manufacturer", "model", "sdk_int"} {
			if _, ok := resp[field]; !ok {
				t.Errorf("expected field %q in device info", field)
			}
		}
		t.Logf("device info: %v", resp)
	})
}

func TestE2E_Services_JNI(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetVersion", func(t *testing.T) {
		out := runSuccess(t, "jni", "get-version")
		assertJSONField(t, out, "version")
	})
	t.Run("FindClass", func(t *testing.T) {
		out := runSuccess(t, "jni", "class", "find", "--name", "java/lang/String")
		assertJSONField(t, out, "classHandle")
	})
}

func TestE2E_Services_Job(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetAllPendingJobs", func(t *testing.T) {
		runSuccess(t, "job", "scheduler", "get-all-pending-jobs")
	})
}

func TestE2E_Services_Keyguard(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsDeviceLocked", func(t *testing.T) {
		runSuccess(t, "keyguard", "manager", "is-device-locked")
	})
	t.Run("IsKeyguardLocked", func(t *testing.T) {
		runSuccess(t, "keyguard", "manager", "is-keyguard-locked")
	})
}

func TestE2E_Services_Location(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsLocationEnabled", func(t *testing.T) {
		runSuccess(t, "location", "manager", "is-location-enabled")
	})
	t.Run("GetAllProviders", func(t *testing.T) {
		runSuccess(t, "location", "manager", "get-all-providers")
	})
	t.Run("GetGnssHardwareModelName", func(t *testing.T) {
		runSuccess(t, "location", "manager", "get-gnss-hardware-model-name")
	})
}

func TestE2E_Services_Net(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetActiveNetwork", func(t *testing.T) {
		runSuccess(t, "net", "connectivity-manager", "get-active-network")
	})
	t.Run("IsActiveNetworkMetered", func(t *testing.T) {
		runSuccess(t, "net", "connectivity-manager", "is-active-network-metered")
	})
	t.Run("IsDefaultNetworkActive", func(t *testing.T) {
		runSuccess(t, "net", "connectivity-manager", "is-default-network-active")
	})
}

func TestE2E_Services_Notification(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("AreNotificationsEnabled", func(t *testing.T) {
		runSuccess(t, "notification", "manager", "are-notifications-enabled")
	})
	t.Run("GetImportance", func(t *testing.T) {
		runSuccess(t, "notification", "manager", "get-importance")
	})
	t.Run("AreBubblesEnabled", func(t *testing.T) {
		runSuccess(t, "notification", "manager", "are-bubbles-enabled")
	})
}

func TestE2E_Services_Power(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsInteractive", func(t *testing.T) {
		runSuccess(t, "power", "manager", "is-interactive")
	})
	t.Run("IsScreenOn", func(t *testing.T) {
		runSuccess(t, "power", "manager", "is-screen-on")
	})
	t.Run("GetCurrentThermalStatus", func(t *testing.T) {
		runSuccess(t, "power", "manager", "get-current-thermal-status")
	})
	t.Run("GetThermalHeadroom", func(t *testing.T) {
		runSuccess(t, "power", "manager", "get-thermal-headroom", "--arg0", "10")
	})
}

func TestE2E_Services_Print(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetPrintJobs", func(t *testing.T) {
		runSuccess(t, "print", "manager", "get-print-jobs")
	})
}

func TestE2E_Services_Projection(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("CreateScreenCaptureIntent0", func(t *testing.T) {
		runSuccess(t, "projection", "media-projection-manager", "create-screen-capture-intent0")
	})
}

func TestE2E_Services_Role(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsRoleAvailable", func(t *testing.T) {
		runSuccess(t, "role", "manager", "is-role-available", "--arg0", "android.app.role.BROWSER")
	})
}

func TestE2E_Services_Storage(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsCheckpointSupported", func(t *testing.T) {
		runSuccess(t, "storage", "manager", "is-checkpoint-supported")
	})
}

func TestE2E_Services_Telecom(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetDefaultDialerPackage", func(t *testing.T) {
		runSuccess(t, "telecom", "manager", "get-default-dialer-package")
	})
	t.Run("IsTtySupported", func(t *testing.T) {
		runSuccess(t, "telecom", "manager", "is-tty-supported")
	})
}

func TestE2E_Services_Telephony(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetPhoneType", func(t *testing.T) {
		runSuccess(t, "telephony", "manager", "get-phone-type")
	})
	t.Run("GetPhoneCount", func(t *testing.T) {
		runSuccess(t, "telephony", "manager", "get-phone-count")
	})
	t.Run("IsSmsCapable", func(t *testing.T) {
		runSuccess(t, "telephony", "manager", "is-sms-capable")
	})
}

func TestE2E_Services_Usage(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetAppStandbyBucket", func(t *testing.T) {
		runSuccess(t, "usage", "stats-manager", "get-app-standby-bucket")
	})
}

func TestE2E_Services_USB(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetAccessoryList", func(t *testing.T) {
		runSuccess(t, "usb", "manager", "get-accessory-list")
	})
}

func TestE2E_Services_Vibrator(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("HasVibrator", func(t *testing.T) {
		runSuccess(t, "vibrator", "vibrator", "has-vibrator")
	})
	t.Run("HasAmplitudeControl", func(t *testing.T) {
		runSuccess(t, "vibrator", "vibrator", "has-amplitude-control")
	})
}

func TestE2E_Services_WiFi(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsWifiEnabled", func(t *testing.T) {
		runSuccess(t, "wifi", "manager", "is-wifi-enabled")
	})
	t.Run("GetWifiState", func(t *testing.T) {
		runSuccess(t, "wifi", "manager", "get-wifi-state")
	})
	t.Run("GetMaxSignalLevel", func(t *testing.T) {
		runSuccess(t, "wifi", "manager", "get-max-signal-level")
	})
}

func TestE2E_Services_WiFiP2P(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsChannelConstrainedDiscoverySupported", func(t *testing.T) {
		runSuccess(t, "wifi_p2p", "wifi-p2p-manager", "is-channel-constrained-discovery-supported")
	})
}

func TestE2E_Services_WiFiRTT(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsAvailable", func(t *testing.T) {
		runSuccess(t, "wifi_rtt", "wifi-rtt-manager", "is-available")
	})
}

func TestE2E_Services_Auth(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("ListPermissions", func(t *testing.T) {
		out := runSuccess(t, "auth", "list-permissions")
		// The output should be valid (non-empty) since the test client
		// was granted /* in TestMain.
		if len(strings.TrimSpace(out)) == 0 {
			t.Error("expected non-empty list-permissions output")
		}
		t.Logf("permissions output length: %d bytes", len(out))
	})
}

func TestE2E_Services_Download(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetMimeTypeForDownloadedFile", func(t *testing.T) {
		runSuccess(t, "download", "manager", "get-mime-type-for-downloaded-file", "--arg0", "1")
	})
}

// ---------- Expected error services ----------

func TestE2E_Services_Blob_SecurityException(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetLeasedBlobs", func(t *testing.T) {
		out := runExpectErr(t, "blob", "store-manager", "get-leased-blobs")
		assertContains(t, out, "SecurityException")
	})
}

func TestE2E_Services_Location_SecurityException(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetLastKnownLocation_GPS", func(t *testing.T) {
		out := runExpectErr(t, "location", "manager", "get-last-known-location", "--arg0", "gps")
		assertContains(t, out, "SecurityException")
	})
}

func TestE2E_Services_Notification_SecurityException(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetNotificationChannels", func(t *testing.T) {
		out := runExpectErr(t, "notification", "manager", "get-notification-channels")
		assertContains(t, out, "SecurityException")
	})
}

// ---------- Unimplemented services ----------

func TestE2E_Services_Bluetooth_Unimplemented(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("AdapterIsEnabled", func(t *testing.T) {
		out := runExpectErr(t, "bluetooth", "adapter", "is-enabled")
		assertContains(t, out, "Unimplemented")
	})
}

func TestE2E_Services_Build_Unimplemented(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("GetSerial", func(t *testing.T) {
		out := runExpectErr(t, "build", "build", "get-serial")
		assertContains(t, out, "Unimplemented")
	})
}

func TestE2E_Services_NFC_Unimplemented(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("AdapterIsEnabled", func(t *testing.T) {
		out := runExpectErr(t, "nfc", "adapter", "is-enabled")
		assertContains(t, out, "Unimplemented")
	})
}

func TestE2E_Services_PM_Unimplemented(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsSafeMode", func(t *testing.T) {
		out := runExpectErr(t, "pm", "package-manager", "is-safe-mode")
		assertContains(t, out, "Unimplemented")
	})
}

func TestE2E_Services_Speech_Unimplemented(t *testing.T) {
	skipIfNoEmulator(t)
	t.Run("IsSpeaking", func(t *testing.T) {
		out := runExpectErr(t, "speech", "text-to-speech", "is-speaking")
		assertContains(t, out, "Unimplemented")
	})
}
