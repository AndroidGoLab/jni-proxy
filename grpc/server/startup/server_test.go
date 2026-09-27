package startup

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"path/filepath"
	"testing"

	pb "github.com/AndroidGoLab/jni-proxy/proto/startup"
	domain "github.com/AndroidGoLab/jni-proxy/startup"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

// discoveryRecords captures asynchronous work requests without emitting a local ADB endpoint.
//
// Example: configuration RPC acceptance must not fabricate a successful shell result.
type discoveryRecords struct {
	Enabled    bool
	Generation uint64
}

// Update records desired platform discovery state for assertions at the RPC boundary.
//
// Example: disabling configuration immediately replaces an earlier enabled subscription.
func (d *discoveryRecords) Update(
	_ context.Context,
	generation uint64,
	enabled bool,
) error {
	d.Enabled = enabled
	d.Generation = generation
	return nil
}

// TestStartupRPCStatus verifies key redaction, default disablement, invalid input and separate apply semantics.
//
// Example: Configure returns waiting with no exit code until an actual local shell completes.
func TestStartupRPCStatus(t *testing.T) {
	discovery := &discoveryRecords{}
	controller := domain.New(filepath.Join(t.TempDir(), "startup.json"), domain.ADBRunner{}, discovery)
	s := &Server{Controller: controller}
	before, err := s.GetStatus(t.Context(), &pb.GetStatusRequest{})
	require.NoError(t, err)
	require.False(t, before.GetConfigured())
	require.Nil(t, before.LastExitCode)
	_, err = s.Apply(t.Context(), &pb.ApplyRequest{})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	_, err = s.Configure(t.Context(), &pb.ConfigureRequest{PrivateKeyPem: "invalid", Enabled: true})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.NoError(t, controller.Start(t.Context()))
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	configured, err := s.Configure(t.Context(), &pb.ConfigureRequest{PrivateKeyPem: keyPEM, Enabled: true})
	require.NoError(t, err)
	require.True(t, configured.GetConfigured())
	require.True(t, discovery.Enabled)
	require.Equal(t, string(domain.StateWaiting), configured.State)
	require.Nil(t, configured.LastExitCode)
	encoded, err := protojson.Marshal(configured)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE KEY")
	require.NotContains(t, string(encoded), keyPEM)
	disabled, err := s.Configure(t.Context(), &pb.ConfigureRequest{Enabled: false})
	require.NoError(t, err)
	require.False(t, disabled.Enabled)
	require.False(t, discovery.Enabled)
	manual, err := s.Apply(t.Context(), &pb.ApplyRequest{})
	require.NoError(t, err)
	require.False(t, manual.Enabled)
	require.True(t, discovery.Enabled)
	require.Nil(t, manual.LastExitCode)
}
