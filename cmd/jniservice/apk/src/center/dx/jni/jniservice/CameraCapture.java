package center.dx.jni.jniservice;

import android.content.Context;
import android.graphics.ImageFormat;
import android.hardware.camera2.CameraAccessException;
import android.hardware.camera2.CameraCaptureSession;
import android.hardware.camera2.CameraCharacteristics;
import android.hardware.camera2.CameraDevice;
import android.hardware.camera2.CameraManager;
import android.hardware.camera2.CaptureResult;
import android.hardware.camera2.CaptureRequest;
import android.hardware.camera2.TotalCaptureResult;
import android.media.Image;
import android.media.ImageReader;
import android.os.Handler;
import android.os.HandlerThread;
import android.util.Log;
import android.util.Size;
import android.view.Surface;

import java.nio.ByteBuffer;
import java.util.Arrays;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/**
 * Synchronous camera capture helper for headless (no-preview) use.
 * Call {@link #takePicture(Context, int)} from any thread — it blocks
 * until the JPEG bytes are available (or timeout).
 */
public class CameraCapture {
    private static final String TAG = "CameraCapture";
    private static final int TIMEOUT_SECONDS = 10;
    private static final int DEFAULT_AE_SETTLE_TIMEOUT_MILLIS = 3000;
    private static final int MAX_AE_SETTLE_TIMEOUT_MILLIS = 30000;

    /** Disable flash while retaining automatic exposure. */
    public static final int FLASH_OFF = 0;
    /** Let automatic exposure decide whether to fire the flash. */
    public static final int FLASH_AUTO = 1;
    /** Request the flash for the final still capture. */
    public static final int FLASH_ON = 2;

    /**
     * Takes a JPEG photo from the given camera (0 = back, 1 = front).
     * Blocks the calling thread until the image is captured.
     *
     * @return JPEG bytes, or null on failure.
     */
    public static byte[] takePicture(Context context, int cameraIndex) {
        return takePicture(context, cameraIndex, DEFAULT_AE_SETTLE_TIMEOUT_MILLIS, FLASH_OFF, 0, 0L);
    }

    /**
     * Takes a JPEG with explicit exposure and flash controls.
     *
     * @param context application context used to access Camera2
     * @param cameraIndex camera index (0 = back, 1 = front)
     * @param aeWarmupMillis maximum automatic-exposure warm-up time; zero skips warm-up
     * @param flashPolicy one of {@link #FLASH_OFF}, {@link #FLASH_AUTO}, or {@link #FLASH_ON}
     * @param manualIso sensor ISO, or zero to use automatic exposure
     * @param manualExposureTimeNs sensor exposure time in nanoseconds, or zero to use automatic exposure
     * @return JPEG bytes, or null on camera failure
     * @throws IllegalArgumentException if manual controls are incomplete or conflict with flash
     */
    public static byte[] takePicture(Context context, int cameraIndex, int aeWarmupMillis,
            int flashPolicy, int manualIso, long manualExposureTimeNs) {
        validateOptions(cameraIndex, aeWarmupMillis, flashPolicy, manualIso, manualExposureTimeNs);

        CameraManager manager = (CameraManager) context.getSystemService(Context.CAMERA_SERVICE);
        if (manager == null) {
            Log.e(TAG, "CameraManager not available");
            return null;
        }

        HandlerThread handlerThread = new HandlerThread("CameraCapture");
        handlerThread.start();
        Handler handler = new Handler(handlerThread.getLooper());

        try {
            String[] cameraIds = manager.getCameraIdList();
            if (cameraIndex < 0 || cameraIndex >= cameraIds.length) {
                Log.e(TAG, "Camera index " + cameraIndex + " out of range (have " + cameraIds.length + ")");
                return null;
            }
            String cameraId = cameraIds[cameraIndex];

            CameraCharacteristics chars = manager.getCameraCharacteristics(cameraId);
            Size[] jpegSizes = chars.get(CameraCharacteristics.SCALER_STREAM_CONFIGURATION_MAP)
                    .getOutputSizes(ImageFormat.JPEG);
            // Use the largest available JPEG size.
            Size size = jpegSizes[0];
            for (Size s : jpegSizes) {
                if (s.getWidth() * s.getHeight() > size.getWidth() * size.getHeight()) {
                    size = s;
                }
            }

            ImageReader imageReader = ImageReader.newInstance(size.getWidth(), size.getHeight(), ImageFormat.JPEG, 1);
            final byte[][] result = {null};
            CountDownLatch imageLatch = new CountDownLatch(1);
            // Repeating requests use the same JPEG surface to let the camera's AE
            // algorithm settle. Only the image belonging to the final still request
            // is retained; warm-up images are drained and discarded.
            final long[] finalCaptureTimestamp = {Long.MAX_VALUE};

            imageReader.setOnImageAvailableListener(reader -> {
                Image image = reader.acquireLatestImage();
                if (image != null) {
                    try {
                        if (image.getTimestamp() >= finalCaptureTimestamp[0]) {
                            ByteBuffer buffer = image.getPlanes()[0].getBuffer();
                            result[0] = new byte[buffer.remaining()];
                            buffer.get(result[0]);
                            imageLatch.countDown();
                        }
                    } finally {
                        image.close();
                    }
                }
            }, handler);

            // Open camera synchronously.
            CountDownLatch openLatch = new CountDownLatch(1);
            final CameraDevice[] deviceHolder = {null};

            manager.openCamera(cameraId, new CameraDevice.StateCallback() {
                @Override
                public void onOpened(CameraDevice camera) {
                    deviceHolder[0] = camera;
                    openLatch.countDown();
                }
                @Override
                public void onDisconnected(CameraDevice camera) {
                    camera.close();
                    openLatch.countDown();
                }
                @Override
                public void onError(CameraDevice camera, int error) {
                    Log.e(TAG, "Camera open error: " + error);
                    camera.close();
                    openLatch.countDown();
                }
            }, handler);

            if (!openLatch.await(TIMEOUT_SECONDS, TimeUnit.SECONDS) || deviceHolder[0] == null) {
                Log.e(TAG, "Camera open timed out or failed");
                imageReader.close();
                return null;
            }

            CameraDevice camera = deviceHolder[0];

            // Create capture session.
            CountDownLatch sessionLatch = new CountDownLatch(1);
            final CameraCaptureSession[] sessionHolder = {null};

            camera.createCaptureSession(Arrays.asList(imageReader.getSurface()),
                    new CameraCaptureSession.StateCallback() {
                        @Override
                        public void onConfigured(CameraCaptureSession session) {
                            sessionHolder[0] = session;
                            sessionLatch.countDown();
                        }
                        @Override
                        public void onConfigureFailed(CameraCaptureSession session) {
                            Log.e(TAG, "Session configure failed");
                            sessionLatch.countDown();
                        }
                    }, handler);

            if (!sessionLatch.await(TIMEOUT_SECONDS, TimeUnit.SECONDS) || sessionHolder[0] == null) {
                Log.e(TAG, "Session creation timed out or failed");
                camera.close();
                imageReader.close();
                return null;
            }

            boolean manualExposure = manualIso > 0;
            int aeMode = aeModeForFlashPolicy(flashPolicy);

            if (!manualExposure && aeWarmupMillis > 0) {
                // A headless session has no preview repeating request, so submitting a
                // still request immediately after configuration can use stale/default
                // exposure values on some devices (notably Pixel camera HALs).
                CaptureRequest.Builder warmupBuilder = camera.createCaptureRequest(CameraDevice.TEMPLATE_PREVIEW);
                warmupBuilder.addTarget(imageReader.getSurface());
                warmupBuilder.set(CaptureRequest.CONTROL_MODE, CaptureRequest.CONTROL_MODE_AUTO);
                warmupBuilder.set(CaptureRequest.CONTROL_AE_MODE, aeMode);

                CountDownLatch aeLatch = new CountDownLatch(1);
                sessionHolder[0].setRepeatingRequest(warmupBuilder.build(), new CameraCaptureSession.CaptureCallback() {
                    @Override
                    public void onCaptureCompleted(CameraCaptureSession session, CaptureRequest request,
                            TotalCaptureResult r) {
                        Integer aeState = r.get(CaptureResult.CONTROL_AE_STATE);
                        if (aeState != null && (aeState == CaptureResult.CONTROL_AE_STATE_CONVERGED
                                || aeState == CaptureResult.CONTROL_AE_STATE_FLASH_REQUIRED)) {
                            aeLatch.countDown();
                        }
                    }
                }, handler);
                if (!aeLatch.await(aeWarmupMillis, TimeUnit.MILLISECONDS)) {
                    Log.w(TAG, "Auto-exposure did not report convergence before timeout; capturing anyway");
                }
                sessionHolder[0].stopRepeating();
            }

            // Capture the final still after AE has converged. The image callback
            // filters out any warm-up frame that was already in flight.
            CaptureRequest.Builder captureBuilder = camera.createCaptureRequest(CameraDevice.TEMPLATE_STILL_CAPTURE);
            captureBuilder.addTarget(imageReader.getSurface());
            captureBuilder.set(CaptureRequest.CONTROL_MODE, CaptureRequest.CONTROL_MODE_AUTO);
            if (manualExposure) {
                captureBuilder.set(CaptureRequest.CONTROL_AE_MODE, CaptureRequest.CONTROL_AE_MODE_OFF);
                captureBuilder.set(CaptureRequest.SENSOR_SENSITIVITY, manualIso);
                captureBuilder.set(CaptureRequest.SENSOR_EXPOSURE_TIME, manualExposureTimeNs);
            } else {
                captureBuilder.set(CaptureRequest.CONTROL_AE_MODE, aeMode);
            }
            captureBuilder.set(CaptureRequest.JPEG_QUALITY, (byte) 90);

            sessionHolder[0].capture(captureBuilder.build(), new CameraCaptureSession.CaptureCallback() {
                @Override
                public void onCaptureStarted(CameraCaptureSession session, CaptureRequest request, long timestamp,
                        long frameNumber) {
                    finalCaptureTimestamp[0] = timestamp;
                }

                @Override
                public void onCaptureCompleted(CameraCaptureSession session, CaptureRequest request, TotalCaptureResult r) {
                    Log.d(TAG, "Capture completed: ae=" + r.get(CaptureResult.CONTROL_AE_STATE)
                            + " exposureNs=" + r.get(CaptureResult.SENSOR_EXPOSURE_TIME)
                            + " iso=" + r.get(CaptureResult.SENSOR_SENSITIVITY));
                }
            }, handler);

            // Wait for image.
            if (!imageLatch.await(TIMEOUT_SECONDS, TimeUnit.SECONDS)) {
                Log.e(TAG, "Image capture timed out");
            }

            // Cleanup.
            sessionHolder[0].close();
            camera.close();
            imageReader.close();

            return result[0];

        } catch (CameraAccessException | InterruptedException e) {
            Log.e(TAG, "Camera capture failed", e);
            return null;
        } finally {
            handlerThread.quitSafely();
        }
    }

    /** Validates capture controls before opening the camera. */
    static void validateOptions(int cameraIndex, int aeWarmupMillis, int flashPolicy,
            int manualIso, long manualExposureTimeNs) {
        if (cameraIndex < 0) {
            throw new IllegalArgumentException("camera index must not be negative");
        }
        if (aeWarmupMillis < 0 || aeWarmupMillis > MAX_AE_SETTLE_TIMEOUT_MILLIS) {
            throw new IllegalArgumentException("AE warm-up timeout must be between 0 and "
                    + MAX_AE_SETTLE_TIMEOUT_MILLIS + " ms");
        }
        if (flashPolicy < FLASH_OFF || flashPolicy > FLASH_ON) {
            throw new IllegalArgumentException("invalid flash policy: " + flashPolicy);
        }
        if (manualIso < 0 || manualExposureTimeNs < 0) {
            throw new IllegalArgumentException("manual ISO and exposure time must not be negative");
        }
        boolean hasManualIso = manualIso > 0;
        boolean hasManualExposure = manualExposureTimeNs > 0;
        if (hasManualIso != hasManualExposure) {
            throw new IllegalArgumentException("manual ISO and exposure time must be supplied together");
        }
        if (hasManualIso && flashPolicy != FLASH_OFF) {
            throw new IllegalArgumentException("manual ISO/exposure require flash off because manual exposure disables AE");
        }
    }

    /** Maps the public flash policy to the Camera2 automatic-exposure mode. */
    private static int aeModeForFlashPolicy(int flashPolicy) {
        switch (flashPolicy) {
            case FLASH_OFF:
                return CaptureRequest.CONTROL_AE_MODE_ON;
            case FLASH_AUTO:
                return CaptureRequest.CONTROL_AE_MODE_ON_AUTO_FLASH;
            case FLASH_ON:
                return CaptureRequest.CONTROL_AE_MODE_ON_ALWAYS_FLASH;
            default:
                throw new IllegalArgumentException("invalid flash policy: " + flashPolicy);
        }
    }
}
