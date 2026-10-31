package adb

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// testClient creates a disposable RSA identity, never a developer's ADB key.
//
// Example: protocol tests authenticate only against a local test server.
func testClient(t *testing.T) *Client {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	client, err := NewClient(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	require.NoError(t, err)
	return client
}

// TestKeyFormats verifies supported private keys and rejects ambiguous or invalid input.
//
// Example: a PEM certificate cannot silently become an ADB identity.
func TestKeyFormats(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	for _, block := range []pem.Block{{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}, {Type: "PRIVATE KEY", Bytes: pkcs8}} {
		data := pem.EncodeToMemory(&block)
		_, err = NewClient(data)
		require.NoError(t, err)
		_, err = NewClient(append(data, data...))
		require.Error(t, err)
	}
	for _, data := range [][]byte{nil, []byte("not a key"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1}})} {
		_, err = NewClient(data)
		require.Error(t, err)
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecdsaDER, err := x509.MarshalPKCS8PrivateKey(ecdsaKey)
	require.NoError(t, err)
	for _, data := range [][]byte{make([]byte, 17000), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte{1}}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecdsaDER})} {
		_, err := NewClient(data)
		require.Error(t, err)
	}
}

// TestPacketBounds rejects invalid framing before allocating or accepting a payload.
//
// Example: an oversized ADB packet fails even if its payload has not arrived.
func TestPacketBounds(t *testing.T) {
	var valid bytes.Buffer
	require.NoError(t, writePacket(&valid, packet{Command: commandWrite, Arg0: 7, Arg1: 1, Data: []byte("test")}, maxPayload))
	p, err := readPacket(bytes.NewReader(valid.Bytes()))
	require.NoError(t, err)
	require.Equal(t, []byte("test"), p.Data)
	for _, offset := range []int{16, 20} {
		invalid := bytes.Clone(valid.Bytes())
		invalid[offset] ^= 1
		_, err := readPacket(bytes.NewReader(invalid))
		require.ErrorIs(t, err, ErrProtocol)
	}
	invalid := bytes.Clone(valid.Bytes())
	binary.LittleEndian.PutUint32(invalid[12:16], maxPayload+1)
	_, err = readPacket(bytes.NewReader(invalid[:24]))
	require.ErrorIs(t, err, ErrProtocol)
	_, err = readPacket(bytes.NewReader(valid.Bytes()[:25]))
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

// shellFrame creates independent shell-v2 wire bytes for parser falsification.
//
// Example: a frame can be split across several ADB WRTE packets.
func shellFrame(
	id byte,
	data []byte,
) []byte {
	frame := make([]byte, 5+len(data))
	frame[0] = id
	binary.LittleEndian.PutUint32(frame[1:5], uint32(len(data)))
	copy(frame[5:], data)
	return frame
}

// TestShellFrames checks fragmentation, output accounting and exactly one terminal exit.
//
// Example: closing without an exit never reports command success.
func TestShellFrames(t *testing.T) {
	wire := append(shellFrame(1, []byte("out")), shellFrame(2, []byte("err"))...)
	wire = append(wire, shellFrame(3, []byte{0})...)
	for split := 0; split <= len(wire); split++ {
		var stream shellStream
		require.NoError(t, stream.feed(wire[:split]))
		require.NoError(t, stream.feed(wire[split:]))
		result, err := stream.finish()
		require.NoError(t, err)
		require.Equal(t, Result{Stdout: "out", Stderr: "err", ExitCode: 0}, result)
	}
	for _, data := range [][]byte{
		nil, {1}, shellFrame(3, nil), shellFrame(3, []byte{0, 0}), shellFrame(9, nil),
		append(shellFrame(3, []byte{0}), shellFrame(3, []byte{0})...),
		append(shellFrame(3, []byte{0}), shellFrame(1, []byte("late"))...),
		shellFrame(1, make([]byte, maxOutput+1)),
		append(shellFrame(1, make([]byte, maxOutput)), shellFrame(2, []byte{1})...),
	} {
		var stream shellStream
		if err := stream.feed(data); err == nil {
			_, err = stream.finish()
			require.Error(t, err)
		}
	}
	var failed shellStream
	require.NoError(t, failed.feed(shellFrame(3, []byte{7})))
	result, err := failed.finish()
	require.Equal(t, 7, result.ExitCode)
	var exitError *ExitError
	require.ErrorAs(t, err, &exitError)
	require.Contains(t, exitError.Error(), "7")
	require.Nil(t, exitError.Unwrap())
}

// TestShellInputAndPhaseFailures checks rejected caller input and unsupported stream transitions.
//
// Example: a duplicate OKAY after the stream opened cannot reset its remote identity.
func TestShellInputAndPhaseFailures(t *testing.T) {
	client := testClient(t)
	for _, port := range []Port{-1, 0, 65536} {
		_, err := client.Shell(t.Context(), port, "id")
		require.Error(t, err)
	}
	for _, command := range []string{"", "id\x00extra", string(make([]byte, 5000))} {
		_, err := client.Shell(t.Context(), 1, command)
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := client.Shell(ctx, 1, "id")
	require.ErrorIs(t, err, context.Canceled)
	for _, p := range []packet{{Command: commandOkay, Arg0: 42, Arg1: 1}, {Command: commandClose, Arg0: 42, Arg1: 1, Data: []byte{1}}, {Command: "WHAT", Arg0: 42, Arg1: 1}, {Command: commandClose, Arg0: 42, Arg1: 1}} {
		var input bytes.Buffer
		require.NoError(t, writePacket(&input, packet{Command: commandOkay, Arg0: 42, Arg1: 1}, maxPayload))
		require.NoError(t, writePacket(&input, p, maxPayload))
		stream := &testStream{Reader: bytes.NewReader(input.Bytes())}
		_, err := runShell(stream, maxPayload, "id")
		require.ErrorIs(t, err, ErrProtocol)
	}
	require.ErrorIs(t, writePacket(io.Discard, packet{Command: "BAD"}, maxPayload), ErrProtocol)
	require.ErrorIs(t, writePacket(io.Discard, packet{Command: commandWrite, Data: []byte{1}}, 0), ErrProtocol)
	_, err = runShell(&testStream{Reader: bytes.NewReader(nil)}, maxPayload, "id")
	require.ErrorIs(t, err, io.EOF)
}

// TestFeatureNegotiation accepts only an explicitly advertised shell-v2 feature.
//
// Example: a similar property name or a feature substring cannot enable an unsupported protocol.
func TestFeatureNegotiation(t *testing.T) {
	for _, banner := range []string{"device::features=shell_v2\x00", "device::ro.product.name=phone;features=stat_v2,shell_v2\x00"} {
		require.True(t, hasShellV2([]byte(banner)))
	}
	for _, banner := range []string{"", "device::features=not_shell_v2\x00", "device::notfeatures=shell_v2\x00", "device::model=features=shell_v2\x00"} {
		require.False(t, hasShellV2([]byte(banner)), banner)
	}
}

// testStream separates scripted remote input from locally generated output.
//
// Example: a parser test cannot accidentally read the OPEN packet it just wrote.
type testStream struct {
	*bytes.Reader
	Output bytes.Buffer
}

// Write captures outgoing protocol data while Read consumes the independent script.
//
// Example: runShell's acknowledgements do not affect the next scripted remote packet.
func (s *testStream) Write(data []byte) (int, error) { return s.Output.Write(data) }

// TestTLSCommand validates authentication, stream IDs, phases and actual loopback I/O.
//
// Example: a forged stream ID is rejected instead of being acknowledged.
func TestTLSCommand(t *testing.T) {
	client := testClient(t)
	for _, scenario := range []string{"success", "wrong-stream", "write-before-open", "no-shell-v2", "reject-key", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			require.NoError(t, err)
			defer func() {
				if err := listener.Close(); err != nil {
					t.Errorf("close listener: %v", err)
				}
			}()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				serveErr := serveTestADB(ctx, conn, client, scenario, cancel)
				done <- errors.Join(serveErr, conn.Close())
			}()
			result, err := client.Shell(ctx, Port(listener.Addr().(*net.TCPAddr).Port), "id -u")
			switch scenario {
			case "success":
				require.NoError(t, err)
				require.Equal(t, "2000\n", result.Stdout)
				require.Zero(t, result.ExitCode)
			case "reject-key":
				require.ErrorIs(t, err, ErrAuthentication)
			case "cancel":
				require.ErrorIs(t, err, context.Canceled)
			default:
				require.ErrorIs(t, err, ErrProtocol)
			}
			require.NoError(t, <-done)
		})
	}
}

// serveTestADB emulates the relevant adbd protocol boundary, including its custom issuer list.
//
// Example: it refuses the TLS client unless GetClientCertificate supplies the test identity.
func serveTestADB(
	ctx context.Context,
	conn net.Conn,
	client *Client,
	scenario string,
	cancel context.CancelFunc,
) error {
	p, err := readPacket(conn)
	if err != nil {
		return err
	}
	if p.Command != commandConnect {
		return errors.New("missing greeting")
	}
	if err := writePacket(conn, packet{Command: commandTLS, Arg0: tlsVersion}, maxPayload); err != nil {
		return err
	}
	if _, err := readPacket(conn); err != nil {
		return err
	}
	pool := x509.NewCertPool()
	pool.AddCert(&x509.Certificate{RawSubject: []byte("ADB custom key fingerprint, not an X.509 issuer")})
	tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{client.certificate}, MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAnyClientCert, ClientCAs: pool})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return err
	}
	if scenario == "reject-key" {
		return nil
	}
	features := "device::features=shell_v2\x00"
	if scenario == "no-shell-v2" {
		features = "device::features=not_shell_v2\x00"
	}
	if err := writePacket(tlsConn, packet{Command: commandConnect, Arg0: protocolVersion, Arg1: maxPayload, Data: []byte(features)}, maxPayload); err != nil {
		return err
	}
	if scenario == "no-shell-v2" {
		return nil
	}
	p, err = readPacket(tlsConn)
	if err != nil {
		return err
	}
	if p.Command != commandOpen || string(p.Data) != "shell,v2,raw:id -u\x00" {
		return errors.New("wrong shell request")
	}
	if scenario == "cancel" {
		cancel()
		return nil
	}
	if scenario == "write-before-open" {
		return writePacket(tlsConn, packet{Command: commandWrite, Arg0: 42, Arg1: 1}, maxPayload)
	}
	if err := writePacket(tlsConn, packet{Command: commandOkay, Arg0: 42, Arg1: 1}, maxPayload); err != nil {
		return err
	}
	remoteID := uint32(42)
	if scenario == "wrong-stream" {
		remoteID++
	}
	frames := append(shellFrame(1, []byte("2000\n")), shellFrame(3, []byte{0})...)
	if err := writePacket(tlsConn, packet{Command: commandWrite, Arg0: remoteID, Arg1: 1, Data: frames}, maxPayload); err != nil {
		return err
	}
	if scenario == "wrong-stream" {
		return nil
	}
	p, err = readPacket(tlsConn)
	if err != nil {
		return err
	}
	if p.Command != commandOkay || p.Arg0 != 1 || p.Arg1 != 42 {
		return errors.New("wrong acknowledgement")
	}
	if err := writePacket(tlsConn, packet{Command: commandClose, Arg0: 42, Arg1: 1}, maxPayload); err != nil {
		return err
	}
	p, err = readPacket(tlsConn)
	if err != nil {
		return err
	}
	if p.Command != commandClose || p.Arg0 != 1 || p.Arg1 != 42 {
		return errors.New("wrong close")
	}
	return nil
}
