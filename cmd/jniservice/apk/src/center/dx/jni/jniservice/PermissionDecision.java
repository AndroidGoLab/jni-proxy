package center.dx.jni.jniservice;

/** Tracks saved ACL state separately from Android runtime-permission UI. Main-thread owned. */
final class PermissionDecision {
    enum Outcome { READY, SAVING, APPROVED, DENIED, ERROR, UNAVAILABLE, ALREADY_DECIDED, UNCERTAIN }
    Outcome outcome = Outcome.READY;
    boolean approve;
    boolean permissionRequested;
    boolean permissionFinished;

    boolean begin(boolean approve) {
        if (outcome != Outcome.READY && outcome != Outcome.ERROR && outcome != Outcome.UNAVAILABLE) return false;
        this.approve = approve;
        outcome = Outcome.SAVING;
        return true;
    }

    /** JNI result codes match the local Go bridge; Intent extras never supply grant authority. */
    void complete(int result) {
        switch (result) {
            case 0: outcome = approve ? Outcome.APPROVED : Outcome.DENIED; break;
            case 1: outcome = Outcome.UNAVAILABLE; break;
            case 2: outcome = Outcome.ALREADY_DECIDED; break;
            default: outcome = Outcome.ERROR;
        }
    }

    /** A process death during saving has an unknown result, so never replay that decision. */
    static PermissionDecision restore(String outcome, boolean requested, boolean finished) {
        PermissionDecision state = new PermissionDecision();
        state.outcome = Outcome.valueOf(outcome);
        if (state.outcome == Outcome.SAVING) state.outcome = Outcome.UNCERTAIN;
        state.permissionRequested = requested;
        state.permissionFinished = finished;
        return state;
    }
}
