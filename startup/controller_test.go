package startup

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/AndroidGoLab/jni-proxy/adb"
	"github.com/stretchr/testify/require"
)

// identity creates an isolated private key for persistence and controller tests.
//
// Example: no test reads the developer's paired ADB key.
func identity(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

// TestConfiguration verifies atomic private storage and rejects corrupt persisted state.
//
// Example: invalid replacement keys preserve the last valid configuration.
func TestConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.json")
	config, err := loadConfig(path)
	require.NoError(t, err)
	require.False(t, config.Enabled)
	config = Config{Version: 1, Enabled: true, PrivateKey: identity(t)}
	require.NoError(t, saveConfig(path, config))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.EqualValues(t, 0600, info.Mode().Perm())
	got, err := loadConfig(path)
	require.NoError(t, err)
	require.Equal(t, config, got)
	invalid := config
	invalid.PrivateKey = "broken"
	require.Error(t, saveConfig(path, invalid))
	got, err = loadConfig(path)
	require.NoError(t, err)
	require.Equal(t, config, got)
	for _, data := range []string{"", "null", "{}", "{\"version\":2}", "{\"version\":1,\"unexpected\":true}", "{\"version\":1} {}", "{\"version\":1,\"enabled\":true}"} {
		require.NoError(t, os.WriteFile(path, []byte(data), 0600))
		_, err := loadConfig(path)
		require.Error(t, err)
	}
}

// discoveryEvent records an observable subscription boundary.
//
// Example: success must unsubscribe from further automatic command attempts.
type discoveryEvent struct {
	Generation uint64
	Enabled    bool
}

// testDiscovery captures subscription changes without Android timing.
//
// Example: tests replay a stale generation after disabling.
type testDiscovery struct{ Events chan discoveryEvent }

// Update records each lifecycle request in order.
//
// Example: the test consumes an enabled request before injecting a local port.
func (d testDiscovery) Update(
	_ context.Context,
	generation uint64,
	enabled bool,
) error {
	d.Events <- discoveryEvent{generation, enabled}
	return nil
}

// execution holds a controlled command invocation and its cancellation context.
//
// Example: releasing a cancelled old command proves stale results cannot overwrite new state.
type execution struct {
	Context  context.Context
	Port     adb.Port
	Result   chan error
	Returned chan struct{}
}

// testRunner exposes the actual action boundary while the controller owns lifecycle decisions.
//
// Example: no call can happen until a current discovery callback supplies a port.
type testRunner struct{ Calls chan execution }

// Run blocks until the test chooses completion, including deliberately late completion after cancellation.
//
// Example: a nil result means that the fixed command reported exit zero.
func (r testRunner) Run(
	ctx context.Context,
	_ string,
	port adb.Port,
) (adb.Result, error) {
	call := execution{ctx, port, make(chan error, 1), make(chan struct{})}
	r.Calls <- call
	err := <-call.Result
	close(call.Returned)
	return adb.Result{}, err
}

// TestControllerLifetime falsifies duplicate execution, stale callbacks and concurrent replacement work.
//
// Example: a fresh process loads enabled configuration without an Apply request.
func TestControllerLifetime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.json")
	discovery := testDiscovery{make(chan discoveryEvent, 20)}
	runner := testRunner{make(chan execution, 20)}
	c := New(path, runner, discovery)
	require.NoError(t, c.Start(t.Context()))
	require.False(t, (<-discovery.Events).Enabled)
	require.NoError(t, c.Configure(t.Context(), identity(t), true))
	first := <-discovery.Events
	require.True(t, first.Enabled)
	require.NoError(t, c.Start(t.Context()))
	select {
	case <-discovery.Events:
		t.Fatal("duplicate Start restarted discovery")
	default:
	}
	c.Discovered(t.Context(), first.Generation, 37000)
	old := <-runner.Calls
	c.Discovered(t.Context(), first.Generation, 37000)
	require.NoError(t, c.Configure(t.Context(), "", false))
	disabled := <-discovery.Events
	require.False(t, disabled.Enabled)
	<-old.Context.Done()
	c.Discovered(t.Context(), first.Generation, 37001)
	require.NoError(t, c.Configure(t.Context(), "", true))
	current := <-discovery.Events
	c.Discovered(t.Context(), current.Generation, 38000)
	select {
	case <-runner.Calls:
		t.Fatal("replacement overlapped cancelled command")
	default:
	}
	old.Result <- nil
	next := <-runner.Calls
	require.Equal(t, adb.Port(38000), next.Port)
	next.Result <- nil
	success := <-discovery.Events
	require.False(t, success.Enabled)
	status := c.Status()
	require.Equal(t, StateSucceeded, status.State)
	require.True(t, status.HasResult)
	encoded, err := json.Marshal(status)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE KEY")
	c.Discovered(t.Context(), current.Generation, 39000)
	select {
	case <-runner.Calls:
		t.Fatal("continued enforcement after success")
	default:
	}
	require.NoError(t, c.Apply(t.Context()))
	manual := <-discovery.Events
	require.True(t, manual.Enabled)
	c.Discovered(t.Context(), manual.Generation, 39000)
	last := <-runner.Calls
	last.Result <- adb.ErrAuthentication
	<-last.Returned
	// A new controller represents a new Android process, reading only the file.
	fresh := New(path, runner, discovery)
	require.NoError(t, fresh.Start(t.Context()))
	boot := <-discovery.Events
	require.True(t, boot.Enabled)
	fresh.Discovered(t.Context(), boot.Generation, 40000)
	bootCall := <-runner.Calls
	bootCall.Result <- nil
	require.False(t, (<-discovery.Events).Enabled)
	require.Equal(t, StateSucceeded, fresh.Status().State)
}

// TestUnconfiguredAndCorruptState confirms explicit failures instead of silently enabling defaults.
//
// Example: the operator can inspect a corrupt file through redacted status and replace it.
func TestUnconfiguredAndCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.json")
	discovery := testDiscovery{make(chan discoveryEvent, 10)}
	c := New(path, testRunner{make(chan execution, 1)}, discovery)
	require.Error(t, c.Apply(t.Context()))
	require.Error(t, c.Configure(t.Context(), "", true))
	require.NoError(t, os.WriteFile(path, []byte("invalid"), 0600))
	c = New(path, testRunner{make(chan execution, 1)}, discovery)
	require.Equal(t, StateFailed, c.Status().State)
	require.Error(t, c.Start(t.Context()))
	require.NoError(t, c.Configure(t.Context(), identity(t), false))
	require.False(t, c.Status().Enabled)
	require.False(t, (<-discovery.Events).Enabled)
	require.False(t, errors.Is(c.Apply(t.Context()), os.ErrNotExist))
}

// TestTransientRetriesAndEndpointChanges exercises bounded retry timing with a virtual clock.
//
// Example: exhausted endpoints stay idle until service loss and rediscovery provide new state.
func TestTransientRetriesAndEndpointChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "startup.json")
	require.NoError(t, saveConfig(path, Config{Version: 1, Enabled: true, PrivateKey: identity(t)}))
	synctest.Test(t, func(t *testing.T) {
		discovery := testDiscovery{make(chan discoveryEvent, 20)}
		runner := testRunner{make(chan execution, 20)}
		c := New(path, runner, discovery)
		require.NoError(t, c.Start(t.Context()))
		generation := (<-discovery.Events).Generation
		c.DiscoveryFailed(generation-1, 7)
		require.Empty(t, c.Status().LastError)
		c.DiscoveryFailed(generation, 8)
		require.Contains(t, c.Status().LastError, "code 8")
		c.Discovered(t.Context(), generation, 37001)
		for range 3 {
			(<-runner.Calls).Result <- &net.OpError{Op: "dial", Net: "tcp", Err: io.EOF}
		}
		synctest.Wait()
		require.Equal(t, StateFailed, c.Status().State)
		require.Equal(t, 3, c.Status().Attempts)
		c.Discovered(t.Context(), generation, 37001)
		select {
		case <-runner.Calls:
			t.Fatal("retried an exhausted unchanged endpoint")
		default:
		}
		c.Discovered(t.Context(), generation, 0)
		c.Discovered(t.Context(), generation, 38001)
		call := <-runner.Calls
		call.Result <- io.EOF
		synctest.Wait()
		require.NoError(t, c.Configure(t.Context(), "", false))
		require.False(t, (<-discovery.Events).Enabled)
		synctest.Wait()
		require.Equal(t, StateDisabled, c.Status().State)
		select {
		case <-runner.Calls:
			t.Fatal("retry survived disabling")
		default:
		}
	})
	for _, err := range []error{adb.ErrAuthentication, adb.ErrProtocol, context.Canceled, errors.New("permanent")} {
		require.False(t, transient(err))
	}
	for _, err := range []error{io.EOF, io.ErrUnexpectedEOF, context.DeadlineExceeded} {
		require.True(t, transient(err))
	}
}

// failedDiscovery makes platform subscription errors observable without pretending discovery succeeded.
//
// Example: Configure may persist successfully but must still report an unavailable platform service.
type failedDiscovery struct{}

// Update returns a stable external-boundary failure for recovery tests.
//
// Example: an unavailable Android NSD manager produces failed status.
func (failedDiscovery) Update(
	_ context.Context,
	_ uint64,
	_ bool,
) error {
	return errors.New("NSD unavailable")
}

// TestStorageAndPlatformFailures rejects inaccessible files and surfaces subscription errors.
//
// Example: a writable temporary file cannot replace a directory at the configured destination.
func TestStorageAndPlatformFailures(t *testing.T) {
	dir := t.TempDir()
	_, err := loadConfig(dir)
	require.Error(t, err)
	config := Config{Version: 1, Enabled: false}
	require.Error(t, saveConfig(dir, config))
	path := filepath.Join(dir, "startup.json")
	require.NoError(t, saveConfig(path, config))
	require.NoError(t, os.Chmod(path, 0644))
	_, err = loadConfig(path)
	require.Error(t, err)
	require.Error(t, saveConfig(filepath.Join(path, "child.json"), config))
	c := New(filepath.Join(dir, "missing.json"), ADBRunner{}, failedDiscovery{})
	require.Error(t, c.Start(t.Context()))
	require.Contains(t, c.Status().LastError, "NSD unavailable")
	_, err = (ADBRunner{}).Run(t.Context(), "invalid", 37000)
	require.Error(t, err)
}
