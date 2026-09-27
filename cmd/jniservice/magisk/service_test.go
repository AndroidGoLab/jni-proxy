package magisk

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AndroidGoLab/jni-proxy/internal/jniservicebindconfig"
)

// TestMagiskLauncherPassesConfigAsData verifies the module file reaches app_process without stale defaults.
//
// Example: command syntax in a module jniservice.env remains inert launcher input.
func TestMagiskLauncherPassesConfigAsData(t *testing.T) {
	moduleDir := t.TempDir()
	markerPath := filepath.Join(moduleDir, "executed")
	config := fmt.Sprintf("JNISERVICE_LISTEN=192.168.1.42\nJNISERVICE_PORT=50052\nJNISERVICE_DATA_DIR=$(touch %q)\n", markerPath)
	got := runMagiskLauncher(t, moduleDir, config, nil, "")

	if got["JNISERVICE_CONFIG"] != filepath.Join(moduleDir, "jniservice.env") {
		t.Errorf("JNISERVICE_CONFIG = %q, want module config path", got["JNISERVICE_CONFIG"])
	}
	if got["JNISERVICE_LISTEN"] != "" || got["JNISERVICE_PORT"] != "" {
		t.Errorf("launcher supplied stale bind overrides: LISTEN=%q PORT=%q", got["JNISERVICE_LISTEN"], got["JNISERVICE_PORT"])
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Errorf("config data executed shell syntax (stat error %v)", err)
	}
	if got["JNISERVICE_CONFIG"] == "" {
		return
	}

	data, err := os.ReadFile(got["JNISERVICE_CONFIG"])
	if err != nil {
		t.Fatalf("read launcher config path: %v", err)
	}
	if string(data) != config {
		t.Fatalf("config data = %q, want unchanged input %q", data, config)
	}
}

// TestMagiskLauncherPreservesExplicitEnvironmentOverrides checks launcher compatibility overrides.
//
// Example: nonempty JNISERVICE_LISTEN and JNISERVICE_PORT values reach app_process unchanged.
func TestMagiskLauncherPreservesExplicitEnvironmentOverrides(t *testing.T) {
	moduleDir := t.TempDir()
	got := runMagiskLauncher(t, moduleDir,
		"JNISERVICE_LISTEN=192.168.1.42\nJNISERVICE_PORT=50052\n",
		map[string]string{
			"JNISERVICE_LISTEN": "192.168.1.43",
			"JNISERVICE_PORT":   "50053",
		},
		"",
	)

	if got["JNISERVICE_CONFIG"] != filepath.Join(moduleDir, "jniservice.env") {
		t.Fatalf("JNISERVICE_CONFIG = %q, want module config path", got["JNISERVICE_CONFIG"])
	}
	if got["JNISERVICE_LISTEN"] != "192.168.1.43" || got["JNISERVICE_PORT"] != "50053" {
		t.Fatalf("explicit environment = LISTEN %q PORT %q, want 192.168.1.43:50053", got["JNISERVICE_LISTEN"], got["JNISERVICE_PORT"])
	}
	data, err := os.ReadFile(got["JNISERVICE_CONFIG"])
	if err != nil {
		t.Fatalf("read launcher config path: %v", err)
	}
	if string(data) != "JNISERVICE_LISTEN=192.168.1.42\nJNISERVICE_PORT=50052\n" {
		t.Fatalf("module config = %q, want original config data", data)
	}
}

// TestMagiskLauncherClearsAPKOverridesWhenEnvironmentIsRemoved verifies transient overrides do not alter durable config.
//
// Example: an empty second launch override restores the selected file or loopback defaults.
func TestMagiskLauncherClearsAPKOverridesWhenEnvironmentIsRemoved(t *testing.T) {
	tests := []struct {
		name          string
		durableConfig string
		wantBase      jniservicebindconfig.Config
		wantDurable   bool
	}{
		{
			name:          "durable config",
			durableConfig: "# preserve API-managed settings\nJNISERVICE_LISTEN=192.168.0.242\nJNISERVICE_PORT=50051\nCUSTOM_SETTING=keep\n",
			wantBase:      jniservicebindconfig.Config{Listen: "192.168.0.242", Port: "50051"},
			wantDurable:   true,
		},
		{
			name:        "safe defaults",
			wantBase:    jniservicebindconfig.Config{Listen: "127.0.0.1", Port: "50051"},
			wantDurable: false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			appConfigDir := t.TempDir()
			durablePath := filepath.Join(appConfigDir, "jniservice.env")
			launchPath := filepath.Join(appConfigDir, "jniservice.launch.env")
			if testCase.wantDurable {
				if err := os.WriteFile(durablePath, []byte(testCase.durableConfig), 0o600); err != nil {
					t.Fatalf("write durable app config: %v", err)
				}
			}

			launchEnvironment := map[string]string{
				"JNISERVICE_LISTEN": "192.168.0.243",
				"JNISERVICE_PORT":   "50053",
			}
			runMagiskLauncher(t, t.TempDir(), "", launchEnvironment, appConfigDir)

			assertMagiskDurableConfig(t, durablePath, testCase.durableConfig, testCase.wantDurable)
			assertMagiskLaunchConfig(t, launchPath, "JNISERVICE_LISTEN=192.168.0.243\nJNISERVICE_PORT=50053\n")
			got, err := jniservicebindconfig.Load(durablePath, map[string]string{"JNISERVICE_LAUNCH_CONFIG": launchPath})
			if err != nil {
				t.Fatalf("resolve staged listener config: %v", err)
			}
			if want := (jniservicebindconfig.Config{Listen: "192.168.0.243", Port: "50053"}); got != want {
				t.Fatalf("resolved listener after override launch = %+v, want %+v", got, want)
			}

			runMagiskLauncher(t, t.TempDir(), "", nil, appConfigDir)

			assertMagiskDurableConfig(t, durablePath, testCase.durableConfig, testCase.wantDurable)
			assertMagiskLaunchConfig(t, launchPath, "")
			got, err = jniservicebindconfig.Load(durablePath, map[string]string{"JNISERVICE_LAUNCH_CONFIG": launchPath})
			if err != nil {
				t.Fatalf("resolve listener after clearing overrides: %v", err)
			}
			if got != testCase.wantBase {
				t.Fatalf("resolved listener after override removal = %+v, want base %+v", got, testCase.wantBase)
			}
		})
	}
}

// assertMagiskDurableConfig verifies transient startup settings leave the selected file unchanged.
//
// Example: a missing app-managed file remains missing after launcher overrides are cleared.
func assertMagiskDurableConfig(t *testing.T, path string, want string, wantPresent bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if !wantPresent {
		if !os.IsNotExist(err) {
			t.Fatalf("durable config read error = %v, want file to remain absent", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("read durable config: %v", err)
	}
	if string(data) != want {
		t.Fatalf("durable config = %q, want unchanged contents %q", data, want)
	}
}

// assertMagiskLaunchConfig verifies the distinct transient launch file contents.
//
// Example: a launch with no explicit values leaves an empty override file.
func assertMagiskLaunchConfig(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read staged launch config: %v", err)
	}
	if string(data) != want {
		t.Fatalf("staged launch config = %q, want %q", data, want)
	}
}

// TestMagiskLauncherCopiesConfigToAPKWithoutOverrides verifies file-only APK setup.
//
// Example: a module listener config is copied unchanged into the app-private directory.
func TestMagiskLauncherCopiesConfigToAPKWithoutOverrides(t *testing.T) {
	moduleDir := t.TempDir()
	appConfigDir := filepath.Join(moduleDir, "app-files")
	config := "# listener settings\nJNISERVICE_LISTEN=192.168.0.242\nJNISERVICE_PORT=50051\n"
	runMagiskLauncher(t, moduleDir, config, nil, appConfigDir)

	data, err := os.ReadFile(filepath.Join(appConfigDir, "jniservice.env"))
	if err != nil {
		t.Fatalf("read APK config: %v", err)
	}
	if string(data) != config {
		t.Fatalf("APK config = %q, want copied file %q", data, config)
	}
}

// TestMagiskLauncherStopsAPKStartWhenConfigStagingFails verifies every required staging command is checked.
//
// Example: an unsuccessful copy leaves the activity unstarted and a diagnostic in the module log.
func TestMagiskLauncherStopsAPKStartWhenConfigStagingFails(t *testing.T) {
	cases := []struct {
		name                    string
		failedCmd               string
		environment             map[string]string
		blockStagedFileCreation bool
	}{
		{name: "create directory", failedCmd: "mkdir"},
		{name: "copy config", failedCmd: "cp"},
		{name: "verify staged overrides", failedCmd: "cmp", environment: map[string]string{"JNISERVICE_PORT": "50052"}},
		{name: "clear staged overrides", blockStagedFileCreation: true},
		{name: "set file mode", failedCmd: "chmod", environment: map[string]string{"JNISERVICE_PORT": "50052"}},
		{name: "replace staged file", failedCmd: "mv", environment: map[string]string{"JNISERVICE_PORT": "50052"}},
		{name: "resolve app uid", failedCmd: "stat"},
		{name: "set file owner", failedCmd: "chown"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			moduleDir := t.TempDir()
			appConfigDir := filepath.Join(moduleDir, "app-files")
			stagedPath := filepath.Join(appConfigDir, "jniservice.launch.env")
			if testCase.blockStagedFileCreation {
				if err := os.MkdirAll(appConfigDir, 0o700); err != nil {
					t.Fatalf("create app config directory: %v", err)
				}
				if err := os.WriteFile(stagedPath, []byte("JNISERVICE_PORT=50052\n"), 0o600); err != nil {
					t.Fatalf("write previous staged override: %v", err)
				}
				if err := os.Mkdir(stagedPath+".tmp", 0o700); err != nil {
					t.Fatalf("block staged config temp path: %v", err)
				}
			}
			_, _, err := runMagiskLauncherResult(
				t,
				moduleDir,
				"JNISERVICE_LISTEN=192.168.0.242\nJNISERVICE_PORT=50051\n",
				testCase.environment,
				appConfigDir,
				testCase.failedCmd,
			)
			if err == nil {
				t.Errorf("launcher succeeded when %s failed", testCase.failedCmd)
			}

			launchMarker := filepath.Join(moduleDir, "app-process-env")
			if _, err := os.Stat(launchMarker); !os.IsNotExist(err) {
				t.Errorf("APK start reached am after %s failed (stat error %v)", testCase.failedCmd, err)
			}
			log := readMagiskLog(t, filepath.Join(moduleDir, "jniservice.log"))
			if !strings.Contains(log, "APK config staging failed") {
				t.Errorf("launcher log = %q, want config staging diagnostic", log)
			}
			if testCase.blockStagedFileCreation {
				data, readErr := os.ReadFile(stagedPath)
				if readErr != nil || string(data) != "JNISERVICE_PORT=50052\n" {
					t.Errorf("staged config after failed clear = %q (error %v), want previous contents preserved", data, readErr)
				}
			}
		})
	}
}

// runMagiskLauncher exercises service.sh with harmless command shims.
//
// Example: an empty appConfigDir captures app_process's environment; a temporary appConfigDir exercises APK config writes.
func runMagiskLauncher(
	t *testing.T,
	moduleDir string,
	config string,
	explicitEnvironment map[string]string,
	appConfigDir string,
) map[string]string {
	got, output, err := runMagiskLauncherResult(t, moduleDir, config, explicitEnvironment, appConfigDir, "")
	if err != nil {
		t.Fatalf("run service.sh: %v; output: %s", err, output)
	}
	return got
}

// runMagiskLauncherResult runs service.sh and returns its observed process start result.
//
// Example: failedCommand replaces one launcher utility so config-staging errors can be exercised.
func runMagiskLauncherResult(
	t *testing.T,
	moduleDir string,
	config string,
	explicitEnvironment map[string]string,
	appConfigDir string,
	failedCommand string,
) (map[string]string, []byte, error) {
	t.Helper()
	launcherData, err := os.ReadFile("service.sh")
	if err != nil {
		t.Fatalf("read service.sh: %v", err)
	}
	if appConfigDir != "" {
		launcher := string(launcherData)
		redirectedLauncher := strings.Replace(launcher,
			`CONFIG_DIR="/data/data/$PKG/files/jniservice"`,
			`CONFIG_DIR="`+appConfigDir+`"`,
			1,
		)
		if redirectedLauncher == launcher {
			t.Fatal("could not redirect APK config directory in launcher test")
		}
		launcherData = []byte(redirectedLauncher)
	}
	launcherPath := filepath.Join(moduleDir, "service.sh")
	if err := os.WriteFile(launcherPath, launcherData, 0o700); err != nil {
		t.Fatalf("write temporary service.sh: %v", err)
	}
	configPath := filepath.Join(moduleDir, "jniservice.env")
	if config != "" {
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			t.Fatalf("write temporary jniservice.env: %v", err)
		}
	}

	commandDir := filepath.Join(moduleDir, "bin")
	if err := os.Mkdir(commandDir, 0o700); err != nil {
		t.Fatalf("create fake command directory: %v", err)
	}
	writeMagiskCommand(t, commandDir, "getprop", "#!/bin/sh\nprintf '1\\n'\n")
	for _, commandName := range []string{"sleep", "pkill"} {
		writeMagiskCommand(t, commandDir, commandName, "#!/bin/sh\nexit 0\n")
	}
	pmCommand := "#!/bin/sh\nexit 0\n"
	if appConfigDir != "" {
		pmCommand = "#!/bin/sh\nprintf 'package:center.dx.jni.jniservice\\n'\n"
	}
	writeMagiskCommand(t, commandDir, "pm", pmCommand)
	if appConfigDir != "" {
		for _, commandName := range []string{"chown"} {
			writeMagiskCommand(t, commandDir, commandName, "#!/bin/sh\nexit 0\n")
		}
		writeMagiskCommand(t, commandDir, "stat", "#!/bin/sh\nprintf '10192\\n'\n")
		writeMagiskCommand(t, commandDir, "am", "#!/bin/sh\nprintf 'started\\n' > \"$CAPTURE\"\n")
	}
	if failedCommand != "" {
		writeMagiskCommand(t, commandDir, failedCommand, "#!/bin/sh\nprintf 'simulated "+failedCommand+" failure\\n' >&2\nexit 42\n")
	}
	capturePath := filepath.Join(moduleDir, "app-process-env")
	appProcess := "#!/bin/sh\nprintf 'JNISERVICE_CONFIG=%s\\n' \"${JNISERVICE_CONFIG-}\" > \"$CAPTURE\"\nprintf 'JNISERVICE_LISTEN=%s\\n' \"${JNISERVICE_LISTEN-}\" >> \"$CAPTURE\"\nprintf 'JNISERVICE_PORT=%s\\n' \"${JNISERVICE_PORT-}\" >> \"$CAPTURE\"\n"
	writeMagiskCommand(t, commandDir, "app_process", appProcess)

	command := exec.Command("/bin/sh", launcherPath)
	command.Env = []string{
		"PATH=" + commandDir + ":/usr/bin:/bin",
		"CAPTURE=" + capturePath,
	}
	for name, value := range explicitEnvironment {
		command.Env = append(command.Env, name+"="+value)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, output, err
	}
	if appConfigDir != "" {
		return nil, output, nil
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(capturePath)
		if err == nil {
			return parseMagiskEnvironment(t, data), output, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, output, fmt.Errorf("app_process did not capture its environment; launcher log: %s", readMagiskLog(t, filepath.Join(moduleDir, "jniservice.log")))
}

// writeMagiskCommand creates a temporary executable used to isolate launcher control flow.
//
// Example: fake getprop completes boot immediately and fake app_process captures its environment.
func writeMagiskCommand(t *testing.T, directory string, name string, content string) {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
}

// parseMagiskEnvironment decodes the variables captured by fake app_process.
//
// Example: the captured JNISERVICE_CONFIG points app_process at the module config file.
func parseMagiskEnvironment(t *testing.T, data []byte) map[string]string {
	t.Helper()
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid captured environment line %q", line)
		}
		result[name] = value
	}
	return result
}

// readMagiskLog returns launcher output after the fake app_process timeout.
//
// Example: failing host launch tests retain shell diagnostics without real Magisk commands.
func readMagiskLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read launcher log: %v", err)
	}
	return string(data)
}
