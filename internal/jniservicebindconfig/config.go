// Package jniservicebindconfig resolves Android jniservice listener settings from
// defaults, durable config, staged launch config, and process environment.
//
// Example: Load keeps transient launcher overrides separate from its durable config file.
package jniservicebindconfig

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
)

// Config contains the jniservice listener host and port.
//
// Example: Config{Listen: "127.0.0.1", Port: "50051"} represents the safe defaults.
type Config struct {
	Listen string
	Port   string
}

// Parse reads listener values from KEY=VALUE lines without executing shell
// syntax. Unknown keys and comments are ignored.
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
			if err := validateListenerAddress(value); err != nil {
				return Config{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			config.Listen = value
		case "JNISERVICE_PORT":
			config.Port = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read listener config: %w", err)
	}
	return config, nil
}

// Load applies safe defaults, the selected config file, the optional staged
// launch config, and then nonempty listener environment overrides. Only missing
// files are optional; other read and parse errors stop startup so the service
// cannot silently bind to an unintended address.
//
// Example: env["JNISERVICE_LAUNCH_CONFIG"] selects transient listener overrides for this launch.
func Load(path string, env map[string]string) (Config, error) {
	file, err := readOptionalConfigFile(path)
	if err != nil {
		return Config{}, err
	}
	launch, err := readOptionalConfigFile(env["JNISERVICE_LAUNCH_CONFIG"])
	if err != nil {
		return Config{}, err
	}
	if value := env["JNISERVICE_LISTEN"]; value != "" {
		if err := validateListenerAddress(value); err != nil {
			return Config{}, fmt.Errorf("invalid JNISERVICE_LISTEN environment value: %w", err)
		}
	}
	launchEnv := map[string]string{
		"JNISERVICE_LISTEN": launch.Listen,
		"JNISERVICE_PORT":   launch.Port,
	}
	return Merge(env, Merge(launchEnv, file)), nil
}

// readOptionalConfigFile reads a listener file, treating only absence as empty configuration.
//
// Example: a missing staged launch file contributes no listener overrides.
func readOptionalConfigFile(path string) (Config, error) {
	if path == "" {
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		config, err := Parse(data)
		if err != nil {
			return Config{}, fmt.Errorf("parse listener config %q: %w", path, err)
		}
		return config, nil
	case errors.Is(err, os.ErrNotExist):
		return Config{}, nil
	default:
		return Config{}, fmt.Errorf("read listener config %q: %w", path, err)
	}
}

// validateListenerAddress rejects IP literals that bind every local interface so
// the service remains explicitly scoped to selected addresses.
//
// Example: validateListenerAddress("0.0.0.0") errors, while a concrete interface IP is accepted.
func validateListenerAddress(address string) error {
	host := address
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	parsed, err := netip.ParseAddr(host)
	if err == nil && parsed.WithZone("").Unmap().IsUnspecified() {
		return fmt.Errorf("unspecified listener address %q is not allowed", address)
	}
	return nil
}

// Merge combines the loopback defaults, file values, and nonempty environment
// overrides in that order. Only listener keys are consulted.
//
// Example: Merge(env, Config{Port: "50052"}) applies the file port before environment overrides.
func Merge(env map[string]string, file Config) Config {
	result := Config{Listen: "127.0.0.1", Port: "50051"}
	if file.Listen != "" {
		result.Listen = file.Listen
	}
	if file.Port != "" {
		result.Port = file.Port
	}
	if value := env["JNISERVICE_LISTEN"]; value != "" {
		result.Listen = value
	}
	if value := env["JNISERVICE_PORT"]; value != "" {
		result.Port = value
	}
	return result
}
