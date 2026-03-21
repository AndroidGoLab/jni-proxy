package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
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
	Short: "Record audio from the microphone via MediaRecorder (raw JNI)",
	Long: `Records audio using the Android MediaRecorder API,
driven entirely through raw JNI calls over gRPC.
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

		data, err := recordAudio(ctx, j, duration)
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

// recordAudio orchestrates audio-only recording via MediaRecorder over raw JNI.
func recordAudio(
	ctx context.Context,
	j *jniCaller,
	duration time.Duration,
) ([]byte, error) {
	appCtx, err := j.getAppContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting app context: %w", err)
	}

	// Resolve output path on the device.
	outputPath, err := getAudioOutputPath(ctx, j, appCtx)
	if err != nil {
		return nil, fmt.Errorf("getting output path: %w", err)
	}

	// Create, configure, and prepare the MediaRecorder.
	recorder, err := setupAudioRecorder(ctx, j, appCtx, outputPath)
	if err != nil {
		return nil, fmt.Errorf("setting up MediaRecorder: %w", err)
	}
	defer func() {
		if releaseErr := releaseMediaRecorder(ctx, j, recorder); releaseErr != nil {
			fmt.Fprintf(os.Stderr, "warning: releasing MediaRecorder: %v\n", releaseErr)
		}
	}()

	// Start recording.
	mrCls, err := j.findClass(ctx, "android/media/MediaRecorder")
	if err != nil {
		return nil, fmt.Errorf("finding MediaRecorder class: %w", err)
	}

	startMid, err := j.getMethodID(ctx, mrCls, "start", "()V")
	if err != nil {
		return nil, fmt.Errorf("getting MediaRecorder.start method: %w", err)
	}
	if err := j.callVoidMethod(ctx, recorder, startMid); err != nil {
		return nil, fmt.Errorf("calling MediaRecorder.start: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Recording audio for %v...\n", duration)
	select {
	case <-time.After(duration):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Stop recording.
	stopMid, err := j.getMethodID(ctx, mrCls, "stop", "()V")
	if err != nil {
		return nil, fmt.Errorf("getting MediaRecorder.stop method: %w", err)
	}
	if err := j.callVoidMethod(ctx, recorder, stopMid); err != nil {
		return nil, fmt.Errorf("calling MediaRecorder.stop: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Recording stopped.\n")

	// Read the recorded file back via JNI.
	fmt.Fprintf(os.Stderr, "Reading recorded file...\n")
	return readFileViaJNI(ctx, j, outputPath)
}

// audioRecordingFallbackDir is used when getCacheDir fails (e.g. when
// running as the "android" package which has no data directory).
const audioRecordingFallbackDir = "/data/local/tmp"

// getAudioOutputPath returns a device-side path for the audio recording.
// It tries the app's cache directory first and falls back to
// audioRecordingFallbackDir when getCacheDir fails.
func getAudioOutputPath(ctx context.Context, j *jniCaller, appCtx int64) (string, error) {
	cachePath, err := getCacheDirPath(ctx, j, appCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: getCacheDir failed (%v), falling back to %s\n", err, audioRecordingFallbackDir)
		return audioRecordingFallbackDir + "/recording.m4a", nil
	}
	return cachePath + "/recording.m4a", nil
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

// setupAudioRecorder creates and configures a MediaRecorder for audio-only recording.
func setupAudioRecorder(
	ctx context.Context,
	j *jniCaller,
	appCtx int64,
	outputPath string,
) (int64, error) {
	mrCls, err := j.findClass(ctx, "android/media/MediaRecorder")
	if err != nil {
		return 0, fmt.Errorf("finding MediaRecorder class: %w", err)
	}

	// MediaRecorder(Context) constructor (API 31+).
	// Fall back to no-arg constructor for older APIs.
	var recorder int64
	ctor, ctorErr := j.getMethodID(ctx, mrCls, "<init>", "(Landroid/content/Context;)V")
	if ctorErr == nil {
		recorder, err = j.newObject(ctx, mrCls, ctor, objVal(appCtx))
	}
	if ctorErr != nil || err != nil {
		ctor, err = j.getMethodID(ctx, mrCls, "<init>", "()V")
		if err != nil {
			return 0, fmt.Errorf("getting MediaRecorder constructor: %w", err)
		}
		recorder, err = j.newObject(ctx, mrCls, ctor)
		if err != nil {
			return 0, fmt.Errorf("creating MediaRecorder: %w", err)
		}
	}

	outputPathStr, err := j.newString(ctx, outputPath)
	if err != nil {
		return 0, fmt.Errorf("creating output path string: %w", err)
	}

	type methodCall struct {
		name string
		sig  string
		args []*pb.JValue
	}

	calls := []methodCall{
		{"setAudioSource", "(I)V", []*pb.JValue{intVal(micAudioSourceMIC)}},
		{"setOutputFormat", "(I)V", []*pb.JValue{intVal(micOutputFormatMPEG4)}},
		{"setAudioEncoder", "(I)V", []*pb.JValue{intVal(micAudioEncoderAAC)}},
		{"setAudioEncodingBitRate", "(I)V", []*pb.JValue{intVal(micAudioEncodingBitRate)}},
		{"setAudioSamplingRate", "(I)V", []*pb.JValue{intVal(micAudioSamplingRate)}},
		{"setOutputFile", "(Ljava/lang/String;)V", []*pb.JValue{objVal(outputPathStr)}},
		{"prepare", "()V", nil},
	}

	for _, c := range calls {
		mid, err := j.getMethodID(ctx, mrCls, c.name, c.sig)
		if err != nil {
			return 0, fmt.Errorf("getting MediaRecorder.%s method: %w", c.name, err)
		}
		if err := j.callVoidMethod(ctx, recorder, mid, c.args...); err != nil {
			return 0, fmt.Errorf("calling MediaRecorder.%s: %w", c.name, err)
		}
	}

	return recorder, nil
}
