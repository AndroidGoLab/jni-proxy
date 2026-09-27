package jni_raw

import (
	"testing"

	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimitiveArrayRegionValuesConvertsByteValues(t *testing.T) {
	values, err := newPrimitiveArrayRegionValues(pb.JType_BYTE, []*pb.JValue{
		{Value: &pb.JValue_B{B: 1}},
		{Value: &pb.JValue_B{B: -128}},
		{Value: &pb.JValue_B{B: -1}},
	})

	require.NoError(t, err)
	assert.Equal(t, []int8{1, -128, -1}, values.bytes)
}

func TestPrimitiveArrayRegionValuesRejectsMismatchedByteValue(t *testing.T) {
	values, err := newPrimitiveArrayRegionValues(pb.JType_BYTE, []*pb.JValue{
		{Value: &pb.JValue_I{I: 1}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "element 0")
	assert.Empty(t, values.bytes)
}
