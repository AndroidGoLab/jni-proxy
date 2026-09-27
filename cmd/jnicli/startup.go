package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	pb "github.com/AndroidGoLab/jni-proxy/proto/startup"
	"github.com/spf13/cobra"
)

// init registers the manually maintained startup commands without changing generated commands.
//
// Example: jnicli startup configure --adb-key-file paired-adbkey --enabled starts opt-in automation.
func init() {
	command := &cobra.Command{Use: "startup", Short: "Phone-local fixed-performance startup after unlock and trusted Wi-Fi"}
	configure := &cobra.Command{Use: "configure", Short: "Store a dedicated paired ADB key and startup opt-in", Args: cobra.NoArgs, RunE: configureStartup}
	configure.Flags().String("adb-key-file", "", "dedicated paired RSA private key file; omit to retain the stored key")
	configure.Flags().Bool("enabled", false, "enable automatic fixed-performance startup")
	command.AddCommand(configure,
		&cobra.Command{Use: "enable", Short: "Enable future startup and apply once", Args: cobra.NoArgs, RunE: enableStartup},
		&cobra.Command{Use: "disable", Short: "Cancel startup retries; leave current power mode unchanged", Args: cobra.NoArgs, RunE: disableStartup},
		&cobra.Command{Use: "apply", Short: "Start one asynchronous attempt; inspect status for its result", Args: cobra.NoArgs, RunE: applyStartup},
		&cobra.Command{Use: "status", Short: "Show redacted configuration and last command result", Args: cobra.NoArgs, RunE: startupStatus})
	rootCmd.AddCommand(command)
}

// readADBKey reads a bounded private file without displaying its contents.
//
// Example: a mistakenly selected large file is rejected before an RPC is made.
func readADBKey(path string) (_key string, _err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open dedicated ADB key: %w", err)
	}
	defer func() { _err = errors.Join(_err, file.Close()) }()
	data, err := io.ReadAll(io.LimitReader(file, 16*1024+1))
	if err != nil {
		return "", fmt.Errorf("read dedicated ADB key: %w", err)
	}
	if len(data) > 16*1024 {
		return "", errors.New("ADB key file exceeds 16 KiB")
	}
	return string(data), nil
}

// configureStartup sends the key only through the existing authenticated configuration RPC.
//
// Example: --enabled=false stores a key for an explicit apply without future startup automation.
func configureStartup(
	cmd *cobra.Command,
	_ []string,
) error {
	if err := requireVerifiedStartupMTLS(); err != nil {
		return err
	}
	path, err := cmd.Flags().GetString("adb-key-file")
	if err != nil {
		return err
	}
	enabled, err := cmd.Flags().GetBool("enabled")
	if err != nil {
		return err
	}
	var key string
	if path != "" {
		key, err = readADBKey(path)
		if err != nil {
			return err
		}
	}
	return setStartupConfiguration(cmd, key, enabled)
}

// requireVerifiedStartupMTLS rejects startup configuration over an unverified or incomplete CLI transport.
//
// Example: --insecure cannot upload the paired key to an unverified endpoint.
func requireVerifiedStartupMTLS() error {
	if flagInsecure {
		return errors.New("startup configuration requires verified mTLS; --insecure is not allowed")
	}
	if flagCert == "" || flagKey == "" || flagCA == "" {
		return errors.New("startup configuration requires --cert, --key and --ca for verified mTLS")
	}
	return nil
}

// setStartupConfiguration preserves the CLI's usual deadline and redacted protobuf output.
//
// Example: enable and disable reuse the existing key by sending an empty replacement.
func setStartupConfiguration(
	cmd *cobra.Command,
	key string,
	enabled bool,
) error {
	if err := requireVerifiedStartupMTLS(); err != nil {
		return err
	}
	ctx, cancel := requestContext(cmd)
	defer cancel()
	result, err := pb.NewStartupServiceClient(grpcConn).Configure(ctx, &pb.ConfigureRequest{PrivateKeyPem: key, Enabled: enabled})
	if err != nil {
		return err
	}
	return printProtoMessage(result)
}

// enableStartup opts into persisted future startup attempts and starts one current attempt.
//
// Example: jnicli startup enable uses the previously configured paired identity.
func enableStartup(
	cmd *cobra.Command,
	_ []string,
) error {
	return setStartupConfiguration(cmd, "", true)
}

// disableStartup cancels retries without executing the inverse power-mode command.
//
// Example: disabling automation leaves a previously successful fixed-performance request in place.
func disableStartup(
	cmd *cobra.Command,
	_ []string,
) error {
	return setStartupConfiguration(cmd, "", false)
}

// applyStartup initiates one asynchronous run without altering the persisted opt-in.
//
// Example: inspect startup status to distinguish acceptance from actual shell success.
func applyStartup(
	cmd *cobra.Command,
	_ []string,
) error {
	ctx, cancel := requestContext(cmd)
	defer cancel()
	result, err := pb.NewStartupServiceClient(grpcConn).Apply(ctx, &pb.ApplyRequest{})
	if err != nil {
		return err
	}
	return printProtoMessage(result)
}

// startupStatus reads only the last execution and credential-free configuration state.
//
// Example: succeeded refers to the last command, not a live power-mode measurement.
func startupStatus(
	cmd *cobra.Command,
	_ []string,
) error {
	ctx, cancel := requestContext(cmd)
	defer cancel()
	result, err := pb.NewStartupServiceClient(grpcConn).GetStatus(ctx, &pb.GetStatusRequest{})
	if err != nil {
		return err
	}
	return printProtoMessage(result)
}
