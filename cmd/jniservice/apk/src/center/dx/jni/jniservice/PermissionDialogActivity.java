package center.dx.jni.jniservice;

import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import java.lang.ref.WeakReference;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Displays request metadata and sends local decisions to the already-open Go ACL Store. */
public class PermissionDialogActivity extends Activity {
    private static final int PERMISSION_REQUEST = 100;
    private static final Handler MAIN = new Handler(Looper.getMainLooper());
    private static final ExecutorService DECISIONS = Executors.newSingleThreadExecutor();
    private Operation operation;
    private TextView status;
    private Button approveButton;
    private Button denyButton;
    private Button startServiceButton;
    private boolean resumed;

    private static native int decideRequest(long requestId, boolean approve);

    /** Retained across rotation; the asynchronous task never holds an Activity or Context strongly. */
    private static final class Operation {
        final long requestId;
        PermissionDecision state = new PermissionDecision();
        WeakReference<PermissionDialogActivity> observer = new WeakReference<>(null);
        Operation(long requestId) { this.requestId = requestId; }
    }

    @Override protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        long requestId = getIntent().getLongExtra("request_id", -1);
        String clientId = getIntent().getStringExtra("client_id");
        String methods = getIntent().getStringExtra("methods");
        if (requestId <= 0 || clientId == null || methods == null) { finish(); return; }

        operation = (Operation) getLastNonConfigurationInstance();
        if (operation == null || operation.requestId != requestId) {
            operation = new Operation(requestId);
            if (savedInstanceState != null) {
                operation.state = PermissionDecision.restore(savedInstanceState.getString("outcome", "READY"),
                        savedInstanceState.getBoolean("permission_requested"),
                        savedInstanceState.getBoolean("permission_finished"));
            }
        }
        operation.observer = new WeakReference<>(this);

        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        int padding = dp(20);
        layout.setPadding(padding, padding, padding, padding);
        TextView title = new TextView(this);
        title.setText("Permission request");
        title.setTextSize(20);
        title.setGravity(Gravity.CENTER);
        layout.addView(title);
        TextView body = new TextView(this);
        // These extras are display-only. The native Store resolves grant authority from requestId.
        body.setText("Client: " + clientId + "\n\nRequested methods:\n" + methods.replace(",", "\n"));
        body.setTextSize(14);
        body.setPadding(0, dp(16), 0, dp(16));
        layout.addView(body);
        status = new TextView(this);
        status.setAccessibilityLiveRegion(TextView.ACCESSIBILITY_LIVE_REGION_POLITE);
        status.setPadding(0, 0, 0, dp(12));
        layout.addView(status);

        LinearLayout choices = new LinearLayout(this);
        choices.setOrientation(LinearLayout.HORIZONTAL);
        approveButton = new Button(this);
        approveButton.setText("Approve");
        approveButton.setOnClickListener(view -> submit(true));
        denyButton = new Button(this);
        denyButton.setText("Deny");
        denyButton.setOnClickListener(view -> submit(false));
        choices.addView(approveButton, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1));
        choices.addView(denyButton, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1));
        layout.addView(choices);
        startServiceButton = new Button(this);
        startServiceButton.setText("Open jniservice");
        startServiceButton.setOnClickListener(view -> startActivity(new Intent(this, JNIServiceActivity.class)));
        layout.addView(startServiceButton);
        Button close = new Button(this);
        close.setText("Close");
        close.setOnClickListener(view -> finish());
        layout.addView(close);
        ScrollView scroll = new ScrollView(this);
        scroll.addView(layout);
        setContentView(scroll);
        render();
    }

    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }

    @Override public Object onRetainNonConfigurationInstance() { return operation; }

    @Override protected void onSaveInstanceState(Bundle out) {
        super.onSaveInstanceState(out);
        if (operation == null) return;
        out.putString("outcome", operation.state.outcome.name());
        out.putBoolean("permission_requested", operation.state.permissionRequested);
        out.putBoolean("permission_finished", operation.state.permissionFinished);
    }

    @Override protected void onResume() {
        super.onResume();
        resumed = true;
        if (operation != null) { render(); requestRuntimePermissions(); }
    }

    @Override protected void onPause() { resumed = false; super.onPause(); }

    @Override protected void onDestroy() {
        if (operation != null && operation.observer.get() == this) operation.observer.clear();
        super.onDestroy();
    }

    private void submit(boolean approve) {
        if (!operation.state.begin(approve)) return;
        render();
        final Operation pending = operation;
        DECISIONS.execute(() -> {
            int result;
            try { result = decideRequest(pending.requestId, approve); }
            catch (UnsatisfiedLinkError unavailable) { result = 1; }
            final int decisionResult = result;
            MAIN.post(() -> {
                pending.state.complete(decisionResult);
                PermissionDialogActivity activity = pending.observer.get();
                if (activity == null || activity.isDestroyed() || activity.isFinishing()) return;
                activity.render();
                activity.requestRuntimePermissions();
            });
        });
    }

    private void render() {
        PermissionDecision state = operation.state;
        boolean ready = state.outcome == PermissionDecision.Outcome.READY
                || state.outcome == PermissionDecision.Outcome.ERROR
                || state.outcome == PermissionDecision.Outcome.UNAVAILABLE;
        approveButton.setEnabled(ready);
        denyButton.setEnabled(ready);
        startServiceButton.setVisibility(state.outcome == PermissionDecision.Outcome.UNAVAILABLE ? Button.VISIBLE : Button.GONE);
        switch (state.outcome) {
            case READY: status.setText("Choose which access to allow."); break;
            case SAVING: status.setText("Saving access… Closing this dialog will not cancel the decision."); break;
            case APPROVED:
                status.setText(state.permissionRequested && !state.permissionFinished
                        ? "Access approved and saved. Waiting for the Android permission response."
                        : "Access approved and saved. Android permissions remain controlled by the phone.");
                break;
            case DENIED: status.setText("Request denied. Existing access is unchanged."); break;
            case UNAVAILABLE: status.setText("The native service is unavailable. Open jniservice, return here, and retry."); break;
            case ALREADY_DECIDED: status.setText("This request is no longer pending. Check the client's current permissions, or close and request access again."); break;
            case UNCERTAIN: status.setText("The app restarted while saving. Check the client's permissions before requesting access again; this decision will not be repeated."); break;
            case ERROR: status.setText("Access could not be saved. Retry, or close and request access again."); break;
        }
    }

    /** ACL approval is already committed; requesting platform permissions is a separate operation. */
    private void requestRuntimePermissions() {
        PermissionDecision state = operation.state;
        if (!resumed || state.outcome != PermissionDecision.Outcome.APPROVED || state.permissionRequested) return;
        List<String> needed = new ArrayList<>();
        String[] dangerous = {android.Manifest.permission.CAMERA, android.Manifest.permission.RECORD_AUDIO,
                android.Manifest.permission.ACCESS_FINE_LOCATION, android.Manifest.permission.ACCESS_COARSE_LOCATION};
        for (String permission : dangerous) {
            if (checkSelfPermission(permission) != PackageManager.PERMISSION_GRANTED) needed.add(permission);
        }
        state.permissionRequested = true;
        if (needed.isEmpty()) { state.permissionFinished = true; render(); return; }
        render();
        requestPermissions(needed.toArray(new String[0]), PERMISSION_REQUEST);
    }

    @Override public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode != PERMISSION_REQUEST || operation == null) return;
        operation.state.permissionFinished = true;
        render();
    }
}
