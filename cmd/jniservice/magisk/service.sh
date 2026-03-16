#!/system/bin/sh
# jniservice auto-start via Magisk.
# Runs after boot, starts the gRPC server via app_process.

MODDIR="${0%/*}"
JNISERVICE_DIR="$MODDIR/jniservice"
LOG="$MODDIR/jniservice.log"

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

# Kill any existing instance.
pkill -f "app_process.*JNIService" 2>/dev/null
sleep 1

# Start jniservice.
export JNISERVICE_PORT="$PORT"
export JNISERVICE_LISTEN="$LISTEN"
export JNISERVICE_DATA_DIR="$MODDIR/data"
export LD_LIBRARY_PATH="$JNISERVICE_DIR:$LD_LIBRARY_PATH"

app_process \
    -Djava.class.path="$JNISERVICE_DIR/jniservice.dex" \
    "$JNISERVICE_DIR" JNIService \
    >> "$LOG" 2>&1 &

echo "jniservice started (pid=$!, port=$PORT)" >> "$LOG"
