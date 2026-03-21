package server

import (
	"context"
	"runtime"

	"github.com/AndroidGoLab/jni"
	"google.golang.org/grpc"
)

// ensureLooper prepares an Android Looper on the current OS thread if one
// is not already present. Many Android system services (InputMethodManager,
// WindowManager, etc.) create a Handler internally with
// Handler(Looper.myLooper()), which NPEs when the thread has no Looper.
//
// gRPC handler goroutines run on arbitrary OS threads that the Go runtime
// assigns. These threads are never the main init thread where
// Looper.prepare() was called. This function fills that gap.
//
// The caller MUST have called runtime.LockOSThread() before invoking this
// function, and the current thread MUST already be attached to the JVM
// (i.e. env must be valid).
//
// ensureLooper is best-effort: if any JNI call fails, the function returns
// silently and lets the actual gRPC handler produce a proper error if a
// Looper is truly required.
func ensureLooper(env *jni.Env) {
	looperCls, err := env.FindClass("android/os/Looper")
	if err != nil {
		return
	}

	myLooperMID, err := env.GetStaticMethodID(looperCls, "myLooper", "()Landroid/os/Looper;")
	if err != nil {
		return
	}

	looper, err := env.CallStaticObjectMethod(looperCls, myLooperMID)
	if err != nil {
		return
	}

	if looper != nil && looper.Ref() != 0 {
		return
	}

	prepareMID, err := env.GetStaticMethodID(looperCls, "prepare", "()V")
	if err != nil {
		return
	}

	_ = env.CallStaticVoidMethod(looperCls, prepareMID)
}

// UnaryLooperInterceptor returns a gRPC unary interceptor that pins the
// handler goroutine to its OS thread, attaches it to the JVM for the
// entire handler lifetime, and ensures an Android Looper is prepared
// before the handler executes.
//
// Keeping the JVM attachment alive is critical: if the thread detaches
// between Looper.prepare() and the handler's JNI calls, the JVM destroys
// the thread-local Looper state (sets mQueue = null), causing NPEs in
// services like InputMethodManager and WindowManager.
//
// This must be chained BEFORE the auth interceptor or any interceptor that
// performs JNI work, but after interceptors that don't need JNI.
//
// Note: LockOSThread pins one OS thread per concurrent RPC. This is
// acceptable for the expected single-device workload but could exhaust
// threads under extreme concurrency.
func UnaryLooperInterceptor(vm *jni.VM) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		// Pin the goroutine to this OS thread for the entire handler
		// lifetime so the Looper stays associated with the right thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Attach to the JVM and keep attached for the handler duration.
		// This prevents the Looper from being destroyed between
		// ensureLooper and the actual handler JNI calls.
		env, err := vm.AttachCurrentThread()
		if err != nil {
			return nil, err
		}
		ensureLooper(env)
		// Do NOT detach here — nested vm.Do calls will find the thread
		// already attached (via GetEnv) and will not detach on return.
		// The thread stays attached until the OS thread is recycled
		// or the process exits. This is safe because LockOSThread
		// keeps the goroutine pinned, and the Go runtime will reuse
		// this thread for future goroutines after UnlockOSThread.
		return handler(ctx, req)
	}
}

// StreamLooperInterceptor returns a gRPC stream interceptor that pins the
// handler goroutine to its OS thread, attaches it to the JVM for the
// entire handler lifetime, and ensures an Android Looper is prepared
// before the handler executes.
func StreamLooperInterceptor(vm *jni.VM) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		env, attachErr := vm.AttachCurrentThread()
		if attachErr != nil {
			return attachErr
		}
		ensureLooper(env)
		return handler(srv, ss)
	}
}
