//go:build android

package main

/*
#include <jni.h>
*/
import "C"

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/AndroidGoLab/jni"
	"github.com/AndroidGoLab/jni-proxy/adb"
	startupserver "github.com/AndroidGoLab/jni-proxy/grpc/server/startup"
	startuppb "github.com/AndroidGoLab/jni-proxy/proto/startup"
	"github.com/AndroidGoLab/jni-proxy/startup"
	"google.golang.org/grpc"
)

// globalStartup is created during JNI_OnLoad and used after application-context initialization.
//
// Example: NSD callbacks and authenticated RPCs share the same synchronized controller.
var globalStartup *startup.Controller

// registerStartup installs the manual feature service on the existing mTLS/ACL server.
//
// Example: the private config remains beside the existing credential-protected CA and ACL.
func registerStartup(
	server *grpc.Server,
	dataDir string,
) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	globalStartup = startup.New(filepath.Join(dataDir, "startup.json"), startup.ADBRunner{}, androidADBDiscovery{})
	startuppb.RegisterStartupServiceServer(server, &startupserver.Server{Controller: globalStartup})
}

// androidADBDiscovery posts subscription changes through the APK's application ClassLoader.
//
// Example: Go RPC worker threads can request Android discovery without owning Java lifecycle state.
type androidADBDiscovery struct{}

// Update resolves the APK discovery entry point and posts its generation and desired state.
//
// Example: disabling posts a generation change that invalidates already queued callbacks.
func (androidADBDiscovery) Update(
	ctx context.Context,
	generation uint64,
	enabled bool,
) error {
	if globalJNIServer == nil || globalJNIServer.AppClassLoader == 0 {
		return fmt.Errorf("APK ClassLoader is not initialized")
	}
	return globalVM.Do(
		// Local JNI references are confined to this attached native thread.
		func(env *jni.Env) error {
			loader := globalHandles.Get(globalJNIServer.AppClassLoader)
			if loader == nil {
				return fmt.Errorf("APK ClassLoader handle unavailable")
			}
			loaderClass, err := env.FindClass("java/lang/ClassLoader")
			if err != nil {
				return err
			}
			method, err := env.GetMethodID(loaderClass, "loadClass", "(Ljava/lang/String;)Ljava/lang/Class;")
			if err != nil {
				return err
			}
			name, err := env.NewStringUTF("center.dx.jni.jniservice.LocalAdbDiscovery")
			if err != nil {
				return err
			}
			classObject, err := env.CallObjectMethod(loader, method, jni.ObjectValue(&name.Object))
			if err != nil {
				return err
			}
			if classObject == nil {
				return fmt.Errorf("ADB discovery class unavailable")
			}
			class := &jni.Class{Object: *classObject}
			setEnabled, err := env.GetStaticMethodID(class, "setEnabled", "(JZ)V")
			if err != nil {
				return err
			}
			var value uint8
			if enabled {
				value = 1
			}
			if err := env.CallStaticVoidMethod(class, setEnabled, jni.LongValue(int64(generation)), jni.BooleanValue(value)); err != nil {
				return err
			}
			slog.DebugContext(ctx, "posted Android ADB discovery state", "generation", generation, "enabled", enabled)
			return nil
		})
}

// Java_center_dx_jni_jniservice_JNIServiceForeground_startStartup initializes the once-per-process action.
//
// Example: a package-targeted BOOT_COMPLETED starts this path without an Apply RPC.
//
//export Java_center_dx_jni_jniservice_JNIServiceForeground_startStartup
func Java_center_dx_jni_jniservice_JNIServiceForeground_startStartup(
	env *C.JNIEnv,
	class C.jclass,
) {
	if globalStartup == nil {
		return
	}
	if err := globalStartup.Start(context.Background()); err != nil {
		slog.Error("startup initialization failed", "error", err)
	}
}

// Java_center_dx_jni_jniservice_LocalAdbDiscovery_discovered receives an already verified own-device port.
//
// Example: port zero cancels an attempt after the advertised local service disappears.
//
//export Java_center_dx_jni_jniservice_LocalAdbDiscovery_discovered
func Java_center_dx_jni_jniservice_LocalAdbDiscovery_discovered(
	env *C.JNIEnv,
	class C.jclass,
	generation C.jlong,
	port C.jint,
) {
	if globalStartup != nil {
		globalStartup.Discovered(context.Background(), uint64(generation), adb.Port(port))
	}
}

// Java_center_dx_jni_jniservice_LocalAdbDiscovery_discoveryFailed exposes platform discovery failures.
//
// Example: status explains why no shell attempt has started after Wi-Fi connects.
//
//export Java_center_dx_jni_jniservice_LocalAdbDiscovery_discoveryFailed
func Java_center_dx_jni_jniservice_LocalAdbDiscovery_discoveryFailed(
	env *C.JNIEnv,
	class C.jclass,
	generation C.jlong,
	code C.jint,
) {
	if globalStartup != nil {
		globalStartup.DiscoveryFailed(uint64(generation), int(code))
	}
}
