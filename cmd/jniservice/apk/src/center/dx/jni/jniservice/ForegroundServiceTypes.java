package center.dx.jni.jniservice;

import android.content.pm.ServiceInfo;

/** Selects service types permitted by the launch origin and granted permissions. */
final class ForegroundServiceTypes {
    private ForegroundServiceTypes() {}

    /** Boot and sticky restarts conservatively use the base type only. */
    static int select(int sdk, boolean fromActivity, boolean camera,
                      boolean microphone, boolean location, boolean locationEnabled) {
        int types = sdk >= 34 ? ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE : 0;
        if (!fromActivity) return types;
        if (sdk >= 29 && location && locationEnabled) types |= ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION;
        if (sdk >= 30 && camera) types |= ServiceInfo.FOREGROUND_SERVICE_TYPE_CAMERA;
        if (sdk >= 30 && microphone) types |= ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE;
        return types;
    }
}
