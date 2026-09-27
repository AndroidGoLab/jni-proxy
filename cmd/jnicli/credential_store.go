package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// jnicliCredentialDirectory is the persistent per-user credential root.
const jnicliCredentialDirectory = ".jnicli"

// credentialPaths names the mTLS files used by one jnicli profile.
type credentialPaths struct {
	cert string
	key  string
	ca   string
}

// credentialPathsFor resolves a profile below the supplied home directory.
func credentialPathsFor(homeDir, profile string) credentialPaths {
	baseDir := filepath.Join(homeDir, jnicliCredentialDirectory)
	if profile != "" {
		baseDir = filepath.Join(baseDir, profile)
	}

	return credentialPaths{
		cert: filepath.Join(baseDir, "client.crt"),
		key:  filepath.Join(baseDir, "client.key"),
		ca:   filepath.Join(baseDir, "ca.crt"),
	}
}

// persistentCredentialPaths resolves a profile below the current user's home.
func persistentCredentialPaths(profile string) (credentialPaths, error) {
	if err := validateCredentialProfile(profile); err != nil {
		return credentialPaths{}, err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return credentialPaths{}, fmt.Errorf("finding home directory: %w", err)
	}

	return credentialPathsFor(homeDir, profile), nil
}

// validateCredentialProfile prevents profile paths from escaping ~/.jnicli.
func validateCredentialProfile(profile string) error {
	if profile == "" {
		return nil
	}

	if profile == "." || profile == ".." || filepath.Base(profile) != profile {
		return fmt.Errorf("credential profile %q must be a single directory name", profile)
	}

	return nil
}

// credentialPathsComplete reports whether all three credential files exist.
func credentialPathsComplete(paths credentialPaths) (bool, error) {
	for _, path := range []string{paths.cert, paths.key, paths.ca} {
		info, err := os.Stat(path)
		switch {
		case err == nil && !info.IsDir():
			continue
		case err == nil:
			return false, nil
		case errors.Is(err, os.ErrNotExist):
			return false, nil
		default:
			return false, fmt.Errorf("checking credential file %s: %w", path, err)
		}
	}

	return true, nil
}

// applyDefaultCredentialFlags fills mTLS flags from the selected profile.
func applyDefaultCredentialFlags(cmdName, parentName string) error {
	if cmdName == "register" && parentName == "auth" {
		return nil
	}

	if flagCert != "" || flagKey != "" || flagCA != "" {
		return nil
	}
	paths, err := persistentCredentialPaths(flagProfile)
	if err != nil {
		return err
	}

	complete, err := credentialPathsComplete(paths)
	if err != nil {
		return err
	}
	if !complete {
		if flagProfile == "" {
			return nil
		}

		return fmt.Errorf("credentials for profile %q are incomplete under %s", flagProfile, filepath.Dir(paths.cert))
	}

	flagCert = paths.cert
	flagKey = paths.key
	flagCA = paths.ca
	return nil
}
