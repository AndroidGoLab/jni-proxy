package center.dx.jni.jniservice;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.NetworkRequest;
import android.net.nsd.NsdManager;
import android.net.nsd.NsdServiceInfo;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;

import java.net.InetAddress;
import java.net.NetworkInterface;
import java.net.SocketException;
import java.util.ArrayDeque;
import java.util.HashMap;
import java.util.Map;

/** Observes the phone's own current adbd service; native code dials only loopback. */
final class LocalAdbDiscovery {
    private static final String TYPE = "_adb-tls-connect._tcp.";
    private static final String TAG = "jniservice-startup";
    private static final Handler MAIN = new Handler(Looper.getMainLooper());
    private static LocalAdbDiscovery instance;

    private final NsdManager nsd;
    private final ConnectivityManager connectivity;
    private final Map<String, Service> services = new HashMap<>();
    private final ArrayDeque<Service> resolveQueue = new ArrayDeque<>();
    private NsdManager.DiscoveryListener listener;
    private ConnectivityManager.NetworkCallback networkCallback;
    private long generation;
    private long revision;
    private boolean enabled;
    private boolean resolving;
    private int currentPort;
    private int discoveryRetries;

    private static native void discovered(long generation, int port);
    private static native void discoveryFailed(long generation, int errorCode);

    private LocalAdbDiscovery(Context context) {
        nsd = context.getSystemService(NsdManager.class);
        connectivity = context.getSystemService(ConnectivityManager.class);
    }

    static void initialize(Context context) {
        if (instance == null) instance = new LocalAdbDiscovery(context);
    }

    /** JNI may call from a worker; all platform state is owned by the main looper. */
    public static void setEnabled(long generation, boolean enabled) {
        MAIN.post(() -> {
            if (instance == null) throw new IllegalStateException("ADB discovery is not initialized");
            instance.update(generation, enabled);
        });
    }

    private void update(long nextGeneration, boolean nextEnabled) {
        if (nextGeneration < generation) return;
        enabled = false;
        stopDiscovery();
        if (networkCallback != null) {
            connectivity.unregisterNetworkCallback(networkCallback);
            networkCallback = null;
        }
        generation = nextGeneration;
        enabled = nextEnabled;
        discoveryRetries = 0;
        Log.i(TAG, "discovery generation=" + generation + " enabled=" + enabled);
        if (!enabled) return;
        final long expected = generation;
        networkCallback = new ConnectivityManager.NetworkCallback() {
            @Override public void onAvailable(Network network) { networkChanged(expected); }
            @Override public void onLost(Network network) { networkChanged(expected); }
        };
        connectivity.registerNetworkCallback(new NetworkRequest.Builder()
                .addTransportType(NetworkCapabilities.TRANSPORT_WIFI).build(), networkCallback, MAIN);
        startDiscovery();
    }

    private void networkChanged(long expected) {
        if (!enabled || generation != expected) return;
        stopDiscovery();
        discovered(generation, 0);
        discoveryRetries = 0;
        startDiscovery();
    }

    private void startDiscovery() {
        final long expected = ++revision;
        listener = new NsdManager.DiscoveryListener() {
            @Override public void onDiscoveryStarted(String type) {}
            @Override public void onDiscoveryStopped(String type) {}
            @Override public void onStartDiscoveryFailed(String type, int code) {
                MAIN.post(() -> {
                    if (!current(expected)) return;
                    listener = null;
                    discoveryFailed(generation, code);
                    if (discoveryRetries++ < 2) {
                        MAIN.postDelayed(() -> { if (current(expected)) startDiscovery(); }, 1000);
                    }
                });
            }
            @Override public void onStopDiscoveryFailed(String type, int code) {
                Log.w(TAG, "stop discovery failed: " + code);
            }
            @Override public void onServiceFound(NsdServiceInfo info) {
                MAIN.post(() -> { if (current(expected)) found(info, expected); });
            }
            @Override public void onServiceLost(NsdServiceInfo info) {
                MAIN.post(() -> {
                    if (!current(expected)) return;
                    Service service = services.remove(serviceKey(info));
                    if (service != null) service.close();
                    refreshPort();
                });
            }
        };
        try {
            nsd.discoverServices(TYPE, NsdManager.PROTOCOL_DNS_SD, listener);
        } catch (IllegalArgumentException | IllegalStateException failure) {
            listener = null;
            discoveryFailed(generation, -1);
            Log.w(TAG, "cannot start ADB discovery", failure);
        }
    }

    private boolean current(long expected) {
        return enabled && revision == expected;
    }

    private static String serviceKey(NsdServiceInfo info) {
        // Network-scoped NSD keys are available only on Android 13 and later.
        String network = Build.VERSION.SDK_INT >= 33 ? String.valueOf(info.getNetwork()) : "";
        return info.getServiceName() + "@" + network;
    }

    private void found(NsdServiceInfo info, long expected) {
        String key = serviceKey(info);
        if (services.containsKey(key)) return;
        Service service = new Service(info, expected);
        services.put(key, service);
        if (Build.VERSION.SDK_INT >= 34) {
            service.subscribe();
        } else {
            // Older Android has one-shot resolution only; serialize requests.
            resolveQueue.add(service);
            resolveNext();
        }
    }

    private void resolveNext() {
        if (resolving || resolveQueue.isEmpty()) return;
        Service service = resolveQueue.remove();
        if (!service.active()) { resolveNext(); return; }
        resolving = true;
        nsd.resolveService(service.info, new NsdManager.ResolveListener() {
            @Override public void onResolveFailed(NsdServiceInfo info, int code) {
                MAIN.post(() -> {
                    if (!current(service.expected)) return;
                    resolving = false;
                    discoveryFailed(generation, code);
                    resolveNext();
                });
            }
            @Override public void onServiceResolved(NsdServiceInfo info) {
                MAIN.post(() -> {
                    if (!current(service.expected)) return;
                    resolving = false;
                    service.update(info);
                    resolveNext();
                });
            }
        });
    }

    private void stopDiscovery() {
        revision++;
        if (listener != null) {
            try { nsd.stopServiceDiscovery(listener); }
            catch (IllegalArgumentException stopped) { Log.d(TAG, "discovery already stopped", stopped); }
            listener = null;
        }
        for (Service service : services.values()) service.close();
        services.clear();
        resolveQueue.clear();
        resolving = false;
        currentPort = 0;
    }

    private void refreshPort() {
        int port = 0;
        for (Service service : services.values()) {
            if (service.port != 0) { port = service.port; break; }
        }
        if (port == currentPort) return;
        currentPort = port;
        Log.i(TAG, "own-device ADB endpoint generation=" + generation + " loopback=127.0.0.1 port=" + port);
        discovered(generation, port);
    }

    private static boolean local(InetAddress address) throws SocketException {
        return address != null && NetworkInterface.getByInetAddress(address) != null;
    }

    private final class Service {
        final NsdServiceInfo info;
        final long expected;
        NsdManager.ServiceInfoCallback callback;
        int port;

        Service(NsdServiceInfo info, long expected) { this.info = info; this.expected = expected; }

        boolean active() { return current(expected) && services.get(serviceKey(info)) == this; }

        void subscribe() {
            callback = new NsdManager.ServiceInfoCallback() {
                @Override public void onServiceInfoCallbackRegistrationFailed(int code) {
                    if (!active()) return;
                    callback = null;
                    discoveryFailed(generation, code);
                }
                @Override public void onServiceInfoCallbackUnregistered() {}
                @Override public void onServiceLost() {
                    if (!active()) return;
                    port = 0;
                    refreshPort();
                }
                @Override public void onServiceUpdated(NsdServiceInfo updated) { update(updated); }
            };
            nsd.registerServiceInfoCallback(info, MAIN::post, callback);
        }

        void update(NsdServiceInfo updated) {
            if (!active()) return;
            boolean own = false;
            try {
                if (Build.VERSION.SDK_INT >= 34) {
                    for (InetAddress host : updated.getHostAddresses()) {
                        if (local(host)) { own = true; break; }
                    }
                } else {
                    own = local(updated.getHost());
                }
            } catch (SocketException failure) {
                discoveryFailed(generation, -2);
                Log.w(TAG, "cannot inspect local interface addresses", failure);
            }
            port = own && updated.getPort() > 0 && updated.getPort() <= 65535 ? updated.getPort() : 0;
            refreshPort();
        }

        void close() {
            if (Build.VERSION.SDK_INT >= 34 && callback != null) {
                try { nsd.unregisterServiceInfoCallback(callback); }
                catch (IllegalArgumentException stopped) { Log.d(TAG, "service callback already stopped", stopped); }
                callback = null;
            }
        }
    }
}
