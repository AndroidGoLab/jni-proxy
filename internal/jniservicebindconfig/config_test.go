package jniservicebindconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadAppliesDefaultsFileThenNonemptyEnvironment verifies listener precedence.
//
// Example: a nonempty environment host wins while an empty port keeps the file value.
func TestLoadAppliesDefaultsFileThenNonemptyEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jniservice.env")
	if err := os.WriteFile(path, []byte("JNISERVICE_LISTEN=192.168.1.42\nJNISERVICE_PORT=50052\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Load(path, map[string]string{
		"JNISERVICE_LISTEN":   "192.168.1.43",
		"JNISERVICE_PORT":     "",
		"JNISERVICE_DATA_DIR": "/environment-only/data",
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if want := (Config{Listen: "192.168.1.43", Port: "50052"}); got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

// TestLoadEnvironmentPortOverridesFilePort verifies a nonempty port override retains the file host.
//
// Example: JNISERVICE_PORT=50053 overrides the file port without changing its listener host.
func TestLoadEnvironmentPortOverridesFilePort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jniservice.env")
	if err := os.WriteFile(path, []byte("JNISERVICE_LISTEN=192.168.1.42\nJNISERVICE_PORT=50052\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Load(path, map[string]string{"JNISERVICE_PORT": "50053"})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if want := (Config{Listen: "192.168.1.42", Port: "50053"}); got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

// TestLoadAppliesFileLaunchOverridesThenEnvironment verifies each listener source wins in order.
//
// Example: the staged host overrides the file while the actual process port overrides the staged port.
func TestLoadAppliesFileLaunchOverridesThenEnvironment(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "jniservice.env")
	launchPath := filepath.Join(t.TempDir(), "jniservice.launch.env")
	if err := os.WriteFile(configPath, []byte("JNISERVICE_LISTEN=192.168.1.42\nJNISERVICE_PORT=50051\n"), 0o600); err != nil {
		t.Fatalf("write durable config: %v", err)
	}
	if err := os.WriteFile(launchPath, []byte("JNISERVICE_LISTEN=192.168.1.43\nJNISERVICE_PORT=50052\n"), 0o600); err != nil {
		t.Fatalf("write staged launch config: %v", err)
	}

	env := map[string]string{
		"JNISERVICE_LAUNCH_CONFIG": launchPath,
		"JNISERVICE_PORT":          "50053",
	}
	got, err := Load(configPath, env)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if want := (Config{Listen: "192.168.1.43", Port: "50053"}); got != want {
		t.Fatalf("config = %+v, want staged host and environment port %+v", got, want)
	}

	env["JNISERVICE_LISTEN"] = "192.168.1.44"
	got, err = Load(configPath, env)
	if err != nil {
		t.Fatalf("load config with environment host: %v", err)
	}
	if want := (Config{Listen: "192.168.1.44", Port: "50053"}); got != want {
		t.Fatalf("config = %+v, want environment overrides %+v", got, want)
	}
}

// TestLoadEmptyLaunchConfigFallsBackToDurableConfigOrDefaults verifies clearing staged values restores the base.
//
// Example: an empty launch file leaves the selected config intact or returns loopback defaults when it is absent.
func TestLoadEmptyLaunchConfigFallsBackToDurableConfigOrDefaults(t *testing.T) {
	tests := []struct {
		name          string
		durableConfig string
		want          Config
	}{
		{
			name:          "durable config",
			durableConfig: "JNISERVICE_LISTEN=192.168.1.42\nJNISERVICE_PORT=50052\n",
			want:          Config{Listen: "192.168.1.42", Port: "50052"},
		},
		{
			name: "safe defaults",
			want: Config{Listen: "127.0.0.1", Port: "50051"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "jniservice.env")
			if testCase.durableConfig != "" {
				if err := os.WriteFile(configPath, []byte(testCase.durableConfig), 0o600); err != nil {
					t.Fatalf("write durable config: %v", err)
				}
			}
			launchPath := filepath.Join(t.TempDir(), "jniservice.launch.env")
			if err := os.WriteFile(launchPath, nil, 0o600); err != nil {
				t.Fatalf("write empty staged launch config: %v", err)
			}

			got, err := Load(configPath, map[string]string{"JNISERVICE_LAUNCH_CONFIG": launchPath})
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("config = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestLoadUsesLoopbackDefaultsWhenFileIsAbsent verifies the safe fallback.
//
// Example: a missing optional file resolves to 127.0.0.1:50051.
func TestLoadUsesLoopbackDefaultsWhenFileIsAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.env")
	got, err := Load(path, nil)
	if err != nil {
		t.Fatalf("load absent optional config: %v", err)
	}
	if want := (Config{Listen: "127.0.0.1", Port: "50051"}); got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

// TestLoadRejectsUnspecifiedListenerAddressesFromFile verifies file values cannot request wildcard binds.
//
// Example: JNISERVICE_LISTEN=0.0.0.0 in jniservice.env returns an error.
func TestLoadRejectsUnspecifiedListenerAddressesFromFile(t *testing.T) {
	for _, address := range []string{
		"0.0.0.0",
		"::",
		"::ffff:0.0.0.0",
		"[0.0.0.0]",
		"[::]",
		"[::%lo]",
	} {
		t.Run(address, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "jniservice.env")
			data := []byte("JNISERVICE_LISTEN=" + address + "\n")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			if got, err := Load(path, nil); err == nil || !strings.Contains(err.Error(), "unspecified") {
				t.Fatalf("load wildcard listener = %+v, error %v; want unspecified-address error", got, err)
			}
		})
	}
}

// TestLoadRejectsUnspecifiedListenerAddressesFromEnvironment verifies environment overrides cannot request wildcard binds.
//
// Example: JNISERVICE_LISTEN=:: in the service environment returns an error.
func TestLoadRejectsUnspecifiedListenerAddressesFromEnvironment(t *testing.T) {
	for _, address := range []string{
		"0.0.0.0",
		"::",
		"::ffff:0.0.0.0",
		"[0.0.0.0]",
		"[::]",
		"[::%lo]",
	} {
		t.Run(address, func(t *testing.T) {
			if got, err := Load("", map[string]string{"JNISERVICE_LISTEN": address}); err == nil || !strings.Contains(err.Error(), "unspecified") {
				t.Fatalf("load wildcard listener = %+v, error %v; want unspecified-address error", got, err)
			}
		})
	}
}

// TestLoadAcceptsExplicitInterfaceAddress verifies a concrete interface address remains configurable.
//
// Example: JNISERVICE_LISTEN=192.168.0.242 resolves to that specific host.
func TestLoadAcceptsExplicitInterfaceAddress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jniservice.env")
	if err := os.WriteFile(path, []byte("JNISERVICE_LISTEN=192.168.0.242\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := Load(path, nil)
	if err != nil {
		t.Fatalf("load explicit interface address: %v", err)
	}
	if want := (Config{Listen: "192.168.0.242", Port: "50051"}); got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

// TestLoadAcceptsExplicitInterfaceAddressFromEnvironment verifies an explicit environment bind remains configurable.
//
// Example: JNISERVICE_LISTEN=192.168.0.242 overrides the loopback default.
func TestLoadAcceptsExplicitInterfaceAddressFromEnvironment(t *testing.T) {
	got, err := Load("", map[string]string{"JNISERVICE_LISTEN": "192.168.0.242"})
	if err != nil {
		t.Fatalf("load explicit interface address: %v", err)
	}
	if want := (Config{Listen: "192.168.0.242", Port: "50051"}); got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

// TestParseReadsOnlyListenerKeysAsData verifies ignored keys stay inert data.
//
// Example: JNISERVICE_DATA_DIR is ignored and shell syntax is not executed.
func TestParseReadsOnlyListenerKeysAsData(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "executed")
	data := fmt.Sprintf(
		"# comment\nexport JNISERVICE_LISTEN='192.168.1.42'\nJNISERVICE_PORT=50052\nJNISERVICE_DATA_DIR=$(touch '%s')\n",
		markerPath,
	)
	got, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if want := (Config{Listen: "192.168.1.42", Port: "50052"}); got != want {
		t.Fatalf("config = %+v, want listener-only values %+v", got, want)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("data-file shell syntax executed (stat error %v)", err)
	}
}

// TestParseRejectsMalformedData verifies invalid lines report their position.
//
// Example: a line without '=' reports its one-based line number.
func TestParseRejectsMalformedData(t *testing.T) {
	if _, err := Parse([]byte("JNISERVICE_LISTEN=192.168.1.42\ninvalid\n")); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("parse malformed config error = %v, want line 2 error", err)
	}
}

// TestLoadReturnsReadErrors verifies non-absence file errors are returned.
//
// Example: attempting to read a directory as a config file returns a path-specific error.
func TestLoadReturnsReadErrors(t *testing.T) {
	path := t.TempDir()
	if _, err := Load(path, nil); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("load directory config error = %v, want path-specific error", err)
	}
}

// TestParseReportsOversizedLine verifies scanner limits are not silently ignored.
//
// Example: an oversized config line returns the scanner error.
func TestParseReportsOversizedLine(t *testing.T) {
	data := []byte("JNISERVICE_LISTEN=" + strings.Repeat("x", 70*1024) + "\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("parse oversized line succeeded, want scanner error")
	}
}
