package jni_raw

import (
	"errors"
	"io"
	"testing"
	"unsafe"

	"github.com/AndroidGoLab/jni"
	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"github.com/AndroidGoLab/jni/capi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type proxyCallbackSenderFunc func(*pb.ProxyServerMessage) error

func (fn proxyCallbackSenderFunc) Send(msg *pb.ProxyServerMessage) error {
	return fn(msg)
}

type recordingCallbackHandleStore struct {
	nextHandles []int64
	putHandles  []int64
	released    []int64
	objects     map[int64]*jni.Object
}

func (s *recordingCallbackHandleStore) Put(
	_ *jni.Env,
	obj *jni.Object,
) int64 {
	if s.objects == nil {
		s.objects = make(map[int64]*jni.Object)
	}
	handle := int64(len(s.objects) + 1)
	if len(s.nextHandles) > 0 {
		handle = s.nextHandles[0]
		s.nextHandles = s.nextHandles[1:]
	}
	s.objects[handle] = obj
	s.putHandles = append(s.putHandles, handle)
	return handle
}

func (s *recordingCallbackHandleStore) Get(handle int64) *jni.Object {
	return s.objects[handle]
}

func (s *recordingCallbackHandleStore) Release(
	_ *jni.Env,
	handle int64,
) {
	s.released = append(s.released, handle)
	delete(s.objects, handle)
}

type testObjectWithRef struct {
	ref capi.Object
}

func newTestJNIObject(ref capi.Object) *jni.Object {
	return (*jni.Object)(unsafe.Pointer(&testObjectWithRef{ref: ref}))
}

func TestProxyCallbackForwarderFireAndForgetIgnoresClosedStreamSend(t *testing.T) {
	var sent []*pb.ProxyServerMessage
	forwarder := newProxyCallbackForwarder(
		nil,
		proxyCallbackSenderFunc(func(msg *pb.ProxyServerMessage) error {
			sent = append(sent, msg)
			return io.EOF
		}),
	)

	result, err := forwarder.sendCallback(nil, "onScanResult", nil, false)

	require.NoError(t, err)
	assert.Nil(t, result)
	require.Len(t, sent, 1)
	assert.False(t, sent[0].GetCallback().GetExpectsResponse())
	assert.Zero(t, forwarder.pendingLen())
}

func TestProxyCallbackForwarderFireAndForgetIgnoresClosingTransportSend(t *testing.T) {
	var sent []*pb.ProxyServerMessage
	forwarder := newProxyCallbackForwarder(
		nil,
		proxyCallbackSenderFunc(func(msg *pb.ProxyServerMessage) error {
			sent = append(sent, msg)
			return status.Error(codes.Unavailable, "transport is closing")
		}),
	)

	result, err := forwarder.sendCallback(nil, "onScanResult", nil, false)

	require.NoError(t, err)
	assert.Nil(t, result)
	require.Len(t, sent, 1)
	assert.False(t, sent[0].GetCallback().GetExpectsResponse())
	assert.Zero(t, forwarder.pendingLen())
}

func TestProxyCallbackForwarderFireAndForgetReleasesArgHandlesAfterClosingTransportSend(t *testing.T) {
	handles := &recordingCallbackHandleStore{
		nextHandles: []int64{101, 202},
	}
	forwarder := newProxyCallbackForwarder(
		handles,
		proxyCallbackSenderFunc(func(*pb.ProxyServerMessage) error {
			return status.Error(codes.Unavailable, "transport is closing")
		}),
	)

	result, err := forwarder.sendCallback(
		nil,
		"onScanResult",
		[]*jni.Object{
			newTestJNIObject(1),
			nil,
			newTestJNIObject(2),
		},
		false,
	)

	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, []int64{101, 202}, handles.putHandles)
	assert.Equal(t, []int64{101, 202}, handles.released)
	assert.Empty(t, handles.objects)
}

func TestProxyCallbackForwarderFireAndForgetPropagatesUnknownSendError(t *testing.T) {
	sendErr := errors.New("transport write failed")
	forwarder := newProxyCallbackForwarder(
		nil,
		proxyCallbackSenderFunc(func(*pb.ProxyServerMessage) error {
			return sendErr
		}),
	)

	result, err := forwarder.sendCallback(nil, "onScanResult", nil, false)

	require.ErrorIs(t, err, sendErr)
	assert.Nil(t, result)
	assert.Zero(t, forwarder.pendingLen())
}

func TestProxyCallbackForwarderFireAndForgetPropagatesOtherUnavailableSendError(t *testing.T) {
	sendErr := status.Error(codes.Unavailable, "connection refused")
	forwarder := newProxyCallbackForwarder(
		nil,
		proxyCallbackSenderFunc(func(*pb.ProxyServerMessage) error {
			return sendErr
		}),
	)

	result, err := forwarder.sendCallback(nil, "onScanResult", nil, false)

	require.ErrorIs(t, err, sendErr)
	assert.Nil(t, result)
	assert.Zero(t, forwarder.pendingLen())
}

func TestProxyCallbackForwarderResponseRequiredPropagatesClosingTransportSendError(t *testing.T) {
	sendErr := status.Error(codes.Unavailable, "transport is closing")
	forwarder := newProxyCallbackForwarder(
		nil,
		proxyCallbackSenderFunc(func(*pb.ProxyServerMessage) error {
			return sendErr
		}),
	)

	result, err := forwarder.sendCallback(nil, "onScanResult", nil, true)

	require.ErrorIs(t, err, sendErr)
	assert.Nil(t, result)
	assert.Zero(t, forwarder.pendingLen())
}

func TestProxyCallbackForwarderResponseRequiredSurfacesClosedResponse(t *testing.T) {
	ch := make(chan *pb.CallbackResponse)
	close(ch)

	result, err := callbackResultFromResponse(nil, ch)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream closed while waiting for callback response")
	assert.Nil(t, result)
}
