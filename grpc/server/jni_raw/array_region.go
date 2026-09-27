package jni_raw

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/AndroidGoLab/jni"
	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type primitiveArrayRegionValues struct {
	booleans []uint8
	bytes    []int8
	chars    []uint16
	shorts   []int16
	ints     []int32
	longs    []int64
	floats   []float32
	doubles  []float64
}

func (v primitiveArrayRegionValues) len(elementType pb.JType) int {
	switch elementType {
	case pb.JType_BOOLEAN:
		return len(v.booleans)
	case pb.JType_BYTE:
		return len(v.bytes)
	case pb.JType_CHAR:
		return len(v.chars)
	case pb.JType_SHORT:
		return len(v.shorts)
	case pb.JType_INT:
		return len(v.ints)
	case pb.JType_LONG:
		return len(v.longs)
	case pb.JType_FLOAT:
		return len(v.floats)
	case pb.JType_DOUBLE:
		return len(v.doubles)
	default:
		return 0
	}
}

func newPrimitiveArrayRegionValues(
	elementType pb.JType,
	elements []*pb.JValue,
) (primitiveArrayRegionValues, error) {
	var values primitiveArrayRegionValues

	switch elementType {
	case pb.JType_BOOLEAN:
		values.booleans = make([]uint8, len(elements))
		for idx, element := range elements {
			value, ok := element.GetValue().(*pb.JValue_Z)
			if !ok {
				return primitiveArrayRegionValues{}, arrayRegionValueError(idx, elementType)
			}
			if value.Z {
				values.booleans[idx] = 1
			}
		}
	case pb.JType_BYTE:
		values.bytes = make([]int8, len(elements))
		for idx, element := range elements {
			value, ok := element.GetValue().(*pb.JValue_B)
			if !ok {
				return primitiveArrayRegionValues{}, arrayRegionValueError(idx, elementType)
			}
			if value.B < -128 || value.B > 127 {
				return primitiveArrayRegionValues{}, fmt.Errorf("element %d for %s is out of byte range: %d", idx, elementType, value.B)
			}
			values.bytes[idx] = int8(value.B)
		}
	case pb.JType_CHAR:
		values.chars = make([]uint16, len(elements))
		for idx, element := range elements {
			value, ok := element.GetValue().(*pb.JValue_C)
			if !ok {
				return primitiveArrayRegionValues{}, arrayRegionValueError(idx, elementType)
			}
			if value.C > 65535 {
				return primitiveArrayRegionValues{}, fmt.Errorf("element %d for %s is out of char range: %d", idx, elementType, value.C)
			}
			values.chars[idx] = uint16(value.C)
		}
	case pb.JType_SHORT:
		values.shorts = make([]int16, len(elements))
		for idx, element := range elements {
			value, ok := element.GetValue().(*pb.JValue_S)
			if !ok {
				return primitiveArrayRegionValues{}, arrayRegionValueError(idx, elementType)
			}
			if value.S < -32768 || value.S > 32767 {
				return primitiveArrayRegionValues{}, fmt.Errorf("element %d for %s is out of short range: %d", idx, elementType, value.S)
			}
			values.shorts[idx] = int16(value.S)
		}
	case pb.JType_INT:
		values.ints = make([]int32, len(elements))
		for idx, element := range elements {
			value, ok := element.GetValue().(*pb.JValue_I)
			if !ok {
				return primitiveArrayRegionValues{}, arrayRegionValueError(idx, elementType)
			}
			values.ints[idx] = value.I
		}
	case pb.JType_LONG:
		values.longs = make([]int64, len(elements))
		for idx, element := range elements {
			value, ok := element.GetValue().(*pb.JValue_J)
			if !ok {
				return primitiveArrayRegionValues{}, arrayRegionValueError(idx, elementType)
			}
			values.longs[idx] = value.J
		}
	case pb.JType_FLOAT:
		values.floats = make([]float32, len(elements))
		for idx, element := range elements {
			value, ok := element.GetValue().(*pb.JValue_F)
			if !ok {
				return primitiveArrayRegionValues{}, arrayRegionValueError(idx, elementType)
			}
			values.floats[idx] = value.F
		}
	case pb.JType_DOUBLE:
		values.doubles = make([]float64, len(elements))
		for idx, element := range elements {
			value, ok := element.GetValue().(*pb.JValue_D)
			if !ok {
				return primitiveArrayRegionValues{}, arrayRegionValueError(idx, elementType)
			}
			values.doubles[idx] = value.D
		}
	default:
		return primitiveArrayRegionValues{}, fmt.Errorf("unsupported array element type %s", elementType)
	}

	return values, nil
}

func arrayRegionValueError(idx int, elementType pb.JType) error {
	return fmt.Errorf("element %d does not match array element type %s", idx, elementType)
}

func (s *Server) SetArrayRegion(_ context.Context, req *pb.SetArrayRegionRequest) (*pb.SetArrayRegionResponse, error) {
	obj, err := s.requireObject(req.GetArrayHandle())
	if err != nil {
		return nil, err
	}
	if req.GetStart() < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "start must be non-negative")
	}
	values, err := newPrimitiveArrayRegionValues(req.GetElementType(), req.GetElements())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if values.len(req.GetElementType()) == 0 {
		return &pb.SetArrayRegionResponse{}, nil
	}

	if err := s.withEnv(func(env *jni.Env) error {
		setPrimitiveArrayRegion(env, obj, req.GetStart(), req.GetElementType(), values)
		return nil
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.SetArrayRegionResponse{}, nil
}

func setPrimitiveArrayRegion(
	env *jni.Env,
	obj *jni.Object,
	start int32,
	elementType pb.JType,
	values primitiveArrayRegionValues,
) {
	switch elementType {
	case pb.JType_BOOLEAN:
		env.SetBooleanArrayRegion((*jni.BooleanArray)(unsafe.Pointer(obj)), start, int32(len(values.booleans)), unsafe.Pointer(&values.booleans[0]))
	case pb.JType_BYTE:
		env.SetByteArrayRegion((*jni.ByteArray)(unsafe.Pointer(obj)), start, int32(len(values.bytes)), unsafe.Pointer(&values.bytes[0]))
	case pb.JType_CHAR:
		env.SetCharArrayRegion((*jni.CharArray)(unsafe.Pointer(obj)), start, int32(len(values.chars)), unsafe.Pointer(&values.chars[0]))
	case pb.JType_SHORT:
		env.SetShortArrayRegion((*jni.ShortArray)(unsafe.Pointer(obj)), start, int32(len(values.shorts)), unsafe.Pointer(&values.shorts[0]))
	case pb.JType_INT:
		env.SetIntArrayRegion((*jni.IntArray)(unsafe.Pointer(obj)), start, int32(len(values.ints)), unsafe.Pointer(&values.ints[0]))
	case pb.JType_LONG:
		env.SetLongArrayRegion((*jni.LongArray)(unsafe.Pointer(obj)), start, int32(len(values.longs)), unsafe.Pointer(&values.longs[0]))
	case pb.JType_FLOAT:
		env.SetFloatArrayRegion((*jni.FloatArray)(unsafe.Pointer(obj)), start, int32(len(values.floats)), unsafe.Pointer(&values.floats[0]))
	case pb.JType_DOUBLE:
		env.SetDoubleArrayRegion((*jni.DoubleArray)(unsafe.Pointer(obj)), start, int32(len(values.doubles)), unsafe.Pointer(&values.doubles[0]))
	}
}
