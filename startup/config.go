// Package startup owns the opt-in, once-per-startup fixed-performance action.
package startup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/AndroidGoLab/jni-proxy/adb"
)

// Config is the credential-protected startup file; it must never be exposed as RPC status.
//
// Example: enabling stores a dedicated paired RSA key and the opt-in flag together.
type Config struct {
	Version    int    `json:"version"`
	Enabled    bool   `json:"enabled"`
	PrivateKey string `json:"private_key"`
}

// validate rejects unknown formats and enabled configurations without a valid dedicated key.
//
// Example: a malformed replacement leaves the previous file intact.
func (c Config) validate() error {
	if c.Version != 1 {
		return errors.New("unsupported startup configuration version")
	}
	if c.PrivateKey == "" {
		if c.Enabled {
			return errors.New("configure a paired ADB private key before enabling startup")
		}
		return nil
	}
	_, err := adb.NewClient([]byte(c.PrivateKey))
	return err
}

// loadConfig defaults only a missing file to disabled; corruption remains an observable error.
//
// Example: invalid JSON cannot silently discard an operator's configured action.
func loadConfig(path string) (_config Config, _err error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Version: 1}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("open startup configuration: %w", err)
	}
	defer func() { _err = errors.Join(_err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return Config{}, fmt.Errorf("stat startup configuration: %w", err)
	}
	if info.Size() > 32*1024 || info.Mode().Perm() != 0600 {
		return Config{}, errors.New("startup configuration must be at most 32 KiB and mode 0600")
	}
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errors.New("invalid startup configuration JSON")
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("unexpected trailing startup configuration data")
	}
	if err := config.validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// saveConfig atomically replaces the private file and synchronizes it and its parent directory.
//
// Example: a restart sees either the previous complete opt-in/key pair or the new pair.
func saveConfig(
	path string,
	config Config,
) (_err error) {
	if err := config.validate(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create startup directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".startup-*")
	if err != nil {
		return fmt.Errorf("create private startup file: %w", err)
	}
	temporary := f.Name()
	defer func() {
		if f != nil {
			_err = errors.Join(_err, f.Close())
		}
		if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			_err = errors.Join(_err, err)
		}
	}()
	if err := json.NewEncoder(f).Encode(config); err != nil {
		return fmt.Errorf("write startup configuration: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync startup configuration: %w", err)
	}
	closeErr := f.Close()
	f = nil
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace startup configuration: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
