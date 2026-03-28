package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	recpb "github.com/AndroidGoLab/jni-proxy/proto/recorder"
)

// Android MediaRecorder constants for audio-only recording.
const (
	micAudioSourceMIC       = 1      // MediaRecorder.AudioSource.MIC
	micOutputFormatMPEG4    = 2      // MediaRecorder.OutputFormat.MPEG_4
	micAudioEncoderAAC      = 3      // MediaRecorder.AudioEncoder.AAC
	micAudioEncodingBitRate = 128000 // 128 kbps
	micAudioSamplingRate    = 44100  // 44.1 kHz
)

var microphoneCmd = &cobra.Command{
	Use:   "microphone",
	Short: "Microphone operations (record)",
}

var microphoneRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record audio from the microphone via MediaRecorder (typed gRPC)",
	Long: `Records audio using the Android MediaRecorder API,
driven through typed gRPC calls (with raw JNI only for the constructor).
The resulting M4A is written to stdout or a file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		duration, _ := cmd.Flags().GetDuration("duration")
		if duration <= 0 {
			return fmt.Errorf("--duration must be positive")
		}
		output, _ := cmd.Flags().GetString("output")

		// Recording duration + margin for setup/teardown.
		ctx, cancel := context.WithTimeout(cmd.Context(), duration+setupTeardownMargin)
		defer cancel()

		client := pb.NewJNIServiceClient(grpcConn)
		j := &jniCaller{client: client}
		mrClient := recpb.NewMediaRecorderServiceClient(grpcConn)

		data, err := recordAudio(ctx, j, mrClient, duration)
		if err != nil {
			return err
		}

		switch output {
		case "-", "":
			_, err := os.Stdout.Write(data)
			return err
		default:
			if err := os.WriteFile(output, data, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", output, err)
			}
			fmt.Fprintf(os.Stderr, "Saved %d bytes to %s\n", len(data), output)
			return nil
		}
	},
}

func init() {
	microphoneRecordCmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	microphoneRecordCmd.Flags().DurationP("duration", "d", 10*time.Second, "recording duration")
	microphoneCmd.AddCommand(microphoneRecordCmd)
	rootCmd.AddCommand(microphoneCmd)
}

// recordAudio orchestrates audio-only recording via MediaRecorder using
// the typed gRPC client for all MediaRecorder operations (except the
// constructor, which still uses raw JNI).
func recordAudio(
	ctx context.Context,
	j *jniCaller,
	mrClient recpb.MediaRecorderServiceClient,
	duration time.Duration,
) ([]byte, error) {
	appCtx, err := j.getAppContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting app context: %w", err)
	}

	// Output path on the device.
	// Prefer the app's cache directory (works inside APKs without
	// WRITE_EXTERNAL_STORAGE); fall back to /sdcard/ for non-APK mode.
	outputPath, cacheDirErr := getCacheDirPath(ctx, j, appCtx)
	if cacheDirErr != nil {
		outputPath = "/sdcard"
	}
	outputPath += "/recording.m4a"

	// Create the MediaRecorder via raw JNI (no typed constructor RPC).
	recorder, err := createMediaRecorder(ctx, j, appCtx)
	if err != nil {
		return nil, fmt.Errorf("creating MediaRecorder: %w", err)
	}
	defer func() {
		if _, releaseErr := mrClient.Release(ctx, &recpb.ReleaseRequest{Handle: recorder}); releaseErr != nil {
			fmt.Fprintf(os.Stderr, "warning: releasing MediaRecorder: %v\n", releaseErr)
		}
	}()

	// Configure the recorder via typed gRPC calls.
	if err := configureAudioRecorder(ctx, mrClient, recorder, outputPath); err != nil {
		return nil, fmt.Errorf("configuring MediaRecorder: %w", err)
	}

	// Prepare.
	if _, err := mrClient.Prepare(ctx, &recpb.PrepareRequest{Handle: recorder}); err != nil {
		return nil, fmt.Errorf("calling MediaRecorder.prepare: %w", err)
	}

	// Start recording.
	if _, err := mrClient.Start(ctx, &recpb.StartRequest{Handle: recorder}); err != nil {
		return nil, fmt.Errorf("calling MediaRecorder.start: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Recording audio for %v...\n", duration)
	select {
	case <-time.After(duration):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Stop recording.
	if _, err := mrClient.Stop(ctx, &recpb.StopRequest{Handle: recorder}); err != nil {
		return nil, fmt.Errorf("calling MediaRecorder.stop: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Recording stopped.\n")

	// Read the recorded file back via JNI.
	fmt.Fprintf(os.Stderr, "Reading recorded file...\n")
	return readFileViaJNI(ctx, j, outputPath)
}

// createMediaRecorder instantiates an android.media.MediaRecorder via raw
// JNI. It tries the Context-accepting constructor (API 31+) first and
// falls back to the no-arg constructor for older APIs.
func createMediaRecorder(ctx context.Context, j *jniCaller, appCtx int64) (int64, error) {
	mrCls, err := j.findClass(ctx, "android/media/MediaRecorder")
	if err != nil {
		return 0, fmt.Errorf("finding MediaRecorder class: %w", err)
	}

	// MediaRecorder(Context) constructor (API 31+).
	var recorder int64
	ctor, ctorErr := j.getMethodID(ctx, mrCls, "<init>", "(Landroid/content/Context;)V")
	if ctorErr == nil {
		recorder, err = j.newObject(ctx, mrCls, ctor, objVal(appCtx))
	}
	if ctorErr != nil || err != nil {
		// Fall back to no-arg constructor for older APIs.
		ctor, err = j.getMethodID(ctx, mrCls, "<init>", "()V")
		if err != nil {
			return 0, fmt.Errorf("getting MediaRecorder constructor: %w", err)
		}
		recorder, err = j.newObject(ctx, mrCls, ctor)
		if err != nil {
			return 0, fmt.Errorf("creating MediaRecorder: %w", err)
		}
	}

	return recorder, nil
}

// configureAudioRecorder sets up MediaRecorder parameters for audio-only
// recording using the typed gRPC client.
func configureAudioRecorder(
	ctx context.Context,
	mrClient recpb.MediaRecorderServiceClient,
	recorder int64,
	outputPath string,
) error {
	if _, err := mrClient.SetAudioSource(ctx, &recpb.SetAudioSourceRequest{
		Handle: recorder,
		Arg0:   micAudioSourceMIC,
	}); err != nil {
		return fmt.Errorf("SetAudioSource: %w", err)
	}

	if _, err := mrClient.SetOutputFormat(ctx, &recpb.SetOutputFormatRequest{
		Handle: recorder,
		Arg0:   micOutputFormatMPEG4,
	}); err != nil {
		return fmt.Errorf("SetOutputFormat: %w", err)
	}

	if _, err := mrClient.SetAudioEncoder(ctx, &recpb.SetAudioEncoderRequest{
		Handle: recorder,
		Arg0:   micAudioEncoderAAC,
	}); err != nil {
		return fmt.Errorf("SetAudioEncoder: %w", err)
	}

	if _, err := mrClient.SetAudioEncodingBitRate(ctx, &recpb.SetAudioEncodingBitRateRequest{
		Handle: recorder,
		Arg0:   micAudioEncodingBitRate,
	}); err != nil {
		return fmt.Errorf("SetAudioEncodingBitRate: %w", err)
	}

	if _, err := mrClient.SetAudioSamplingRate(ctx, &recpb.SetAudioSamplingRateRequest{
		Handle: recorder,
		Arg0:   micAudioSamplingRate,
	}); err != nil {
		return fmt.Errorf("SetAudioSamplingRate: %w", err)
	}

	// Use setOutputFile(String) with the output path.
	if _, err := mrClient.SetOutputFile1_2(ctx, &recpb.SetOutputFile1_2Request{
		Handle: recorder,
		Arg0:   outputPath,
	}); err != nil {
		return fmt.Errorf("SetOutputFile: %w", err)
	}

	return nil
}

// getCacheDirPath resolves the app's cache directory via JNI.
func getCacheDirPath(ctx context.Context, j *jniCaller, appCtx int64) (string, error) {
	contextCls, err := j.findClass(ctx, "android/content/Context")
	if err != nil {
		return "", fmt.Errorf("finding Context class: %w", err)
	}

	getCacheDirMid, err := j.getMethodID(ctx, contextCls, "getCacheDir", "()Ljava/io/File;")
	if err != nil {
		return "", fmt.Errorf("getting getCacheDir method: %w", err)
	}

	cacheDir, err := j.callObjectMethod(ctx, appCtx, getCacheDirMid)
	if err != nil {
		return "", fmt.Errorf("calling getCacheDir: %w", err)
	}

	fileCls, err := j.findClass(ctx, "java/io/File")
	if err != nil {
		return "", fmt.Errorf("finding File class: %w", err)
	}

	getAbsolutePathMid, err := j.getMethodID(ctx, fileCls, "getAbsolutePath", "()Ljava/lang/String;")
	if err != nil {
		return "", fmt.Errorf("getting getAbsolutePath method: %w", err)
	}

	pathHandle, err := j.callObjectMethod(ctx, cacheDir, getAbsolutePathMid)
	if err != nil {
		return "", fmt.Errorf("calling getAbsolutePath: %w", err)
	}

	cachePath, err := j.getStringUTFChars(ctx, pathHandle)
	if err != nil {
		return "", fmt.Errorf("reading cache dir path: %w", err)
	}

	return cachePath, nil
}
