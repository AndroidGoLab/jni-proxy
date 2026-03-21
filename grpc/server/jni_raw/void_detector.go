package jni_raw

import (
	"fmt"
	"sync"

	"github.com/AndroidGoLab/jni"
)

// voidDetector caches JNI class/method/field IDs needed to detect whether a
// java.lang.reflect.Method returns void. The lookups are performed once via
// sync.Once so that subsequent proxy callbacks avoid repeated FindClass /
// GetMethodID / GetStaticFieldID calls on hot paths.
type voidDetector struct {
	once sync.Once
	err  error

	getReturnMID jni.MethodID // Method.getReturnType() -> Class
	voidType     *jni.Object  // cached Void.TYPE value
}

func (d *voidDetector) init(env *jni.Env) error {
	d.once.Do(func() {
		d.err = d.doInit(env)
	})
	return d.err
}

func (d *voidDetector) doInit(env *jni.Env) error {
	methodCls, err := env.FindClass("java/lang/reflect/Method")
	if err != nil {
		return fmt.Errorf("finding java.lang.reflect.Method: %w", err)
	}

	d.getReturnMID, err = env.GetMethodID(methodCls, "getReturnType", "()Ljava/lang/Class;")
	if err != nil {
		return fmt.Errorf("finding Method.getReturnType: %w", err)
	}

	voidCls, err := env.FindClass("java/lang/Void")
	if err != nil {
		return fmt.Errorf("finding java.lang.Void: %w", err)
	}

	typeFID, err := env.GetStaticFieldID(voidCls, "TYPE", "Ljava/lang/Class;")
	if err != nil {
		return fmt.Errorf("finding Void.TYPE: %w", err)
	}

	localRef := env.GetStaticObjectField(voidCls, typeFID)
	if localRef == nil || localRef.Ref() == 0 {
		return fmt.Errorf("Void.TYPE is null")
	}
	// Promote to a global reference so it remains valid across VM.Do frames.
	d.voidType = env.NewGlobalRef(localRef)

	return nil
}

// close releases the JNI global reference held by the voidDetector.
// Must be called inside a VM.Do callback when the detector is no longer needed.
func (d *voidDetector) close(env *jni.Env) {
	if d.voidType != nil {
		env.DeleteGlobalRef(d.voidType)
		d.voidType = nil
	}
}

// isVoid returns true when method's return type is java.lang.Void.TYPE.
func (d *voidDetector) isVoid(
	env *jni.Env,
	method *jni.Object,
) bool {
	if method == nil {
		return true
	}

	if err := d.init(env); err != nil {
		// Cannot determine return type; assume non-void to be safe.
		return false
	}

	retType, err := env.CallObjectMethod(method, d.getReturnMID)
	if err != nil || retType == nil || retType.Ref() == 0 {
		return false
	}

	return env.IsSameObject(retType, d.voidType)
}
