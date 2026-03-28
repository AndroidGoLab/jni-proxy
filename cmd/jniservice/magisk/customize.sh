#!/system/bin/sh
# Magisk module install script.
# MODPATH is set by Magisk to the module's install directory.

ui_print "Installing jniservice..."
ui_print "  Config: <module_dir>/jniservice.env"
ui_print "  Logs:   <module_dir>/jniservice.log"
ui_print "  Data:   <module_dir>/data/"

# Set permissions on service.sh.
set_perm "$MODPATH/service.sh" 0 0 0755
set_perm_recursive "$MODPATH/jniservice" 0 0 0755 0644

# ---- Install the companion APK ----
PKG="center.dx.jni.jniservice"
APK="$MODPATH/jniservice.apk"

if [ -f "$APK" ]; then
    ui_print "  Installing companion APK..."
    pm install -r "$APK" 2>&1 | while IFS= read -r line; do
        ui_print "    $line"
    done
    if pm path "$PKG" >/dev/null 2>&1; then
        ui_print "  APK installed successfully"

        # Grant runtime permissions so the service can use camera, mic, etc.
        # without user interaction (Magisk runs as root so pm grant works).
        ui_print "  Granting runtime permissions..."
        for perm in \
            android.permission.CAMERA \
            android.permission.RECORD_AUDIO \
            android.permission.ACCESS_FINE_LOCATION \
            android.permission.ACCESS_COARSE_LOCATION \
            android.permission.ACCESS_BACKGROUND_LOCATION \
            android.permission.READ_PHONE_STATE \
            android.permission.CALL_PHONE \
            android.permission.BODY_SENSORS \
            android.permission.BLUETOOTH_CONNECT \
            android.permission.BLUETOOTH_SCAN \
            android.permission.BLUETOOTH_ADVERTISE \
            android.permission.POST_NOTIFICATIONS \
        ; do
            pm grant "$PKG" "$perm" 2>/dev/null
        done
        ui_print "  gRPC server will start via APK foreground service on next boot"
    else
        ui_print "  WARNING: APK install failed; falling back to app_process on boot"
    fi
else
    ui_print "  No APK bundled; gRPC server will start via app_process on next boot"
fi
