package apk_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestForegroundServiceTypes exercises Android launch policy on the host JVM.
//
// Example: a boot launch never requests a while-in-use permission type.
func TestForegroundServiceTypes(t *testing.T) {
	dir := t.TempDir()
	sdk := os.Getenv("ANDROID_HOME")
	if sdk == "" {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		sdk = filepath.Join(homeDir, "Android", "Sdk")
	}
	jar := filepath.Join(sdk, "platforms", "android-36", "android.jar")
	_, err := os.Stat(jar)
	require.NoError(t, err)
	source := `package center.dx.jni.jniservice;
import android.content.pm.ServiceInfo;
public class ForegroundServiceTypesTest {
  public static void main(String[] args) {
    int special = ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE;
    int camera = ServiceInfo.FOREGROUND_SERVICE_TYPE_CAMERA;
    int microphone = ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE;
    int location = ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION;
    check(special, ForegroundServiceTypes.select(37, false, true, true, true, true));
    check(special, ForegroundServiceTypes.select(37, true, false, false, false, true));
    check(special, ForegroundServiceTypes.select(37, true, false, false, true, false));
    check(special | camera, ForegroundServiceTypes.select(37, true, true, false, false, true));
    check(special | camera | microphone | location,
          ForegroundServiceTypes.select(37, true, true, true, true, true));
    check(0, ForegroundServiceTypes.select(30, false, true, true, true, true));
    check(camera | microphone | location, ForegroundServiceTypes.select(30, true, true, true, true, true));
    check(location, ForegroundServiceTypes.select(29, true, true, true, true, true));
    check(0, ForegroundServiceTypes.select(26, true, true, true, true, true));
  }
  private static void check(int expected, int actual) {
    if (expected != actual) throw new AssertionError(expected + " != " + actual);
  }
}`
	testFile := filepath.Join(dir, "ForegroundServiceTypesTest.java")
	require.NoError(t, os.WriteFile(testFile, []byte(source), 0600))
	policy := "../../cmd/jniservice/apk/src/center/dx/jni/jniservice/ForegroundServiceTypes.java"
	output, err := exec.Command("javac", "-cp", jar, "-d", dir, policy, testFile).CombinedOutput()
	require.NoError(t, err, "%s", output)
	output, err = exec.Command("java", "-cp", dir, "center.dx.jni.jniservice.ForegroundServiceTypesTest").CombinedOutput()
	require.NoError(t, err, "%s", output)
}
