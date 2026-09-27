package apk_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCameraCaptureOptions exercises the service-side capture-control invariant on a host JVM.
//
// Example: manual ISO and exposure are accepted only with flash disabled.
func TestCameraCaptureOptions(t *testing.T) {
	dir := t.TempDir()
	sdk := os.Getenv("ANDROID_HOME")
	if sdk == "" {
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		sdk = filepath.Join(homeDir, "Android", "Sdk")
	}
	jar := filepath.Join(sdk, "platforms", "android-35", "android.jar")
	if _, err := os.Stat(jar); os.IsNotExist(err) {
		jar = filepath.Join(sdk, "platforms", "android-36", "android.jar")
	}
	require.FileExists(t, jar)

	source := `package center.dx.jni.jniservice;
public class CameraCaptureOptionsTest {
  public static void main(String[] args) {
    expectOK(0, 3000, CameraCapture.FLASH_OFF, 0, 0);
    expectOK(0, 0, CameraCapture.FLASH_OFF, 400, 10000000L);
    expectFailure(0, 3000, CameraCapture.FLASH_OFF, 400, 0, "supplied together");
    expectFailure(0, 3000, CameraCapture.FLASH_AUTO, 400, 10000000L, "require flash off");
    expectFailure(0, -1, CameraCapture.FLASH_OFF, 0, 0, "between 0");
  }
  private static void expectOK(int index, int warmup, int flash, int iso, long exposure) {
    CameraCapture.validateOptions(index, warmup, flash, iso, exposure);
  }
  private static void expectFailure(int index, int warmup, int flash, int iso, long exposure, String text) {
    try {
      CameraCapture.validateOptions(index, warmup, flash, iso, exposure);
      throw new AssertionError("expected validation failure");
    } catch (IllegalArgumentException e) {
      if (!e.getMessage().contains(text)) throw new AssertionError(e.getMessage());
    }
  }
}`
	testFile := filepath.Join(dir, "CameraCaptureOptionsTest.java")
	require.NoError(t, os.WriteFile(testFile, []byte(source), 0600))
	javaSource := "../../cmd/jniservice/apk/src/center/dx/jni/jniservice/CameraCapture.java"
	output, err := exec.Command("javac", "-cp", jar, "-d", dir, javaSource, testFile).CombinedOutput()
	require.NoError(t, err, "%s", output)
	output, err = exec.Command("java", "-cp", dir+string(os.PathListSeparator)+jar,
		"center.dx.jni.jniservice.CameraCaptureOptionsTest").CombinedOutput()
	require.NoError(t, err, "%s", output)
}
