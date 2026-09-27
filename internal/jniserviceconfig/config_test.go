package jniserviceconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUsesSafeDefaultsWhenConfigIsAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jniservice.env")
	got, err := Load(path, nil)
	if err != nil {
		t.Fatalf("load absent optional config: %v", err)
	}
	want := Config{Listen: "127.0.0.1", Port: "50051"}
	if got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

func TestLoadAppliesDefaultsFileThenEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jniservice.env")
	if err := os.WriteFile(path, []byte("JNISERVICE_LISTEN=192.168.1.42\nJNISERVICE_PORT=50052\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	env := map[string]string{
		"JNISERVICE_LISTEN":   "192.168.1.43",
		"JNISERVICE_PORT":     "",
		"JNISERVICE_DATA_DIR": "/environment-only/data",
	}
	got, err := Load(path, env)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	want := Config{Listen: "192.168.1.43", Port: "50052"}
	if got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

func TestLoadRejectsMalformedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jniservice.env")
	if err := os.WriteFile(path, []byte("JNISERVICE_LISTEN=192.168.1.42\nnot-a-setting\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := Load(path, nil); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("load malformed config error = %v, want line 2 parse error", err)
	}
}

func TestLoadReturnsNonAbsenceReadErrors(t *testing.T) {
	path := t.TempDir()
	if _, err := Load(path, nil); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("load directory config error = %v, want path-specific read error", err)
	}
}

func TestLoadIgnoresDataDirectoryInFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jniservice.env")
	if err := os.WriteFile(path, []byte("JNISERVICE_DATA_DIR=/file-only/data\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	got, err := Load(path, nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	want := Config{Listen: "127.0.0.1", Port: "50051"}
	if got != want {
		t.Fatalf("config = %+v, want listener defaults %+v", got, want)
	}
}

func TestLoadDoesNotUseDataDirectoryEnvironmentAsListenerConfig(t *testing.T) {
	got, err := Load("", map[string]string{"JNISERVICE_DATA_DIR": "/environment-only/data"})
	if err != nil {
		t.Fatalf("load environment config: %v", err)
	}
	want := Config{Listen: "127.0.0.1", Port: "50051"}
	if got != want {
		t.Fatalf("config = %+v, want listener defaults %+v", got, want)
	}
}

func TestParseDoesNotExecuteShellSyntax(t *testing.T) {
	got, err := Parse([]byte("JNISERVICE_LISTEN=$(touch /tmp/should-not-exist)\n"))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if got.Listen != "$(touch /tmp/should-not-exist)" {
		t.Fatalf("listen value = %q, want literal value", got.Listen)
	}
}
