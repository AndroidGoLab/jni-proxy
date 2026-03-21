package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	appconsts "github.com/AndroidGoLab/jni/app/consts"
	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"google.golang.org/grpc"
)

const setupTeardownMargin = 60 * time.Second

// Android MediaRecorder / Camera2 constants used by the JNI calls below.
const (
	audioSourceMIC        = 1 // MediaRecorder.AudioSource.MIC
	videoSourceSurface    = 2 // MediaRecorder.VideoSource.SURFACE
	outputFormatMPEG4     = 2 // MediaRecorder.OutputFormat.MPEG_4
	videoEncoderH264      = 2 // MediaRecorder.VideoEncoder.H264
	audioEncoderAAC       = 3 // MediaRecorder.AudioEncoder.AAC
	templateRecord        = 3 // CameraDevice.TEMPLATE_RECORD
	defaultVideoBitRate   = 10_000_000
	defaultVideoFrameRate = 30
	defaultVideoWidth     = 1920
	defaultVideoHeight    = 1080
)

var cameraRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record video from the camera via Camera2 + MediaRecorder (raw JNI)",
	Long: `Records video using the Android Camera2 API and MediaRecorder,
driven entirely through raw JNI calls over gRPC Proxy callbacks.
The resulting MP4 is written to stdout or a file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		duration, _ := cmd.Flags().GetDuration("duration")
		if duration <= 0 {
			return fmt.Errorf("--duration must be positive")
		}
		output, _ := cmd.Flags().GetString("output")
		cameraIndex, _ := cmd.Flags().GetInt("index")
		width, _ := cmd.Flags().GetInt("width")
		height, _ := cmd.Flags().GetInt("height")

		// Use a generous timeout: recording duration + margin for setup/teardown.
		ctx, cancel := context.WithTimeout(cmd.Context(), duration+setupTeardownMargin)
		defer cancel()

		client := pb.NewJNIServiceClient(grpcConn)
		j := &jniCaller{client: client}

		data, err := recordVideo(ctx, j, client, grpcConn, cameraIndex, duration, width, height)
		if err != nil {
			return err
		}

		if output == "-" || output == "" {
			_, err := os.Stdout.Write(data)
			return err
		}
		if err := os.WriteFile(output, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", output, err)
		}
		fmt.Fprintf(os.Stderr, "Saved %d bytes to %s\n", len(data), output)
		return nil
	},
}

func init() {
	cameraRecordCmd.Flags().DurationP("duration", "d", 10*time.Second, "recording duration")
	cameraRecordCmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	cameraRecordCmd.Flags().Int("index", 0, "camera index (0=back, 1=front)")
	cameraRecordCmd.Flags().Int("width", defaultVideoWidth, "video width in pixels")
	cameraRecordCmd.Flags().Int("height", defaultVideoHeight, "video height in pixels")
	cameraCmd.AddCommand(cameraRecordCmd)
}

// recordVideo orchestrates the Camera2+MediaRecorder flow via raw JNI.
func recordVideo(
	ctx context.Context,
	j *jniCaller,
	client pb.JNIServiceClient,
	conn *grpc.ClientConn,
	cameraIndex int,
	duration time.Duration,
	width, height int,
) (_ []byte, _err error) {
	appContextHandle, err := j.getAppContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting app context: %w", err)
	}

	// Step 1: Create HandlerThread + Handler for camera callbacks.
	handlerThread, handler, err := createHandlerThread(ctx, j)
	if err != nil {
		return nil, fmt.Errorf("creating handler thread: %w", err)
	}

	// Step 2: Get CameraManager and camera ID.
	cameraID, cameraManager, err := getCameraID(ctx, j, appContextHandle, cameraIndex)
	if err != nil {
		return nil, fmt.Errorf("getting camera ID: %w", err)
	}

	// Step 3: Create CameraDevice.StateCallback proxy and open camera.
	stateCallbackCls, err := j.findClass(ctx, "android/hardware/camera2/CameraDevice$StateCallback")
	if err != nil {
		return nil, fmt.Errorf("finding CameraDevice.StateCallback: %w", err)
	}

	cameraStream, err := client.Proxy(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening camera proxy stream: %w", err)
	}
	defer func() {
		if closeErr := cameraStream.CloseSend(); closeErr != nil && _err == nil {
			_err = fmt.Errorf("closing camera proxy stream: %w", closeErr)
		}
	}()

	if err := cameraStream.Send(&pb.ProxyClientMessage{
		Msg: &pb.ProxyClientMessage_Create{
			Create: &pb.CreateProxyRequest{
				InterfaceClassHandles: []int64{stateCallbackCls},
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("sending CreateProxy for StateCallback: %w", err)
	}

	cameraProxyResp, err := cameraStream.Recv()
	if err != nil {
		return nil, fmt.Errorf("receiving CreateProxy response: %w", err)
	}
	stateCallbackHandle := cameraProxyResp.GetCreated().GetProxyHandle()
	if stateCallbackHandle == 0 {
		return nil, fmt.Errorf("got null proxy handle for StateCallback")
	}

	// Step 4: Open camera.
	cameraIDStr, err := j.newString(ctx, cameraID)
	if err != nil {
		return nil, fmt.Errorf("creating camera ID string: %w", err)
	}

	cameraMgrCls, err := j.findClass(ctx, "android/hardware/camera2/CameraManager")
	if err != nil {
		return nil, fmt.Errorf("finding CameraManager class: %w", err)
	}
	openCameraMid, err := j.getMethodID(ctx, 
		cameraMgrCls,
		"openCamera",
		"(Ljava/lang/String;Landroid/hardware/camera2/CameraDevice$StateCallback;Landroid/os/Handler;)V",
	)
	if err != nil {
		return nil, fmt.Errorf("getting openCamera method: %w", err)
	}

	if err := j.callVoidMethod(ctx, cameraManager, openCameraMid,
		objVal(cameraIDStr), objVal(stateCallbackHandle), objVal(handler),
	); err != nil {
		return nil, fmt.Errorf("calling openCamera: %w", err)
	}

	// Step 5: Wait for onOpened callback.
	cameraDevice, err := waitForCallback(cameraStream, "onOpened")
	if err != nil {
		return nil, fmt.Errorf("waiting for onOpened: %w", err)
	}

	// From here on, ensure camera is closed on error.
	defer func() {
		if closeErr := closeCameraDevice(ctx, j, cameraDevice); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: closing camera: %v\n", closeErr)
		}
		if closeErr := stopHandlerThread(ctx, j, handlerThread); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: stopping handler thread: %v\n", closeErr)
		}
	}()

	// Step 6: Configure MediaRecorder.
	outputPath, err := getOutputPath(ctx, j, appContextHandle)
	if err != nil {
		return nil, fmt.Errorf("getting output path: %w", err)
	}

	recorder, err := setupMediaRecorder(ctx, j, appContextHandle, outputPath, width, height)
	if err != nil {
		return nil, fmt.Errorf("setting up MediaRecorder: %w", err)
	}
	defer func() {
		if releaseErr := releaseMediaRecorder(ctx, j, recorder); releaseErr != nil {
			fmt.Fprintf(os.Stderr, "warning: releasing MediaRecorder: %v\n", releaseErr)
		}
	}()

	// Step 7: Get MediaRecorder surface.
	mediaRecorderCls, err := j.findClass(ctx, "android/media/MediaRecorder")
	if err != nil {
		return nil, fmt.Errorf("finding MediaRecorder class: %w", err)
	}
	getSurfaceMid, err := j.getMethodID(ctx, mediaRecorderCls, "getSurface", "()Landroid/view/Surface;")
	if err != nil {
		return nil, fmt.Errorf("getting getSurface method: %w", err)
	}
	recorderSurface, err := j.callObjectMethod(ctx, recorder, getSurfaceMid)
	if err != nil {
		return nil, fmt.Errorf("getting recorder surface: %w", err)
	}
	if recorderSurface == 0 {
		return nil, fmt.Errorf("MediaRecorder.getSurface() returned null")
	}

	// Step 8: Create capture session via proxy callback.
	sessionCallbackCls, err := j.findClass(ctx, "android/hardware/camera2/CameraCaptureSession$StateCallback")
	if err != nil {
		return nil, fmt.Errorf("finding CameraCaptureSession.StateCallback: %w", err)
	}

	sessionStream, err := client.Proxy(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening session proxy stream: %w", err)
	}
	defer func() {
		if closeErr := sessionStream.CloseSend(); closeErr != nil && _err == nil {
			_err = fmt.Errorf("closing session proxy stream: %w", closeErr)
		}
	}()

	if err := sessionStream.Send(&pb.ProxyClientMessage{
		Msg: &pb.ProxyClientMessage_Create{
			Create: &pb.CreateProxyRequest{
				InterfaceClassHandles: []int64{sessionCallbackCls},
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("sending CreateProxy for SessionCallback: %w", err)
	}

	sessionProxyResp, err := sessionStream.Recv()
	if err != nil {
		return nil, fmt.Errorf("receiving session CreateProxy response: %w", err)
	}
	sessionCallbackHandle := sessionProxyResp.GetCreated().GetProxyHandle()
	if sessionCallbackHandle == 0 {
		return nil, fmt.Errorf("got null proxy handle for SessionCallback")
	}

	// Build a single-element Surface list for createCaptureSession.
	surfaceCls, err := j.findClass(ctx, "android/view/Surface")
	if err != nil {
		return nil, fmt.Errorf("finding Surface class: %w", err)
	}
	surfaceArray, err := j.newObjectArray(ctx, 1, surfaceCls, 0)
	if err != nil {
		return nil, fmt.Errorf("creating Surface array: %w", err)
	}
	if err := j.setObjectArrayElement(ctx, surfaceArray, 0, recorderSurface); err != nil {
		return nil, fmt.Errorf("setting Surface array element: %w", err)
	}

	// Convert Surface[] to List via Arrays.asList().
	arraysCls, err := j.findClass(ctx, "java/util/Arrays")
	if err != nil {
		return nil, fmt.Errorf("finding Arrays class: %w", err)
	}
	asListMid, err := j.getStaticMethodID(ctx, arraysCls, "asList", "([Ljava/lang/Object;)Ljava/util/List;")
	if err != nil {
		return nil, fmt.Errorf("getting Arrays.asList method: %w", err)
	}
	surfaceList, err := j.callStaticMethod(ctx, arraysCls, asListMid, pb.JType_OBJECT, objVal(surfaceArray))
	if err != nil {
		return nil, fmt.Errorf("calling Arrays.asList: %w", err)
	}
	surfaceListHandle := surfaceList.GetL()

	// CameraDevice.createCaptureSession(List<Surface>, StateCallback, Handler)
	cameraDeviceCls, err := j.findClass(ctx, "android/hardware/camera2/CameraDevice")
	if err != nil {
		return nil, fmt.Errorf("finding CameraDevice class: %w", err)
	}
	createSessionMid, err := j.getMethodID(ctx, 
		cameraDeviceCls,
		"createCaptureSession",
		"(Ljava/util/List;Landroid/hardware/camera2/CameraCaptureSession$StateCallback;Landroid/os/Handler;)V",
	)
	if err != nil {
		return nil, fmt.Errorf("getting createCaptureSession method: %w", err)
	}

	if err := j.callVoidMethod(ctx, cameraDevice, createSessionMid,
		objVal(surfaceListHandle), objVal(sessionCallbackHandle), objVal(handler),
	); err != nil {
		return nil, fmt.Errorf("calling createCaptureSession: %w", err)
	}

	// Step 9: Wait for onConfigured callback.
	captureSession, err := waitForCallback(sessionStream, "onConfigured")
	if err != nil {
		return nil, fmt.Errorf("waiting for onConfigured: %w", err)
	}

	// Step 10: Create capture request and start repeating.
	createCaptureRequestMid, err := j.getMethodID(ctx, 
		cameraDeviceCls,
		"createCaptureRequest",
		"(I)Landroid/hardware/camera2/CaptureRequest$Builder;",
	)
	if err != nil {
		return nil, fmt.Errorf("getting createCaptureRequest method: %w", err)
	}
	requestBuilder, err := j.callObjectMethod(ctx, cameraDevice, createCaptureRequestMid, intVal(templateRecord))
	if err != nil {
		return nil, fmt.Errorf("calling createCaptureRequest: %w", err)
	}

	builderCls, err := j.findClass(ctx, "android/hardware/camera2/CaptureRequest$Builder")
	if err != nil {
		return nil, fmt.Errorf("finding CaptureRequest.Builder class: %w", err)
	}
	addTargetMid, err := j.getMethodID(ctx, builderCls, "addTarget", "(Landroid/view/Surface;)V")
	if err != nil {
		return nil, fmt.Errorf("getting addTarget method: %w", err)
	}
	if err := j.callVoidMethod(ctx, requestBuilder, addTargetMid, objVal(recorderSurface)); err != nil {
		return nil, fmt.Errorf("calling addTarget: %w", err)
	}

	buildMid, err := j.getMethodID(ctx, builderCls, "build", "()Landroid/hardware/camera2/CaptureRequest;")
	if err != nil {
		return nil, fmt.Errorf("getting build method: %w", err)
	}
	captureRequest, err := j.callObjectMethod(ctx, requestBuilder, buildMid)
	if err != nil {
		return nil, fmt.Errorf("calling build: %w", err)
	}

	sessionCls, err := j.findClass(ctx, "android/hardware/camera2/CameraCaptureSession")
	if err != nil {
		return nil, fmt.Errorf("finding CameraCaptureSession class: %w", err)
	}
	setRepeatingMid, err := j.getMethodID(ctx, 
		sessionCls,
		"setRepeatingRequest",
		"(Landroid/hardware/camera2/CaptureRequest;Landroid/hardware/camera2/CameraCaptureSession$CaptureCallback;Landroid/os/Handler;)I",
	)
	if err != nil {
		return nil, fmt.Errorf("getting setRepeatingRequest method: %w", err)
	}

	// null CaptureCallback, use our handler.
	if _, err := j.callIntMethod(ctx, captureSession, setRepeatingMid,
		objVal(captureRequest), objVal(0), objVal(handler),
	); err != nil {
		return nil, fmt.Errorf("calling setRepeatingRequest: %w", err)
	}

	// Step 11: Start MediaRecorder and record for the specified duration.
	startMid, err := j.getMethodID(ctx, mediaRecorderCls, "start", "()V")
	if err != nil {
		return nil, fmt.Errorf("getting MediaRecorder.start method: %w", err)
	}
	if err := j.callVoidMethod(ctx, recorder, startMid); err != nil {
		return nil, fmt.Errorf("calling MediaRecorder.start: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Recording for %v...\n", duration)
	select {
	case <-time.After(duration):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Step 12: Stop recording.
	stopRepeatingMid, err := j.getMethodID(ctx, sessionCls, "stopRepeating", "()V")
	if err != nil {
		return nil, fmt.Errorf("getting stopRepeating method: %w", err)
	}
	if err := j.callVoidMethod(ctx, captureSession, stopRepeatingMid); err != nil {
		return nil, fmt.Errorf("calling stopRepeating: %w", err)
	}

	stopMid, err := j.getMethodID(ctx, mediaRecorderCls, "stop", "()V")
	if err != nil {
		return nil, fmt.Errorf("getting MediaRecorder.stop method: %w", err)
	}
	if err := j.callVoidMethod(ctx, recorder, stopMid); err != nil {
		return nil, fmt.Errorf("calling MediaRecorder.stop: %w", err)
	}

	closeSessionMid, err := j.getMethodID(ctx, sessionCls, "close", "()V")
	if err != nil {
		return nil, fmt.Errorf("getting CameraCaptureSession.close method: %w", err)
	}
	if err := j.callVoidMethod(ctx, captureSession, closeSessionMid); err != nil {
		return nil, fmt.Errorf("closing capture session: %w", err)
	}

	// Step 13: Read recorded file back via JNI.
	fmt.Fprintf(os.Stderr, "Reading recorded file...\n")
	return readFileViaJNI(ctx, j, outputPath)
}

// createHandlerThread creates a HandlerThread, starts it, and returns
// both the HandlerThread handle and a Handler backed by its Looper.
func createHandlerThread(ctx context.Context, j *jniCaller) (handlerThread, handler int64, _ error) {
	htCls, err := j.findClass(ctx, "android/os/HandlerThread")
	if err != nil {
		return 0, 0, fmt.Errorf("finding HandlerThread class: %w", err)
	}
	htCtor, err := j.getMethodID(ctx, htCls, "<init>", "(Ljava/lang/String;)V")
	if err != nil {
		return 0, 0, fmt.Errorf("getting HandlerThread constructor: %w", err)
	}
	threadName, err := j.newString(ctx, "CameraRecord")
	if err != nil {
		return 0, 0, fmt.Errorf("creating thread name string: %w", err)
	}
	handlerThread, err = j.newObject(ctx, htCls, htCtor, objVal(threadName))
	if err != nil {
		return 0, 0, fmt.Errorf("creating HandlerThread: %w", err)
	}

	startMid, err := j.getMethodID(ctx, htCls, "start", "()V")
	if err != nil {
		return 0, 0, fmt.Errorf("getting HandlerThread.start method: %w", err)
	}
	if err := j.callVoidMethod(ctx, handlerThread, startMid); err != nil {
		return 0, 0, fmt.Errorf("starting HandlerThread: %w", err)
	}

	getLooperMid, err := j.getMethodID(ctx, htCls, "getLooper", "()Landroid/os/Looper;")
	if err != nil {
		return 0, 0, fmt.Errorf("getting getLooper method: %w", err)
	}
	looper, err := j.callObjectMethod(ctx, handlerThread, getLooperMid)
	if err != nil {
		return 0, 0, fmt.Errorf("getting looper: %w", err)
	}

	handlerCls, err := j.findClass(ctx, "android/os/Handler")
	if err != nil {
		return 0, 0, fmt.Errorf("finding Handler class: %w", err)
	}
	handlerCtor, err := j.getMethodID(ctx, handlerCls, "<init>", "(Landroid/os/Looper;)V")
	if err != nil {
		return 0, 0, fmt.Errorf("getting Handler constructor: %w", err)
	}
	handler, err = j.newObject(ctx, handlerCls, handlerCtor, objVal(looper))
	if err != nil {
		return 0, 0, fmt.Errorf("creating Handler: %w", err)
	}

	return handlerThread, handler, nil
}

// getCameraID retrieves the camera ID string at the given index from CameraManager.
func getCameraID(
	ctx context.Context,
	j *jniCaller,
	appContextHandle int64,
	index int,
) (string, int64, error) {
	contextCls, err := j.findClass(ctx, "android/content/Context")
	if err != nil {
		return "", 0, fmt.Errorf("finding Context class: %w", err)
	}
	getSystemServiceMid, err := j.getMethodID(ctx, 
		contextCls,
		"getSystemService",
		"(Ljava/lang/String;)Ljava/lang/Object;",
	)
	if err != nil {
		return "", 0, fmt.Errorf("getting getSystemService method: %w", err)
	}

	cameraServiceStr, err := j.newString(ctx, appconsts.CameraService)
	if err != nil {
		return "", 0, fmt.Errorf("creating camera service string: %w", err)
	}
	cameraManager, err := j.callObjectMethod(ctx, appContextHandle, getSystemServiceMid, objVal(cameraServiceStr))
	if err != nil {
		return "", 0, fmt.Errorf("calling getSystemService(%q): %w", appconsts.CameraService, err)
	}
	if cameraManager == 0 {
		return "", 0, fmt.Errorf("getSystemService(%q) returned null", appconsts.CameraService)
	}

	cameraMgrCls, err := j.findClass(ctx, "android/hardware/camera2/CameraManager")
	if err != nil {
		return "", 0, fmt.Errorf("finding CameraManager class: %w", err)
	}
	getCameraIdListMid, err := j.getMethodID(ctx, cameraMgrCls, "getCameraIdList", "()[Ljava/lang/String;")
	if err != nil {
		return "", 0, fmt.Errorf("getting getCameraIdList method: %w", err)
	}
	cameraIdArray, err := j.callObjectMethod(ctx, cameraManager, getCameraIdListMid)
	if err != nil {
		return "", 0, fmt.Errorf("calling getCameraIdList: %w", err)
	}
	if cameraIdArray == 0 {
		return "", 0, fmt.Errorf("getCameraIdList returned null")
	}

	cameraIdHandle, err := j.getObjectArrayElement(ctx, cameraIdArray, int32(index))
	if err != nil {
		return "", 0, fmt.Errorf("getting camera ID at index %d: %w", index, err)
	}
	cameraID, err := j.getStringUTFChars(ctx, cameraIdHandle)
	if err != nil {
		return "", 0, fmt.Errorf("reading camera ID string: %w", err)
	}

	return cameraID, cameraManager, nil
}

// videoRecordingFallbackDir is used when getCacheDir fails (e.g. when
// running as the "android" package which has no data directory).
const videoRecordingFallbackDir = "/data/local/tmp"

// getOutputPath returns a device-side path for the camera recording.
// It tries the app's cache directory first and falls back to
// videoRecordingFallbackDir when getCacheDir fails.
func getOutputPath(ctx context.Context, j *jniCaller, appContextHandle int64) (string, error) {
	cachePath, err := getCacheDirPath(ctx, j, appContextHandle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: getCacheDir failed (%v), falling back to %s\n", err, videoRecordingFallbackDir)
		return videoRecordingFallbackDir + "/camera_record.mp4", nil
	}
	return cachePath + "/camera_record.mp4", nil
}

// setupMediaRecorder creates and configures a MediaRecorder via JNI.
func setupMediaRecorder(
	ctx context.Context,
	j *jniCaller,
	appContextHandle int64,
	outputPath string,
	width int,
	height int,
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
		recorder, err = j.newObject(ctx, mrCls, ctor, objVal(appContextHandle))
	}
	if ctorErr != nil || err != nil {
		// Fallback: no-arg constructor for older APIs.
		ctor, err = j.getMethodID(ctx, mrCls, "<init>", "()V")
		if err != nil {
			return 0, fmt.Errorf("getting MediaRecorder constructor: %w", err)
		}
		recorder, err = j.newObject(ctx, mrCls, ctor)
		if err != nil {
			return 0, fmt.Errorf("creating MediaRecorder: %w", err)
		}
	}

	type methodCall struct {
		name string
		sig  string
		args []*pb.JValue
	}

	outputPathStr, err := j.newString(ctx, outputPath)
	if err != nil {
		return 0, fmt.Errorf("creating output path string: %w", err)
	}

	calls := []methodCall{
		{"setAudioSource", "(I)V", []*pb.JValue{intVal(audioSourceMIC)}},
		{"setVideoSource", "(I)V", []*pb.JValue{intVal(videoSourceSurface)}},
		{"setOutputFormat", "(I)V", []*pb.JValue{intVal(outputFormatMPEG4)}},
		{"setOutputFile", "(Ljava/lang/String;)V", []*pb.JValue{objVal(outputPathStr)}},
		{"setVideoEncodingBitRate", "(I)V", []*pb.JValue{intVal(defaultVideoBitRate)}},
		{"setVideoFrameRate", "(I)V", []*pb.JValue{intVal(defaultVideoFrameRate)}},
		{"setVideoSize", "(II)V", []*pb.JValue{intVal(int32(width)), intVal(int32(height))}},
		{"setVideoEncoder", "(I)V", []*pb.JValue{intVal(videoEncoderH264)}},
		{"setAudioEncoder", "(I)V", []*pb.JValue{intVal(audioEncoderAAC)}},
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

// waitForCallback blocks on the proxy stream until the named callback arrives.
// It responds to any callback that expects a response (to avoid blocking the
// Java side), and returns the first argument handle from the target callback.
func waitForCallback(
	stream grpc.BidiStreamingClient[pb.ProxyClientMessage, pb.ProxyServerMessage],
	targetMethod string,
) (int64, error) {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return 0, fmt.Errorf("receiving proxy message while waiting for %q: %w", targetMethod, err)
		}

		cb := msg.GetCallback()
		if cb == nil {
			continue
		}

		// Acknowledge callbacks that expect a response.
		if cb.GetExpectsResponse() {
			if sendErr := stream.Send(&pb.ProxyClientMessage{
				Msg: &pb.ProxyClientMessage_CallbackResponse{
					CallbackResponse: &pb.CallbackResponse{
						CallbackId: cb.GetCallbackId(),
					},
				},
			}); sendErr != nil {
				return 0, fmt.Errorf("sending callback response for %q: %w", cb.GetMethodName(), sendErr)
			}
		}

		if cb.GetMethodName() == targetMethod {
			if len(cb.GetArgHandles()) < 1 {
				return 0, fmt.Errorf("callback %q had no arguments", targetMethod)
			}
			// For onOpened(CameraDevice) and onConfigured(CameraCaptureSession),
			// the target object is the first argument.
			return cb.GetArgHandles()[0], nil
		}

		// Drain other callbacks (e.g., onDisconnected, onError) silently.
		fmt.Fprintf(os.Stderr, "proxy callback: %s (ignoring)\n", cb.GetMethodName())
	}
}

// readFileViaJNI reads a file on the Android device using FileInputStream and
// returns the content as a byte array via GetByteArrayData.
func readFileViaJNI(ctx context.Context, j *jniCaller, path string) ([]byte, error) {
	// new FileInputStream(path)
	fisCls, err := j.findClass(ctx, "java/io/FileInputStream")
	if err != nil {
		return nil, fmt.Errorf("finding FileInputStream: %w", err)
	}
	fisCtor, err := j.getMethodID(ctx, fisCls, "<init>", "(Ljava/lang/String;)V")
	if err != nil {
		return nil, fmt.Errorf("getting FileInputStream constructor: %w", err)
	}
	pathStr, err := j.newString(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("creating path string: %w", err)
	}
	fis, err := j.newObject(ctx, fisCls, fisCtor, objVal(pathStr))
	if err != nil {
		return nil, fmt.Errorf("creating FileInputStream: %w", err)
	}

	// fis.available() -> size
	availableMid, err := j.getMethodID(ctx, fisCls, "available", "()I")
	if err != nil {
		return nil, fmt.Errorf("getting available method: %w", err)
	}
	size, err := j.callIntMethod(ctx, fis, availableMid)
	if err != nil {
		return nil, fmt.Errorf("calling available: %w", err)
	}
	if size <= 0 {
		return nil, fmt.Errorf("file is empty or unavailable (size=%d)", size)
	}

	// byte[] buf = new byte[size]
	resp, err := j.client.NewPrimitiveArray(ctx, &pb.NewPrimitiveArrayRequest{
		ElementType: pb.JType_BYTE,
		Length:      size,
	})
	if err != nil {
		return nil, fmt.Errorf("creating byte array: %w", err)
	}
	buf := resp.GetArrayHandle()

	// fis.read(buf)
	readMid, err := j.getMethodID(ctx, fisCls, "read", "([B)I")
	if err != nil {
		return nil, fmt.Errorf("getting read method: %w", err)
	}
	if _, err := j.callIntMethod(ctx, fis, readMid, objVal(buf)); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// fis.close()
	closeMid, err := j.getMethodID(ctx, fisCls, "close", "()V")
	if err != nil {
		return nil, fmt.Errorf("getting close method: %w", err)
	}
	if err := j.callVoidMethod(ctx, fis, closeMid); err != nil {
		return nil, fmt.Errorf("closing FileInputStream: %w", err)
	}

	// Transfer byte array data over gRPC.
	return j.getByteArrayData(ctx, buf)
}

// closeCameraDevice calls CameraDevice.close().
func closeCameraDevice(ctx context.Context, j *jniCaller, cameraDevice int64) error {
	cls, err := j.findClass(ctx, "android/hardware/camera2/CameraDevice")
	if err != nil {
		return err
	}
	closeMid, err := j.getMethodID(ctx, cls, "close", "()V")
	if err != nil {
		return err
	}
	return j.callVoidMethod(ctx, cameraDevice, closeMid)
}

// stopHandlerThread calls HandlerThread.quitSafely().
func stopHandlerThread(ctx context.Context, j *jniCaller, handlerThread int64) error {
	cls, err := j.findClass(ctx, "android/os/HandlerThread")
	if err != nil {
		return err
	}
	quitMid, err := j.getMethodID(ctx, cls, "quitSafely", "()Z")
	if err != nil {
		return err
	}
	_, err = j.callMethod(ctx, handlerThread, quitMid, pb.JType_BOOLEAN)
	return err
}

// releaseMediaRecorder calls MediaRecorder.release().
func releaseMediaRecorder(ctx context.Context, j *jniCaller, recorder int64) error {
	cls, err := j.findClass(ctx, "android/media/MediaRecorder")
	if err != nil {
		return err
	}
	releaseMid, err := j.getMethodID(ctx, cls, "release", "()V")
	if err != nil {
		return err
	}
	return j.callVoidMethod(ctx, recorder, releaseMid)
}
