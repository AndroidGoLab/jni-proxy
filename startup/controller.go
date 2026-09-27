package startup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/AndroidGoLab/jni-proxy/adb"
)

// FixedPerformanceCommand is the sole startup action; RPC callers cannot supply shell text.
//
// Example: a successful startup invokes exactly this command through the paired shell identity.
const FixedPerformanceCommand = "cmd power set-fixed-performance-mode-enabled true"

// State describes execution progress, not the current hardware power or thermal state.
//
// Example: StateSucceeded means the last fixed command reported exit zero.
type State string

// Startup states keep disabled, waiting, authentication and command outcomes distinct.
//
// Example: StateWaiting is expected until trusted Wi-Fi exposes the local TLS service.
const (
	StateDisabled             State = "disabled"
	StateWaiting              State = "waiting_for_adb"
	StateRunning              State = "running"
	StateSucceeded            State = "succeeded"
	StateAuthenticationFailed State = "authentication_failed"
	StateFailed               State = "failed"
)

// Status is a credential-free snapshot of configuration and the last command in this process.
//
// Example: HasResult distinguishes exit zero from a command that never returned an exit.
type Status struct {
	Enabled          bool
	Configured       bool
	State            State
	Generation       uint64
	Port             adb.Port
	Attempts         int
	HasResult        bool
	LastExitCode     int
	LastRunUnixMilli int64
	LastError        string
}

// Runner performs the fixed action using a dedicated private key and discovered local port.
//
// Example: ADBRunner constructs a new TLS identity for each bounded attempt.
type Runner interface {
	// Run returns the fixed command result, honoring cancellation throughout socket I/O.
	//
	// Example: disabling startup cancels a blocked local ADB connection.
	Run(ctx context.Context, privateKey string, port adb.Port) (adb.Result, error)
}

// ADBRunner executes the single literal command without a host ADB daemon.
//
// Example: a phone APK uses ADBRunner after Android NSD identifies its own service.
type ADBRunner struct{}

// Run creates an ADB client and performs the fixed-performance command.
//
// Example: a revoked key produces adb.ErrAuthentication for the status boundary.
func (ADBRunner) Run(
	ctx context.Context,
	privateKey string,
	port adb.Port,
) (adb.Result, error) {
	client, err := adb.NewClient([]byte(privateKey))
	if err != nil {
		return adb.Result{}, err
	}
	return client.Shell(ctx, port, FixedPerformanceCommand)
}

// Discovery controls platform service subscriptions; Update must not synchronously call the controller.
//
// Example: Android posts subscription changes to its main looper and returns promptly.
type Discovery interface {
	// Update replaces subscription state and tags all subsequent callbacks with generation.
	//
	// Example: a callback queued before disable carries an old generation and is ignored.
	Update(ctx context.Context, generation uint64, enabled bool) error
}

// Controller serializes configuration, subscriptions and the one active action under a single invariant.
//
// Example: reconfiguration cancels old I/O and waits for its completion before starting new work.
type Controller struct {
	mu            sync.Mutex
	path          string
	runner        Runner
	discovery     Discovery
	config        Config
	loadError     error
	status        Status
	started       bool
	wanted        bool
	port          adb.Port
	attemptedPort adb.Port
	cancel        context.CancelFunc
}

// New loads persisted opt-in state without starting discovery or executing commands.
//
// Example: the APK registers the controller before its application context is initialized.
func New(
	path string,
	runner Runner,
	discovery Discovery,
) *Controller {
	config, err := loadConfig(path)
	c := &Controller{path: path, runner: runner, discovery: discovery, config: config, loadError: err}
	c.status = Status{Enabled: config.Enabled, Configured: config.PrivateKey != "", State: StateDisabled}
	if err != nil {
		c.status.State = StateFailed
		c.status.LastError = err.Error()
	}
	return c
}

// Status returns a value snapshot with no key material or mutable references.
//
// Example: the authenticated status RPC exposes this record to the setup client.
func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Start initializes the service lifetime once; duplicate boot deliveries do not repeat a successful action.
//
// Example: a fresh process reads Enabled from disk and starts without any Apply RPC.
func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return c.loadError
	}
	c.started = true
	if c.loadError != nil {
		return c.loadError
	}
	return c.restartLocked(ctx, c.config.Enabled)
}

// Configure atomically saves opt-in and an optional replacement private key, then replaces the active generation.
//
// Example: an empty privateKey retains the configured identity when disabling or re-enabling.
func (c *Controller) Configure(
	ctx context.Context,
	privateKey string,
	enabled bool,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	config := c.config
	config.Version = 1
	config.Enabled = enabled
	if privateKey != "" {
		config.PrivateKey = privateKey
	}
	if err := saveConfig(c.path, config); err != nil {
		return err
	}
	c.config = config
	c.loadError = nil
	return c.restartLocked(ctx, enabled && c.started)
}

// Apply requests another single run without changing the persisted enabled flag.
//
// Example: an operator can test the paired key while future automatic startup remains disabled.
func (c *Controller) Apply(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadError != nil {
		return c.loadError
	}
	if c.config.PrivateKey == "" {
		return errors.New("configure a paired ADB key first")
	}
	if !c.started {
		return errors.New("Android startup discovery is not initialized")
	}
	return c.restartLocked(ctx, true)
}

// restartLocked replaces execution intent and invalidates callbacks before posting platform work.
//
// Example: an old command finishing after disable cannot overwrite disabled status.
func (c *Controller) restartLocked(
	ctx context.Context,
	wanted bool,
) error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wanted = wanted
	c.port = 0
	c.attemptedPort = 0
	c.status.Generation++
	c.status.Enabled = c.config.Enabled
	c.status.Configured = c.config.PrivateKey != ""
	c.status.Port = 0
	c.status.Attempts = 0
	c.status.LastError = ""
	c.status.State = StateDisabled
	if wanted {
		c.status.State = StateWaiting
	}
	if err := c.discovery.Update(ctx, c.status.Generation, wanted); err != nil {
		c.status.State = StateFailed
		c.status.LastError = fmt.Sprintf("update ADB discovery: %v", err)
		return err
	}
	return nil
}

// Discovered consumes current own-device ports; zero means the discovered endpoint was lost.
//
// Example: a changed dynamic port cancels the old socket and starts on the new endpoint.
func (c *Controller) Discovered(
	ctx context.Context,
	generation uint64,
	port adb.Port,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.status.Generation || !c.wanted || port < 0 || port > 65535 || c.port == port {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.port = port
	c.status.Port = port
	c.status.State = StateWaiting
	c.attemptedPort = 0
	c.launchLocked(ctx)
}

// DiscoveryFailed records a platform failure without exposing any credentials or changing opt-in.
//
// Example: NSD registration failure stays visible until a network event renews discovery.
func (c *Controller) DiscoveryFailed(
	generation uint64,
	code int,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.status.Generation || !c.wanted {
		return
	}
	c.status.LastError = fmt.Sprintf("Android ADB discovery failed (code %d); waiting for network/service update", code)
}

// launchLocked starts an action only when the previous action has fully returned.
//
// Example: repeated resolution of an exhausted port does not create an unbounded retry loop.
func (c *Controller) launchLocked(ctx context.Context) {
	if c.cancel != nil || !c.wanted || c.port == 0 || c.attemptedPort == c.port {
		return
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.cancel = cancel
	port, generation, key := c.port, c.status.Generation, c.config.PrivateKey
	c.attemptedPort = port
	c.status.State = StateRunning
	go func() {
		defer cancel()
		result, err := c.execute(runCtx, generation, key, port)
		c.finish(runCtx, generation, port, result, err)
	}()
}

// execute bounds retries of opaque transient socket failures and subscribes to cancellation while waiting.
//
// Example: refused connections retry twice, while revoked pairing immediately needs operator attention.
func (c *Controller) execute(
	ctx context.Context,
	generation uint64,
	key string,
	port adb.Port,
) (adb.Result, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return adb.Result{}, err
		}
		c.recordAttempt(generation, port)
		slog.DebugContext(ctx, "startup fixed-performance attempt", "generation", generation, "port", port, "attempt", attempt)
		result, err := c.runner.Run(ctx, key, port)
		if err == nil || attempt == 3 || !transient(err) {
			return result, err
		}
		timer := time.NewTimer(time.Duration(attempt) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return adb.Result{}, ctx.Err()
		case <-timer.C:
		}
	}
}

// transient admits only socket/EOF failures, excluding authentication and protocol errors.
//
// Example: unsupported shell-v2 is not retried three times.
func transient(err error) bool {
	if errors.Is(err, adb.ErrAuthentication) || errors.Is(err, adb.ErrProtocol) || errors.Is(err, context.Canceled) {
		return false
	}
	var networkError *net.OpError
	return errors.As(err, &networkError) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded)
}

// recordAttempt updates current-generation telemetry without retaining the private key.
//
// Example: stale worker attempts never increment a replacement generation's counter.
func (c *Controller) recordAttempt(
	generation uint64,
	port adb.Port,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.Generation == generation && c.port == port {
		c.status.Attempts++
	}
}

// finish records only current results and relinquishes the one-action slot before launching pending work.
//
// Example: success unsubscribes and prevents continuous power-mode enforcement.
func (c *Controller) finish(
	ctx context.Context,
	generation uint64,
	port adb.Port,
	result adb.Result,
	err error,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancel = nil
	if generation != c.status.Generation || port != c.port || !c.wanted {
		c.launchLocked(ctx)
		return
	}
	c.status.LastRunUnixMilli = time.Now().UnixMilli()
	c.status.HasResult = false
	var exitError *adb.ExitError
	if err == nil || errors.As(err, &exitError) {
		c.status.HasResult = true
		c.status.LastExitCode = result.ExitCode
	}
	switch {
	case err == nil:
		c.status.State = StateSucceeded
		c.status.LastError = ""
		c.wanted = false
		if err := c.discovery.Update(ctx, generation, false); err != nil {
			c.status.LastError = fmt.Sprintf("stop ADB discovery: %v", err)
		}
		slog.InfoContext(ctx, "startup fixed-performance command completed", "generation", generation, "port", port, "exit", result.ExitCode)
	case errors.Is(err, adb.ErrAuthentication):
		c.status.State = StateAuthenticationFailed
		c.status.LastError = adb.ErrAuthentication.Error()
	default:
		c.status.State = StateFailed
		c.status.LastError = err.Error()
	}
	if err != nil {
		slog.WarnContext(ctx, "startup fixed-performance command failed", "generation", generation, "port", port, "error", c.status.LastError)
	}
}
