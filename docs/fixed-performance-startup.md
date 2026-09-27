# Phone-local fixed-performance startup

The APK can run `cmd power set-fixed-performance-mode-enabled true` once after
the first unlock following boot, when the phone joins trusted Wi-Fi and wireless
debugging is available. It uses a dedicated, already paired ADB key to connect
directly to the phone's own adbd over TLS. The computer is needed for initial
setup only. No extra Android app, root access, or host daemon is required.

The action is disabled by default. The existing APK CA and ACL database stay in
credential-protected storage, alongside the atomic mode-0600 startup configuration.
Locked boot is deliberately deferred until `BOOT_COMPLETED` after unlock.

## Prerequisites and one-time setup

Enable Developer options and wireless debugging. Trust the usual Wi-Fi network
using **Always allow on this network**. On devices supporting ADB Wi-Fi 2.0,
wireless debugging becomes available again when a trusted network connects.
Older firmware may need a manual wireless-debugging enable step after reboot;
this feature cannot remove that platform prerequisite. See the
[Android wireless-debugging guide](https://developer.android.com/tools/adb#connect-to-a-device-over-wi-fi).

Create and pair a dedicated RSA key with the SDK's `adb keygen` and `adb pair`.
Pairing uses the SDK process's Android user directory, so merely setting
`ADB_VENDOR_KEYS` does not select the pairing key. Use an isolated SDK profile;
keep the ordinary ADB server and any other computer's pairings intact.

For example, on Linux with Bubblewrap, create a private directory containing
`adbkey` and `adbkey.pub`, and expose that directory as `.android` only inside the
pairing process's mount namespace. Choose an unused server port. Every SDK
command for this profile must use that port and namespace:

```sh
mkdir -m 700 startup-adb-identity
adb keygen startup-adb-identity/adbkey
chmod 600 startup-adb-identity/adbkey
bwrap --bind / / --bind "$PWD/startup-adb-identity" "$HOME/.android" \
  adb -P 5048 --one-device jni-proxy-no-usb pair PHONE_IP:PAIRING_PORT
```

Enter the temporary pairing code from the phone's **Pair device with pairing
code** screen. Stop that isolated server after pairing with the same namespace
and `adb -P 5048 kill-server`. The pairing port is not the TLS connection port;
the APK discovers the changing connection port itself. Retain the dedicated
private key securely for configuration or recovery.

Install the APK and open it once. Enroll a jnicli client through the existing
`auth register` flow and approve `/startup.StartupService/*` through the phone's
permission dialog. Configure the existing service listener with this phone's
current Wi-Fi IPv4 address and port using `JNISERVICE_LISTEN` and
`JNISERVICE_PORT` as described in the [listener configuration](../README.md#running-jniservice-on-android).
The host CLI dials that same direct-LAN address and verifies the stable
`jniservice` TLS authority:

```sh
jnicli --addr PHONE_IPV4:50051 --authority jniservice --cert client.crt --key client.key --ca ca.crt \
  startup configure --adb-key-file startup-adb-identity/adbkey --enabled
jnicli --addr PHONE_IPV4:50051 --authority jniservice --cert client.crt --key client.key --ca ca.crt \
  startup status
```

The configure response acknowledges configuration. Wait for `state: succeeded`
and `lastExitCode: 0` in status to verify the actual command. Key contents are
never returned in status.

## Operation and status

The APK subscribes to `_adb-tls-connect._tcp` service updates, checks that a
resolved address belongs to a local interface, and connects only to literal
`127.0.0.1` at that service's current port. Discovery and network changes renew
the endpoint; a saved port is never trusted. Transient socket failures have at
most three attempts per endpoint, after which discovery stays subscribed for a
new service or network event. Revoked pairing is reported separately.

`startup apply` requests another single asynchronous attempt without changing
the startup opt-in. `startup disable` cancels pending discovery and I/O; it does
not execute the inverse power-mode command. `startup enable` persists enablement
and begins a new attempt. Both reuse the stored key. Reconfiguration invalidates
old callbacks and waits for cancelled I/O to finish before another command runs.

Success stops automatic retries and discovery. Duplicate boot/service delivery
in the same process does not repeat the action. A fresh service process may run
it again. There is no continuous power-policy enforcement or clock monitoring.
Status describes the last execution in the current app process, including its
generation, attempts, timestamp, optional exit code and error. A successful exit
means the fixed command reported success; it does not guarantee future frequency
or thermal behavior, or measure whether fixed-performance mode remains enabled.

## Testing without rebooting

Android documents a package-targeted simulated boot broadcast:

```sh
adb -s PHONE_SERIAL shell am broadcast \
  -a android.intent.action.BOOT_COMPLETED center.dx.jni.jniservice
```

To test persisted startup, first terminate only this app's process, clear its
stopped-package state if `am force-stop` was used, then deliver the broadcast.
Do not launch an activity or send `startup apply` between termination and the
broadcast. Confirm a new process, the receiver log, own-device NSD resolution,
and a new command timestamp/exit-zero result. A duplicate broadcast should keep
that result unchanged. Verify disable/re-enable separately.

This exercises the real receiver and phone-local execution without a hardware
reboot. It does not prove hardware reboot, first-unlock timing, or Wi-Fi recovery
on a different firmware. See Android's
[boot-service restrictions and test command](https://developer.android.com/about/versions/15/behavior-changes-15#fgs-boot-completed).
