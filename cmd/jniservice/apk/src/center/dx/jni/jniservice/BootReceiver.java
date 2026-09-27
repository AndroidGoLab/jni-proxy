package center.dx.jni.jniservice;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.os.UserManager;

public class BootReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        String action = intent.getAction();
        // The CA, ACL and startup key remain in credential-protected storage.
        // BOOT_COMPLETED arrives after first unlock; locked boot is deferred.
        if (Intent.ACTION_BOOT_COMPLETED.equals(action)
                && context.getSystemService(UserManager.class).isUserUnlocked()) {
            android.util.Log.i("jniservice", "after-unlock boot receiver starting service");
            Intent serviceIntent = new Intent(context, JNIServiceForeground.class);
            context.startForegroundService(serviceIntent);
        }
    }
}
