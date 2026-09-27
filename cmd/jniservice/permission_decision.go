//go:build android

package main

/*
#include <jni.h>
*/
import "C"

import (
	"database/sql"
	"errors"
	"log/slog"
	"sync/atomic"

	"github.com/AndroidGoLab/jni-proxy/grpc/server/acl"
)

// activeACL publishes the same successfully opened Store used by RPC authorization.
//
// Example: a restored dialog sees unavailable instead of opening another SQLite engine.
var activeACL atomic.Pointer[acl.Store]

// Decision result codes are the private JNI contract consumed by PermissionDecision.java.
//
// Example: decisionMissing means no pending row was changed and must not look successful.
const (
	decisionSaved = iota
	decisionUnavailable
	decisionMissing
	decisionFailed
)

// Java_center_dx_jni_jniservice_PermissionDialogActivity_decideRequest applies a local user decision to the live Store.
//
// Example: approval derives the client and methods from the pending database row, not Intent extras.
//
//export Java_center_dx_jni_jniservice_PermissionDialogActivity_decideRequest
func Java_center_dx_jni_jniservice_PermissionDialogActivity_decideRequest(
	env *C.JNIEnv,
	class C.jclass,
	requestID C.jlong,
	approve C.jboolean,
) C.jint {
	store := activeACL.Load()
	if store == nil {
		return decisionUnavailable
	}
	if requestID <= 0 {
		return decisionMissing
	}
	var err error
	switch approve {
	case 0:
		err = store.DenyRequest(int64(requestID))
	case 1:
		err = store.ApproveRequest(int64(requestID), "ui")
	default:
		return decisionFailed
	}
	switch {
	case err == nil:
		return decisionSaved
	case errors.Is(err, sql.ErrNoRows):
		return decisionMissing
	default:
		slog.Error("local permission decision failed", "request_id", int64(requestID), "error", err)
		return decisionFailed
	}
}
