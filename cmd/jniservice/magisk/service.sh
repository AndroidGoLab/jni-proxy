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

# Keep the config file as data so Go applies the same defaults → file → env
# precedence without evaluating shell syntax.
CONFIG_FILE="${JNISERVICE_CONFIG:-$MODDIR/jniservice.env}"

# Prefer APK mode: the APK has proper foreground service types
# (camera|microphone|location) which grant process capabilities.
if pm list packages 2>/dev/null | grep -q "$PKG"; then
    echo "Starting jniservice via APK foreground service" >> "$LOG"

    # Keep durable listener settings separate from one-launch overrides.
    CONFIG_DIR="/data/data/$PKG/files/jniservice"
    APP_CONFIG_FILE="$CONFIG_DIR/jniservice.env"
    APP_LAUNCH_CONFIG_FILE="$CONFIG_DIR/jniservice.launch.env"

    fail_config_staging() {
        echo "jniservice: APK config staging failed: $1" >> "$LOG"
        exit 1
    }

    if ! mkdir -p "$CONFIG_DIR"; then
        fail_config_staging "create $CONFIG_DIR"
    fi
    APP_UID=$(stat -c %u "/data/data/$PKG") ||
        fail_config_staging "resolve UID for /data/data/$PKG"
    if [ -z "$APP_UID" ]; then
        fail_config_staging "resolve UID for /data/data/$PKG"
    fi
    if [ -f "$CONFIG_FILE" ] && [ "$CONFIG_FILE" != "$APP_CONFIG_FILE" ]; then
        if ! cp "$CONFIG_FILE" "$APP_CONFIG_FILE"; then
            fail_config_staging "copy $CONFIG_FILE to $APP_CONFIG_FILE"
        fi
        if ! chown "$APP_UID" "$APP_CONFIG_FILE"; then
            fail_config_staging "set owner on $APP_CONFIG_FILE"
        fi
    fi
    if ! chown "$APP_UID" "$CONFIG_DIR"; then
        fail_config_staging "set owner on $CONFIG_DIR"
    fi

    # Replace the staged file on every launch, even when it must be empty.
    CONFIG_TMP_FILE="$APP_LAUNCH_CONFIG_FILE.tmp"
    if ! {
        if [ -n "${JNISERVICE_LISTEN:-}" ]; then
            printf 'JNISERVICE_LISTEN=%s\n' "$JNISERVICE_LISTEN"
        fi
        if [ -n "${JNISERVICE_PORT:-}" ]; then
            printf 'JNISERVICE_PORT=%s\n' "$JNISERVICE_PORT"
        fi
    } | tee "$CONFIG_TMP_FILE" >/dev/null; then
        fail_config_staging "create or write $CONFIG_TMP_FILE"
    fi
    if ! chmod 600 "$CONFIG_TMP_FILE"; then
        fail_config_staging "set mode on $CONFIG_TMP_FILE"
    fi
    if ! chown "$APP_UID" "$CONFIG_TMP_FILE"; then
        fail_config_staging "set owner on $CONFIG_TMP_FILE"
    fi
    if ! mv "$CONFIG_TMP_FILE" "$APP_LAUNCH_CONFIG_FILE"; then
        fail_config_staging "replace $APP_LAUNCH_CONFIG_FILE"
    fi
    if ! {
        if [ -n "${JNISERVICE_LISTEN:-}" ]; then
            printf 'JNISERVICE_LISTEN=%s\n' "$JNISERVICE_LISTEN"
        fi
        if [ -n "${JNISERVICE_PORT:-}" ]; then
            printf 'JNISERVICE_PORT=%s\n' "$JNISERVICE_PORT"
        fi
    } | cmp -s - "$APP_LAUNCH_CONFIG_FILE"; then
        fail_config_staging "verify staged overrides in $APP_LAUNCH_CONFIG_FILE"
    fi

    # Start the APK's activity which launches the foreground service.
    am start -n "$PKG/.JNIServiceActivity" >> "$LOG" 2>&1

    echo "jniservice APK started" >> "$LOG"
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
    export JNISERVICE_CONFIG="$CONFIG_FILE"
    export JNISERVICE_DATA_DIR="${JNISERVICE_DATA_DIR:-$MODDIR/data}"
    export LD_LIBRARY_PATH="$JNISERVICE_DIR:$LD_LIBRARY_PATH"

    app_process \
        -Djava.class.path="$JNISERVICE_DIR/jniservice.dex" \
        "$JNISERVICE_DIR" JNIService \
        >> "$LOG" 2>&1 &

    echo "jniservice started (pid=$!)" >> "$LOG"
fi
