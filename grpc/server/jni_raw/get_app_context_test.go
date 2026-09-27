package jni_raw

import (
	"context"
	"errors"
	"testing"

	"github.com/AndroidGoLab/jni"
	"github.com/AndroidGoLab/jni-proxy/handlestore"
	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetAppContextZeroHandleReturnsFailedPrecondition(t *testing.T) {
	_, err := (&Server{Handles: handlestore.New()}).GetAppContext(
		context.Background(),
		&pb.GetAppContextRequest{},
	)

	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestGetAppContextRequiresHandleStore(t *testing.T) {
	_, err := (&Server{AppContextHandle: 1}).GetAppContext(
		context.Background(),
		&pb.GetAppContextRequest{},
	)

	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestGetAppContextMissingStoredHandleReturnsNotFound(t *testing.T) {
	_, err := (&Server{
		AppContextHandle: 1,
		Handles:          handlestore.New(),
	}).GetAppContext(context.Background(), &pb.GetAppContextRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestCallerOwnedAppContextHandleCreatesFreshHandle(t *testing.T) {
	storedObj := &jni.Object{}
	var gotLookupHandle int64
	var gotCreateObj *jni.Object

	handle, err := callerOwnedAppContextHandle(
		7,
		func(handle int64) *jni.Object {
			gotLookupHandle = handle
			return storedObj
		},
		func(obj *jni.Object) (int64, error) {
			gotCreateObj = obj
			return 11, nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, int64(11), handle)
	assert.Equal(t, int64(7), gotLookupHandle)
	assert.Same(t, storedObj, gotCreateObj)
}

func TestCallerOwnedAppContextHandlePropagatesCreateError(t *testing.T) {
	sentinelErr := errors.New("create app context handle")

	_, err := callerOwnedAppContextHandle(
		7,
		func(int64) *jni.Object {
			return &jni.Object{}
		},
		func(*jni.Object) (int64, error) {
			return 0, sentinelErr
		},
	)

	require.ErrorIs(t, err, sentinelErr)
}
