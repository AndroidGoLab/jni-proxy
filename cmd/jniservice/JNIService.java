// JNIService loads the jniservice shared library and starts the gRPC server.
//
// Run via app_process on an Android device:
//
//   LD_LIBRARY_PATH=/path/to/dir app_process \
//       -Djava.class.path=/path/to/dir/jniservice.dex /path/to/dir JNIService
//
// Configuration is via environment variables (set before invoking app_process):
//
//   JNISERVICE_PORT     — TCP port (default 50051)
//   JNISERVICE_LISTEN   — listen address (default 127.0.0.1)
//   JNISERVICE_DATA_DIR — writable directory for CA, ACL db, etc.
public class JNIService {
    public static void main(String[] args) {
        System.err.println("jniservice: loading shared library");
        try {
            System.loadLibrary("jniservice");
        } catch (Throwable t) {
            System.err.println("jniservice: load failed: " + t);
            System.exit(1);
        }
        System.err.println("jniservice: server started, waiting...");
        // Keep the JVM alive so the Go goroutine serving gRPC can run.
        // JNI_OnLoad starts the server in a goroutine and returns.
        synchronized (JNIService.class) {
            try {
                JNIService.class.wait();
            } catch (InterruptedException e) {
                // Exit on interrupt.
            }
        }
    }
}
