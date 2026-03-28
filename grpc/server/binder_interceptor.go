package server

import (
	"context"
	"fmt"
	"os"

	"github.com/AndroidGoLab/jni"
	"google.golang.org/grpc"
)

// setSystemServerBinderIdentity forces the Binder calling identity on the
// current thread to uid 1000 (system_server). This is necessary when the
// jniservice process runs as root (uid 0): Binder.clearCallingIdentity()
// would set getCallingUid() to the process uid (0), but Android services
// like LocationManager reject uid 0 because no package belongs to it.
//
// By calling Binder.restoreCallingIdentity() with a forged token encoding
// uid 1000, getCallingUid() returns 1000 (system_server). The package
// "android" is valid for uid 1000, so CallerIdentity checks pass.
//
// Token format (per AOSP IPCThreadState.cpp):
//
//	(uint64(callingUid) << 32) | callingPid
//
// The caller MUST have already attached the current thread to the JVM
// (e.g. via the Looper interceptor or vm.Do).
//
// This is best-effort: if any JNI call fails, the function returns
// silently and lets the actual gRPC handler produce a proper error.
func setSystemServerBinderIdentity(env *jni.Env) {
	binderCls, err := env.FindClass("android/os/Binder")
	if err != nil {
		return
	}

	// First clear the calling identity to get the current process pid.
	clearMID, err := env.GetStaticMethodID(binderCls, "clearCallingIdentity", "()J")
	if err != nil {
		return
	}
	currentToken, err := env.CallStaticLongMethod(binderCls, clearMID)
	if err != nil {
		return
	}

	// Extract the pid from the lower 32 bits of the current token.
	pid := currentToken & 0xFFFFFFFF

	// Forge a token with uid 1000 (system_server) and the current pid.
	const systemServerUID = 1000
	forgedToken := (int64(systemServerUID) << 32) | pid

	restoreMID, err := env.GetStaticMethodID(binderCls, "restoreCallingIdentity", "(J)V")
	if err != nil {
		return
	}
	_ = env.CallStaticVoidMethod(binderCls, restoreMID, jni.LongValue(forgedToken))
}

// UnaryBinderInterceptor returns a gRPC unary interceptor that sets the
// Binder calling identity to system_server (uid 1000) on the current
// thread before each RPC handler runs.
//
// Binder identity is per-thread. gRPC methods run on different goroutines
// (and therefore different OS threads). The startup-time identity change
// in runServer only affects the main thread. Each gRPC worker thread
// needs its own identity set so that Android API calls see the
// system_server identity instead of "uid 0".
//
// This interceptor MUST be chained AFTER the Looper interceptor (which
// pins the goroutine to an OS thread and attaches it to the JVM) but
// BEFORE interceptors that perform Android API calls.
func UnaryBinderInterceptor(vm *jni.VM) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		// The Looper interceptor already called LockOSThread and
		// AttachCurrentThread, so the thread is attached. Use vm.Do
		// which will find the thread already attached via GetEnv.
		if doErr := vm.Do(func(env *jni.Env) error {
			setSystemServerBinderIdentity(env)
			return nil
		}); doErr != nil {
			fmt.Fprintf(os.Stderr, "jniservice: WARNING: binder interceptor: %v\n", doErr)
		}
		return handler(ctx, req)
	}
}

// StreamBinderInterceptor returns a gRPC stream interceptor that sets
// the Binder calling identity to system_server (uid 1000) on the current
// thread before each stream handler runs. See UnaryBinderInterceptor
// for rationale.
func StreamBinderInterceptor(vm *jni.VM) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if doErr := vm.Do(func(env *jni.Env) error {
			setSystemServerBinderIdentity(env)
			return nil
		}); doErr != nil {
			fmt.Fprintf(os.Stderr, "jniservice: WARNING: binder stream interceptor: %v\n", doErr)
		}
		return handler(srv, ss)
	}
}
