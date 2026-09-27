package jni_raw

import (
	"context"
	"testing"

	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDeleteGlobalRefZeroHandleReturnsEmptyResponse(t *testing.T) {
	resp, err := (&Server{}).DeleteGlobalRef(context.Background(), &pb.DeleteGlobalRefRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestDeleteGlobalRefNonZeroRequiresRuntimeState(t *testing.T) {
	_, err := (&Server{}).DeleteGlobalRef(
		context.Background(),
		&pb.DeleteGlobalRefRequest{RefHandle: 123},
	)

	require.Error(t, err)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}
