package center.dx.jni.jniservice;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.Manifest;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.IBinder;
import android.os.UserManager;
import android.location.LocationManager;
import android.system.Os;

public class JNIServiceForeground extends Service {
    private static final String CHANNEL_ID = "jniservice";
    private static final int NOTIFICATION_ID = 1;
    private static boolean loaded = false;
    static final String EXTRA_FROM_ACTIVITY = "from_activity";

    // Native method to pass the APK's Application context to Go.
    // Called after System.loadLibrary so the Go side can store a handle
    // to the real app context (with the correct ClassLoader and permissions).
    private static native boolean setAppContext(Object context);
    private static native void startStartup();

    @Override
    public void onCreate() {
        super.onCreate();
        createNotificationChannel();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (!getSystemService(UserManager.class).isUserUnlocked()) {
            stopSelf();
            return START_NOT_STICKY;
        }
        Notification notification = new Notification.Builder(this, CHANNEL_ID)
                .setContentTitle("jniservice")
                .setContentText("gRPC server running on port 50051")
                .setSmallIcon(android.R.drawable.ic_menu_manage)
                .setOngoing(true)
                .build();

        // Boot/sticky launches use the base mask. An activity-origin request
        // may add optional types, but Android still checks actual eligibility.
        boolean fromActivity = intent != null
                && intent.getBooleanExtra(EXTRA_FROM_ACTIVITY, false);
        int types = ForegroundServiceTypes.select(Build.VERSION.SDK_INT, fromActivity,
                granted(Manifest.permission.CAMERA),
                granted(Manifest.permission.RECORD_AUDIO),
                granted(Manifest.permission.ACCESS_FINE_LOCATION)
                    || granted(Manifest.permission.ACCESS_COARSE_LOCATION), locationEnabled());
        if (Build.VERSION.SDK_INT >= 29) {
            int base = ForegroundServiceTypes.select(Build.VERSION.SDK_INT, false,
                    false, false, false, false);
            // Establish the base service first so optional promotion failure
            // does not prevent the server and startup action from running.
            startForeground(NOTIFICATION_ID, notification, base);
            if (types != base) {
                try {
                    startForeground(NOTIFICATION_ID, notification, types);
                } catch (SecurityException | IllegalStateException rejected) {
                    android.util.Log.w("jniservice", "optional foreground types unavailable; using base service", rejected);
                }
            }
        } else {
            startForeground(NOTIFICATION_ID, notification);
        }

        if (!loaded) {
            try {
                // Preserve the existing credential-protected CA, ACL and config.
                Os.setenv("JNISERVICE_DATA_DIR",
                        new java.io.File(getFilesDir(), "jniservice").getPath(), true);
                System.loadLibrary("jniservice");
                android.util.Log.i("jniservice", "native library loaded, gRPC server started");

                // Pass the real app context to the Go side. getApplicationContext()
                // returns the Application with the APK's PathClassLoader and permissions.
                if (!setAppContext(getApplicationContext())) {
                    throw new IllegalStateException("native application context initialization failed");
                }
                LocalAdbDiscovery.initialize(getApplicationContext());
                loaded = true;
                android.util.Log.i("jniservice", "app context passed to native side");
            } catch (Throwable t) {
                android.util.Log.e("jniservice", "failed to load native library", t);
                stopSelf();
                return START_NOT_STICKY;
            }
        }

        // The native controller ignores duplicate starts in this process.
        startStartup();

        return START_STICKY;
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private boolean granted(String permission) {
        return checkSelfPermission(permission) == PackageManager.PERMISSION_GRANTED;
    }

    private boolean locationEnabled() {
        LocationManager manager = getSystemService(LocationManager.class);
        if (Build.VERSION.SDK_INT >= 28) return manager.isLocationEnabled();
        return manager.isProviderEnabled(LocationManager.GPS_PROVIDER)
                || manager.isProviderEnabled(LocationManager.NETWORK_PROVIDER);
    }

    private void createNotificationChannel() {
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID, "jniservice", NotificationManager.IMPORTANCE_LOW);
        channel.setDescription("jniservice gRPC server status");
        getSystemService(NotificationManager.class).createNotificationChannel(channel);
    }
}
