#!/system/bin/sh
# jniservice auto-start via Magisk.
# Prefers APK mode (foreground service with proper capabilities),
# falls back to app_process if the APK is not installed.

MODDIR="${0%/*}"
JNISERVICE_DIR="$MODDIR/jniservice"
LOG="$MODDIR/jniservice.log"
PKG="center.dx.jni.jniservice"

# Wait for boot to fully complete.
while [ "$(getprop sys.boot_completed)" != "1" ]; do
    sleep 1
done
sleep 5

# Read config from module directory if it exists.
PORT="${JNISERVICE_PORT:-50051}"
LISTEN="${JNISERVICE_LISTEN:-127.0.0.1}"
if [ -f "$MODDIR/jniservice.env" ]; then
    . "$MODDIR/jniservice.env"
fi

# Prefer APK mode: the APK has proper foreground service types
# (camera|microphone|location) which grant process capabilities.
if pm list packages 2>/dev/null | grep -q "$PKG"; then
    echo "Starting jniservice via APK foreground service" >> "$LOG"

    # Write config for APK to read (Go native lib picks up env file).
    CONFIG_DIR="/data/data/$PKG/files/jniservice"
    mkdir -p "$CONFIG_DIR"
    echo "JNISERVICE_PORT=$PORT" > "$CONFIG_DIR/jniservice.env"
    echo "JNISERVICE_LISTEN=$LISTEN" >> "$CONFIG_DIR/jniservice.env"
    chown -R $(stat -c %u /data/data/$PKG) "$CONFIG_DIR/"

    # Start the APK's activity which launches the foreground service.
    am start -n "$PKG/.JNIServiceActivity" >> "$LOG" 2>&1

    echo "jniservice APK started (port=$PORT)" >> "$LOG"
else
    echo "APK not installed; starting jniservice via app_process" >> "$LOG"

    # Kill any existing instance.
    pkill -f "app_process.*JNIService" 2>/dev/null
    sleep 1

    # Start jniservice as root. The server auto-detects root mode and
    # sets the Binder identity to system_server (uid 1000) so all
    # Android API calls (camera, location, microphone) pass
    # CallerIdentity checks. When running as non-root (e.g., via adb
    # shell), the server uses com.android.shell package context instead.
    export JNISERVICE_PORT="$PORT"
    export JNISERVICE_LISTEN="$LISTEN"
    export JNISERVICE_DATA_DIR="$MODDIR/data"
    export LD_LIBRARY_PATH="$JNISERVICE_DIR:$LD_LIBRARY_PATH"

    app_process \
        -Djava.class.path="$JNISERVICE_DIR/jniservice.dex" \
        "$JNISERVICE_DIR" JNIService \
        >> "$LOG" 2>&1 &

    echo "jniservice started (pid=$!, port=$PORT)" >> "$LOG"
fi
