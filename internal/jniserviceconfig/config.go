// Package jniserviceconfig resolves the small, non-shell configuration used by
// the Android jniservice launchers.
package jniserviceconfig

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config contains the listener host and port.
//
// Example: the default config binds to 127.0.0.1:50051.
type Config struct {
	Listen string
	Port   string
}

// Parse reads listener values from KEY=VALUE lines. Unknown keys and comments
// are ignored. Values are data, never shell syntax: this parser does not expand
// or execute them.
//
// Example: Parse([]byte("JNISERVICE_PORT=50052\n")) returns port 50052.
func Parse(data []byte) (Config, error) {
	var config Config
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("line %d: want KEY=VALUE", lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			first, last := value[0], value[len(value)-1]
			if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
				value = value[1 : len(value)-1]
			}
		}
		switch key {
		case "JNISERVICE_LISTEN":
			config.Listen = value
		case "JNISERVICE_PORT":
			config.Port = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return config, nil
}

// Load applies defaults, one optional config file, then nonempty environment
// overrides. Only an absent file is optional; read and parse failures are
// returned so the service cannot start with a different listener by mistake.
//
// Example: Load("/data/adb/modules/jniservice/jniservice.env",
// map[string]string{"JNISERVICE_PORT": "50052"}) resolves the Magisk settings.
func Load(path string, env map[string]string) (Config, error) {
	var fileConfig Config
	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			fileConfig, err = Parse(data)
			if err != nil {
				return Config{}, fmt.Errorf("parse listener config %q: %w", path, err)
			}
		case errors.Is(err, os.ErrNotExist):
			// A missing listener file leaves the safe defaults in effect.
		default:
			return Config{}, fmt.Errorf("read listener config %q: %w", path, err)
		}
	}
	return Merge(env, fileConfig), nil
}

// Merge applies the loopback defaults, then config-file values, then nonempty
// listener environment values.
//
// Example: Merge(map[string]string{"JNISERVICE_PORT": "50053"}, Config{})
// returns the default host with port 50053.
func Merge(env map[string]string, file Config) Config {
	result := Config{Listen: "127.0.0.1", Port: "50051"}
	if file.Listen != "" {
		result.Listen = file.Listen
	}
	if file.Port != "" {
		result.Port = file.Port
	}
	if env != nil {
		if value := env["JNISERVICE_LISTEN"]; value != "" {
			result.Listen = value
		}
		if value := env["JNISERVICE_PORT"]; value != "" {
			result.Port = value
		}
	}
	return result
}
