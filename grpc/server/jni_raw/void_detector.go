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

	d.voidType = env.GetStaticObjectField(voidCls, typeFID)
	if d.voidType == nil || d.voidType.Ref() == 0 {
		return fmt.Errorf("Void.TYPE is null")
	}

	return nil
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
