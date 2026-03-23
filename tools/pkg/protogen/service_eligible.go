package protogen

import "github.com/AndroidGoLab/jni/tools/pkg/javagen"

// IsServiceEligible reports whether a class should get a proto service
// and gRPC server/client wrappers. A class is eligible when it has methods
// and uses the system_service obtain pattern (which generates a
// NewXxx(ctx *app.Context) constructor needed by the gRPC server).
func IsServiceEligible(cls javagen.MergedClass) bool {
	if len(cls.Methods) == 0 {
		return false
	}
	switch cls.Kind {
	case "data_class", "iterable_data", "builder":
		return false
	}
	return cls.Obtain == "system_service"
}
