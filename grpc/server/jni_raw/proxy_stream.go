package jni_raw

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unsafe"

	"github.com/AndroidGoLab/jni"
	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Proxy implements the bidirectional streaming RPC that creates a Java
// dynamic proxy and forwards method invocations to the gRPC client as
// CallbackEvent messages. The client responds with CallbackResponse
// messages that are dispatched back to the blocked JVM callback thread.
func (s *Server) Proxy(stream pb.JNIService_ProxyServer) error {
	// 1. Read the first message -- must be CreateProxyRequest.
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}
	createReq := firstMsg.GetCreate()
	if createReq == nil {
		return status.Error(codes.InvalidArgument, "first message must be CreateProxyRequest")
	}

	// 2. Resolve interface classes from handles.
	ifaceHandles := createReq.GetInterfaceClassHandles()
	ifaces := make([]*jni.Class, len(ifaceHandles))
	for i, h := range ifaceHandles {
		cls, err := s.requireClass(h)
		if err != nil {
			return err
		}
		ifaces[i] = cls
	}

	// 3. Callback forwarding state. stream.Send may be called concurrently
	// from multiple JVM callback threads, and response-requiring callbacks need
	// a pending callback_id -> response channel map.
	forwarder := newProxyCallbackForwarder(s.Handles, stream)

	// 4. Cache for void return type detection (used only for interface proxies).
	var detector voidDetector

	// 5. Determine whether the first class is an interface or an abstract class,
	// then create the proxy object using the appropriate mechanism.
	var (
		proxyObj     *jni.Object
		proxyCleanup func()
		proxyHandle  int64
	)

	var isInterface bool
	if err := s.VM.Do(func(env *jni.Env) error {
		classCls, findErr := env.FindClass("java/lang/Class")
		if findErr != nil {
			return fmt.Errorf("finding java.lang.Class: %w", findErr)
		}
		isIfaceMID, findErr := env.GetMethodID(classCls, "isInterface", "()Z")
		if findErr != nil {
			return fmt.Errorf("finding Class.isInterface: %w", findErr)
		}
		ret, callErr := env.CallBooleanMethod(&ifaces[0].Object, isIfaceMID)
		if callErr != nil {
			return fmt.Errorf("calling Class.isInterface: %w", callErr)
		}
		isInterface = ret != 0
		return nil
	}); err != nil {
		return status.Errorf(codes.Internal, "checking isInterface: %v", err)
	}

	switch {
	case isInterface:
		// Interface path: use the existing NewProxyFull mechanism.
		if err := s.VM.Do(func(env *jni.Env) error {
			var createErr error
			proxyObj, proxyCleanup, createErr = env.NewProxyFull(ifaces,
				func(env *jni.Env, method *jni.Object, methodName string, args []*jni.Object) (*jni.Object, error) {
					expectsResponse := !detector.isVoid(env, method)
					return forwarder.sendCallback(env, methodName, args, expectsResponse)
				},
			)
			if createErr != nil {
				return createErr
			}
			// Store in HandleStore within the same VM.Do — local refs are
			// thread-local and invalid across VM.Do boundaries.
			proxyHandle = s.Handles.Put(env, proxyObj)
			return nil
		}); err != nil {
			return status.Errorf(codes.Internal, "creating interface proxy: %v", err)
		}

	default:
		// Abstract class path: ensure native methods are registered (the
		// interface path does this inside NewProxyFull, but we bypass it here).
		if err := s.VM.Do(func(env *jni.Env) error {
			return jni.EnsureProxyInit(env)
		}); err != nil {
			return status.Errorf(codes.Internal, "proxy init: %v", err)
		}

		// Register a basic ProxyHandler, find the generated adapter class,
		// and instantiate it with the handler ID.
		handlerID := jni.RegisterProxyHandler(
			func(env *jni.Env, methodName string, args []*jni.Object) (*jni.Object, error) {
				// callbackgen currently generates only void abstract adapter
				// methods, so these lifecycle callbacks are fire-and-forget.
				return forwarder.sendCallback(env, methodName, args, false)
			},
		)

		if err := s.VM.Do(func(env *jni.Env) error {
			// Get the class's full name to derive the adapter class name.
			// E.g. "android.hardware.camera2.CameraDevice$StateCallback"
			//   → adapter name: "CameraDeviceStateCallbackAdapter"
			classCls, findErr := env.FindClass("java/lang/Class")
			if findErr != nil {
				return fmt.Errorf("finding java.lang.Class: %w", findErr)
			}
			getNameMID, findErr := env.GetMethodID(classCls, "getName", "()Ljava/lang/String;")
			if findErr != nil {
				return fmt.Errorf("finding Class.getName: %w", findErr)
			}

			nameObj, callErr := env.CallObjectMethod(&ifaces[0].Object, getNameMID)
			if callErr != nil {
				return fmt.Errorf("calling Class.getName: %w", callErr)
			}
			fullName := env.GoString((*jni.String)(unsafe.Pointer(nameObj)))

			// Extract the adapter name: take the last package segment + inner
			// class parts, remove '$' and '.', append "Adapter".
			// "android.hardware.camera2.CameraDevice$StateCallback"
			//  → lastDot split: "CameraDevice$StateCallback"
			//  → replace "$" with "": "CameraDeviceStateCallback"
			//  → append "Adapter": "CameraDeviceStateCallbackAdapter"
			shortName := fullName
			if idx := strings.LastIndex(fullName, "."); idx >= 0 {
				shortName = fullName[idx+1:]
			}
			shortName = strings.ReplaceAll(shortName, "$", "")
			adapterJavaName := "center.dx.jni.generated." + shortName + "Adapter"

			// Load the adapter class via AppClassLoader (native threads
			// use BootClassLoader which cannot find APK classes).
			adapterCls, loadErr := s.loadClassViaAppClassLoader(env, adapterJavaName)
			if loadErr != nil {
				return fmt.Errorf("loading adapter class %s: %w", adapterJavaName, loadErr)
			}

			// Get the adapter's constructor: <init>(long handlerID).
			ctorMID, findErr := env.GetMethodID(adapterCls, "<init>", "(J)V")
			if findErr != nil {
				return fmt.Errorf("finding %s constructor: %w", adapterJavaName, findErr)
			}

			// Instantiate the adapter, passing the handler ID.
			adapterObj, newErr := env.NewObject(adapterCls, ctorMID, jni.LongValue(handlerID))
			if newErr != nil {
				return fmt.Errorf("instantiating %s: %w", adapterJavaName, newErr)
			}

			proxyObj = adapterObj
			// Store in HandleStore within the same VM.Do — local refs are
			// thread-local and invalid across VM.Do boundaries.
			proxyHandle = s.Handles.Put(env, adapterObj)
			return nil
		}); err != nil {
			jni.UnregisterProxyHandler(handlerID)
			return status.Errorf(codes.Internal, "creating abstract class proxy: %v", err)
		}

		proxyCleanup = func() {
			jni.UnregisterProxyHandler(handlerID)
		}
	}
	defer proxyCleanup()
	defer func() {
		// Release the voidDetector's cached JNI global ref (Void.TYPE).
		_ = s.VM.Do(func(env *jni.Env) error {
			detector.close(env)
			return nil
		})
	}()

	// Close all pending callback channels when the stream ends so that
	// blocked callback goroutines do not deadlock.
	defer func() {
		forwarder.closePending()
	}()

	// 6. proxyHandle was set inside the VM.Do callbacks above (local JNI
	// refs are thread-local — must be stored before VM.Do returns).

	err = func() error {
		return forwarder.send(&pb.ProxyServerMessage{
			Msg: &pb.ProxyServerMessage_Created{
				Created: &pb.CreateProxyResponse{
					ProxyHandle: proxyHandle,
				},
			},
		})
	}()
	if err != nil {
		return err
	}

	// 7. Receive loop: read CallbackResponse messages and dispatch.
	for {
		msg, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			return nil
		}
		if recvErr != nil {
			return recvErr
		}

		resp := msg.GetCallbackResponse()
		if resp == nil {
			continue
		}
		forwarder.receiveResponse(resp)
	}
}

// loadClassViaAppClassLoader loads a Java class by its dot-separated
// name (e.g. "center.dx.jni.generated.FooAdapter") using the server's
// AppClassLoader. This is necessary because native threads attach to
// BootClassLoader which cannot see APK classes.
func (s *Server) loadClassViaAppClassLoader(
	env *jni.Env,
	javaClassName string,
) (*jni.Class, error) {
	// First try JNI FindClass with the slash-separated name.
	slashName := strings.ReplaceAll(javaClassName, ".", "/")
	cls, err := env.FindClass(slashName)
	if err == nil {
		return cls, nil
	}

	if s.AppClassLoader == 0 {
		return nil, fmt.Errorf("FindClass failed and no AppClassLoader available: %w", err)
	}
	env.ExceptionClear()

	clObj := s.getObject(s.AppClassLoader)
	if clObj == nil {
		return nil, fmt.Errorf("AppClassLoader handle %d not found", s.AppClassLoader)
	}

	clCls, findErr := env.FindClass("java/lang/ClassLoader")
	if findErr != nil {
		return nil, fmt.Errorf("finding java.lang.ClassLoader: %w", findErr)
	}
	loadMID, findErr := env.GetMethodID(clCls, "loadClass", "(Ljava/lang/String;)Ljava/lang/Class;")
	if findErr != nil {
		return nil, fmt.Errorf("finding ClassLoader.loadClass: %w", findErr)
	}

	nameStr, findErr := env.NewStringUTF(javaClassName)
	if findErr != nil {
		return nil, fmt.Errorf("creating Java string for class name: %w", findErr)
	}

	classObj, callErr := env.CallObjectMethod(clObj, loadMID, jni.ObjectValue(&nameStr.Object))
	if callErr != nil {
		return nil, fmt.Errorf("ClassLoader.loadClass(%s): %w", javaClassName, callErr)
	}
	if classObj == nil || classObj.Ref() == 0 {
		return nil, fmt.Errorf("ClassLoader.loadClass(%s) returned null", javaClassName)
	}

	return (*jni.Class)(unsafe.Pointer(classObj)), nil
}
