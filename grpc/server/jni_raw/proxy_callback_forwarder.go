package jni_raw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/AndroidGoLab/jni"
	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type proxyCallbackSender interface {
	Send(*pb.ProxyServerMessage) error
}

type proxyCallbackHandleStore interface {
	Put(
		env *jni.Env,
		obj *jni.Object,
	) int64
	Get(handle int64) *jni.Object
	Release(
		env *jni.Env,
		handle int64,
	)
}

type proxyCallbackForwarder struct {
	handles proxyCallbackHandleStore
	sender  proxyCallbackSender

	pendingMu sync.Mutex
	pending   map[int64]chan *pb.CallbackResponse

	sendMu sync.Mutex
	nextID atomic.Int64
}

func newProxyCallbackForwarder(
	handles proxyCallbackHandleStore,
	sender proxyCallbackSender,
) *proxyCallbackForwarder {
	return &proxyCallbackForwarder{
		handles: handles,
		sender:  sender,
		pending: make(map[int64]chan *pb.CallbackResponse),
	}
}

func (f *proxyCallbackForwarder) sendCallback(
	env *jni.Env,
	methodName string,
	args []*jni.Object,
	expectsResponse bool,
) (*jni.Object, error) {
	callbackID := f.nextID.Add(1)
	argHandles := f.argHandles(env, args)
	event := &pb.ProxyServerMessage{
		Msg: &pb.ProxyServerMessage_Callback{
			Callback: &pb.CallbackEvent{
				CallbackId:      callbackID,
				MethodName:      methodName,
				ArgHandles:      argHandles,
				ExpectsResponse: expectsResponse,
			},
		},
	}

	var ch chan *pb.CallbackResponse
	if expectsResponse {
		ch = f.registerPending(callbackID)
	}

	if err := f.send(event); err != nil {
		f.releaseArgHandles(env, argHandles)
		if expectsResponse {
			f.unregisterPending(callbackID)
		}
		if !expectsResponse && isClosedProxyStreamError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("sending callback event: %w", err)
	}

	if !expectsResponse {
		return nil, nil
	}

	defer f.unregisterPending(callbackID)
	return callbackResultFromResponse(f.handles, ch)
}

func (f *proxyCallbackForwarder) argHandles(
	env *jni.Env,
	args []*jni.Object,
) []int64 {
	argHandles := make([]int64, len(args))
	if f.handles == nil {
		return argHandles
	}
	for i, arg := range args {
		if arg != nil && arg.Ref() != 0 {
			argHandles[i] = f.handles.Put(env, arg)
		}
	}
	return argHandles
}

func (f *proxyCallbackForwarder) releaseArgHandles(
	env *jni.Env,
	argHandles []int64,
) {
	if f.handles == nil {
		return
	}
	for _, handle := range argHandles {
		if handle == 0 {
			continue
		}
		f.handles.Release(env, handle)
	}
}

func (f *proxyCallbackForwarder) send(event *pb.ProxyServerMessage) error {
	f.sendMu.Lock()
	defer f.sendMu.Unlock()
	return f.sender.Send(event)
}

func (f *proxyCallbackForwarder) registerPending(callbackID int64) chan *pb.CallbackResponse {
	ch := make(chan *pb.CallbackResponse, 1)
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	f.pending[callbackID] = ch
	return ch
}

func (f *proxyCallbackForwarder) unregisterPending(callbackID int64) {
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	delete(f.pending, callbackID)
}

func (f *proxyCallbackForwarder) receiveResponse(resp *pb.CallbackResponse) {
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	ch, ok := f.pending[resp.GetCallbackId()]
	if ok {
		ch <- resp
	}
}

func (f *proxyCallbackForwarder) closePending() {
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	for id, ch := range f.pending {
		close(ch)
		delete(f.pending, id)
	}
}

func (f *proxyCallbackForwarder) pendingLen() int {
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	return len(f.pending)
}

func callbackResultFromResponse(
	handles proxyCallbackHandleStore,
	ch <-chan *pb.CallbackResponse,
) (*jni.Object, error) {
	resp, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("stream closed while waiting for callback response")
	}

	if resp.GetError() != "" {
		return nil, fmt.Errorf("client error: %s", resp.GetError())
	}

	resultHandle := resp.GetResultHandle()
	if resultHandle == 0 {
		return nil, nil
	}
	if handles == nil {
		return nil, fmt.Errorf("handle store unavailable for result handle %d", resultHandle)
	}
	return handles.Get(resultHandle), nil
}

func isClosedProxyStreamError(err error) bool {
	switch {
	case errors.Is(err, context.Canceled):
		return true
	case errors.Is(err, io.EOF):
		return true
	case errors.Is(err, io.ErrClosedPipe):
		return true
	case errors.Is(err, net.ErrClosed):
		return true
	case status.Code(err) == codes.Canceled:
		return true
	case status.Code(err) == codes.Unavailable &&
		strings.Contains(status.Convert(err).Message(), "transport is closing"):
		return true
	default:
		return false
	}
}
