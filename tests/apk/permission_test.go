package apk_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPermissionDecisionState prevents duplicate submission and replay after Activity/process recreation.
//
// Example: an approved ACL stays committed while its separate runtime-permission callback is pending.
func TestPermissionDecisionState(t *testing.T) {
	dir := t.TempDir()
	source := `package center.dx.jni.jniservice;
public class PermissionDecisionTest {
  public static void main(String[] args) {
    PermissionDecision decision = new PermissionDecision();
    if (!decision.begin(true)) throw new AssertionError("first approval rejected");
    if (decision.begin(false)) throw new AssertionError("duplicate decision admitted");
    decision.complete(0);
    if (decision.outcome != PermissionDecision.Outcome.APPROVED) throw new AssertionError("approval not saved");
    if (decision.begin(true)) throw new AssertionError("saved approval replayed");
    decision.permissionRequested = true;
    PermissionDecision restored = PermissionDecision.restore(decision.outcome.name(), true, false);
    if (restored.begin(false) || !restored.permissionRequested) throw new AssertionError("runtime flow lost saved ACL");
    PermissionDecision interrupted = PermissionDecision.restore("SAVING", false, false);
    if (interrupted.begin(true) || interrupted.outcome != PermissionDecision.Outcome.UNCERTAIN) throw new AssertionError("unconfirmed decision replayed");
    PermissionDecision failed = new PermissionDecision();
    failed.begin(false); failed.complete(3);
    if (!failed.begin(false)) throw new AssertionError("failed decision cannot retry");
    failed.complete(0);
    if (failed.outcome != PermissionDecision.Outcome.DENIED || failed.begin(true)) throw new AssertionError("denial not final");
    PermissionDecision unavailable = new PermissionDecision();
    unavailable.begin(true); unavailable.complete(1);
    if (unavailable.outcome != PermissionDecision.Outcome.UNAVAILABLE) throw new AssertionError("readiness hidden");
    PermissionDecision stale = new PermissionDecision();
    stale.begin(true); stale.complete(2);
    if (stale.begin(true)) throw new AssertionError("no-row decision retried");
  }
}`
	testFile := filepath.Join(dir, "PermissionDecisionTest.java")
	require.NoError(t, os.WriteFile(testFile, []byte(source), 0600))
	output, err := exec.Command("javac", "-d", dir, "../../cmd/jniservice/apk/src/center/dx/jni/jniservice/PermissionDecision.java", testFile).CombinedOutput()
	require.NoError(t, err, "%s", output)
	output, err = exec.Command("java", "-cp", dir, "center.dx.jni.jniservice.PermissionDecisionTest").CombinedOutput()
	require.NoError(t, err, "%s", output)
}
