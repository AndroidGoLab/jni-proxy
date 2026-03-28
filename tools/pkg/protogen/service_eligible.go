package protogen

import "github.com/AndroidGoLab/jni/tools/pkg/javagen"

// IsServiceEligible reports whether a class should get a proto service
// definition and client wrappers. A class is eligible when it has methods
// and is not a data-only type (data_class, iterable_data, builder).
func IsServiceEligible(cls javagen.MergedClass) bool {
	if len(cls.Methods) == 0 {
		return false
	}
	switch cls.Kind {
	case "data_class", "iterable_data", "builder":
		return false
	}
	return true
}

// IsServerEligible reports whether a class should get a gRPC server
// implementation. system_service classes use NewXxx(ctx *app.Context) and
// constructor classes use NewXxx(vm *jni.VM, ...) with handle-based dispatch.
func IsServerEligible(cls javagen.MergedClass) bool {
	if !IsServiceEligible(cls) {
		return false
	}
	switch cls.Obtain {
	case "system_service", "constructor":
		return true
	}
	return false
}
