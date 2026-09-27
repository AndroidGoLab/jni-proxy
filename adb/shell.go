// Package adb runs bounded shell-v2 commands through an already paired local adbd.
package adb

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"
)

// Wire versions, commands and bounds define the supported modern ADB subset.
//
// Example: advertising maxPayload limits incoming WRTE allocations to 64 KiB.
const (
	protocolVersion = 0x01000001
	tlsVersion      = 0x01000000
	maxPayload      = 64 * 1024
	maxOutput       = 64 * 1024
	maxPackets      = 4096
	commandConnect  = "CNXN"
	commandTLS      = "STLS"
	commandOpen     = "OPEN"
	commandOkay     = "OKAY"
	commandWrite    = "WRTE"
	commandClose    = "CLSE"
)

// Protocol and authentication errors let callers distinguish permanent failures from socket errors.
//
// Example: a revoked pairing is exposed as ErrAuthentication rather than retried indefinitely.
var (
	ErrProtocol       = errors.New("unsupported or invalid ADB protocol")
	ErrAuthentication = errors.New("ADB authentication failed; check wireless debugging and pairing")
)

// Port identifies the current local adbd TLS listening port, obtained through discovery.
//
// Example: Port(37001) always addresses 127.0.0.1, never a remote host.
type Port int

// Result records the last command's bounded output and shell exit status.
//
// Example: ExitCode zero means the shell command reported success.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ExitError reports a completed command with a nonzero exit code.
//
// Example: unsupported fixed-performance hardware may return exit 255.
type ExitError struct{ Code int }

// Error describes the nonzero exit without including command output or credentials.
//
// Example: a caller can separately inspect the bounded Result.Stderr.
func (e *ExitError) Error() string { return fmt.Sprintf("ADB shell exited with status %d", e.Code) }

// Unwrap identifies command failure as distinct from a transport failure.
//
// Example: errors.As retrieves ExitError without suggesting that authentication failed.
func (e *ExitError) Unwrap() error { return nil }

// Client holds one immutable, already paired RSA identity.
//
// Example: NewClient reads a dedicated SDK-generated adbkey.
type Client struct{ certificate tls.Certificate }

// NewClient validates a single PKCS1 or PKCS8 RSA private key and creates its ADB certificate.
//
// Example: NewClient(keyPEM) accepts the output of adb keygen; it does not pair keys.
func NewClient(keyPEM []byte) (*Client, error) {
	if len(keyPEM) > 16*1024 {
		return nil, errors.New("ADB private key exceeds 16 KiB")
	}
	block, rest := pem.Decode(keyPEM)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 || len(block.Headers) != 0 {
		return nil, errors.New("expected one unencrypted PEM private key")
	}
	var key *rsa.PrivateKey
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			var ok bool
			key, ok = parsed.(*rsa.PrivateKey)
			if !ok {
				return nil, errors.New("ADB requires an RSA private key")
			}
		}
	default:
		return nil, errors.New("expected PKCS1 or PKCS8 RSA private key")
	}
	if err != nil {
		return nil, errors.New("invalid RSA private key")
	}
	if key.N.BitLen() != 2048 || key.E != 65537 {
		return nil, errors.New("ADB requires a 2048-bit RSA key with exponent 65537")
	}
	if err := key.Validate(); err != nil {
		return nil, errors.New("invalid RSA private key")
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "adb"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create ADB certificate: %w", err)
	}
	return &Client{certificate: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}}, nil
}

// clientCertificate bypasses normal issuer selection because adbd encodes key fingerprints as issuers.
//
// Example: the already paired RSA identity is presented even with adbd's custom issuer list.
func (c *Client) clientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	return &c.certificate, nil
}

// Shell executes one command using TLS 1.3 and shell-v2 on literal IPv4 loopback.
//
// Example: Shell(ctx, discoveredPort, "id -u") returns the authenticated shell's result.
func (c *Client) Shell(
	ctx context.Context,
	port Port,
	command string,
) (_result Result, _err error) {
	if port < 1 || port > 65535 {
		return Result{}, errors.New("invalid local ADB port")
	}
	if command == "" || strings.ContainsRune(command, 0) || len(command) > 4096 {
		return Result{}, errors.New("invalid shell command")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))))
	if err != nil {
		return Result{}, fmt.Errorf("connect local ADB: %w", err)
	}
	stop := context.AfterFunc(ctx,
		// Closing the connection interrupts packet I/O after the TLS handshake too.
		func() { _ = conn.Close() })
	defer stop()
	defer func() {
		closeErr := conn.Close()
		if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			_err = errors.Join(_err, closeErr)
		}
		if ctx.Err() != nil {
			_err = ctx.Err()
		}
	}()
	if err := writePacket(conn, packet{Command: commandConnect, Arg0: protocolVersion, Arg1: maxPayload, Data: []byte("host::features=shell_v2\x00")}, maxPayload); err != nil {
		return Result{}, err
	}
	p, err := readPacket(conn)
	if err != nil {
		return Result{}, fmt.Errorf("read STLS: %w", err)
	}
	if p.Command != commandTLS || p.Arg0 != tlsVersion || p.Arg1 != 0 || len(p.Data) != 0 {
		return Result{}, fmt.Errorf("%w: expected STLS", ErrProtocol)
	}
	if err := writePacket(conn, packet{Command: commandTLS, Arg0: tlsVersion}, maxPayload); err != nil {
		return Result{}, err
	}
	// ADB uses a self-signed server certificate, as does the SDK client. The
	// caller cannot choose a host: only this phone's loopback port is reachable.
	tlsConn := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true, GetClientCertificate: c.clientCertificate})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return Result{}, fmt.Errorf("%w: TLS handshake: %w", ErrAuthentication, err)
	}
	p, err = readPacket(tlsConn)
	if err != nil {
		return Result{}, fmt.Errorf("%w: no authenticated CNXN: %w", ErrAuthentication, err)
	}
	if p.Command != commandConnect || p.Arg0 != protocolVersion || p.Arg1 < 4096 || !hasShellV2(p.Data) {
		return Result{}, fmt.Errorf("%w: expected modern shell-v2 CNXN", ErrProtocol)
	}
	peerLimit := min(p.Arg1, uint32(maxPayload))
	return runShell(tlsConn, peerLimit, command)
}

// hasShellV2 checks a complete feature name in the negotiated banner.
//
// Example: not_shell_v2 does not satisfy the shell_v2 requirement.
func hasShellV2(banner []byte) bool {
	_, properties, ok := strings.Cut(strings.TrimRight(string(banner), "\x00"), "::")
	if !ok {
		return false
	}
	for _, property := range strings.Split(properties, ";") {
		features, ok := strings.CutPrefix(property, "features=")
		if !ok {
			continue
		}
		for _, feature := range strings.Split(features, ",") {
			if feature == "shell_v2" {
				return true
			}
		}
	}
	return false
}

// packet is one bounded ADB transport message.
//
// Example: WRTE carries fragments of the one opened shell stream.
type packet struct {
	Command    string
	Arg0, Arg1 uint32
	Data       []byte
}

// readPacket validates the header and checksum before exposing a bounded payload.
//
// Example: checksum zero is valid for negotiated modern ADB messages.
func readPacket(r io.Reader) (packet, error) {
	var header [24]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return packet{}, err
	}
	command := binary.LittleEndian.Uint32(header[:4])
	length := binary.LittleEndian.Uint32(header[12:16])
	if command^binary.LittleEndian.Uint32(header[20:24]) != 0xffffffff || length > maxPayload {
		return packet{}, fmt.Errorf("%w: invalid packet header", ErrProtocol)
	}
	p := packet{Command: string(header[:4]), Arg0: binary.LittleEndian.Uint32(header[4:8]), Arg1: binary.LittleEndian.Uint32(header[8:12]), Data: make([]byte, length)}
	if _, err := io.ReadFull(r, p.Data); err != nil {
		return packet{}, err
	}
	checksum := binary.LittleEndian.Uint32(header[16:20])
	var actual uint32
	for _, value := range p.Data {
		actual += uint32(value)
	}
	if checksum != 0 && checksum != actual {
		return packet{}, fmt.Errorf("%w: packet checksum", ErrProtocol)
	}
	return p, nil
}

// writePacket serializes one message within the negotiated peer receive bound.
//
// Example: OPEN contains a null-terminated shell-v2 service string.
func writePacket(
	w io.Writer,
	p packet,
	limit uint32,
) error {
	if len(p.Command) != 4 || uint64(len(p.Data)) > uint64(limit) {
		return fmt.Errorf("%w: outgoing packet bound", ErrProtocol)
	}
	var header [24]byte
	copy(header[:4], p.Command)
	binary.LittleEndian.PutUint32(header[4:8], p.Arg0)
	binary.LittleEndian.PutUint32(header[8:12], p.Arg1)
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(p.Data)))
	var checksum uint32
	for _, value := range p.Data {
		checksum += uint32(value)
	}
	binary.LittleEndian.PutUint32(header[16:20], checksum)
	binary.LittleEndian.PutUint32(header[20:24], binary.LittleEndian.Uint32(header[:4])^0xffffffff)
	_, err := io.Copy(w, io.MultiReader(bytes.NewReader(header[:]), bytes.NewReader(p.Data)))
	return err
}

// runShell enforces OPEN/OKAY, both stream IDs, bounded WRTE traffic, and the closing exchange.
//
// Example: WRTE before the opening OKAY cannot become accepted command output.
func runShell(
	conn io.ReadWriter,
	peerLimit uint32,
	command string,
) (Result, error) {
	if err := writePacket(conn, packet{Command: commandOpen, Arg0: 1, Data: []byte("shell,v2,raw:" + command + "\x00")}, peerLimit); err != nil {
		return Result{}, err
	}
	p, err := readPacket(conn)
	if err != nil {
		return Result{}, err
	}
	if p.Command != commandOkay || p.Arg0 == 0 || p.Arg1 != 1 || len(p.Data) != 0 {
		return Result{}, fmt.Errorf("%w: expected opening OKAY", ErrProtocol)
	}
	remoteID := p.Arg0
	var stream shellStream
	for range maxPackets {
		p, err := readPacket(conn)
		if err != nil {
			return Result{}, fmt.Errorf("shell stream: %w", err)
		}
		if p.Arg0 != remoteID || p.Arg1 != 1 {
			return Result{}, fmt.Errorf("%w: unexpected stream IDs", ErrProtocol)
		}
		switch p.Command {
		case commandWrite:
			if err := stream.feed(p.Data); err != nil {
				return Result{}, err
			}
			if err := writePacket(conn, packet{Command: commandOkay, Arg0: 1, Arg1: remoteID}, peerLimit); err != nil {
				return Result{}, err
			}
		case commandClose:
			if len(p.Data) != 0 {
				return Result{}, fmt.Errorf("%w: CLSE payload", ErrProtocol)
			}
			if err := writePacket(conn, packet{Command: commandClose, Arg0: 1, Arg1: remoteID}, peerLimit); err != nil {
				return Result{}, err
			}
			return stream.finish()
		default:
			return Result{}, fmt.Errorf("%w: unexpected shell packet %q", ErrProtocol, p.Command)
		}
	}
	return Result{}, fmt.Errorf("%w: shell packet count limit", ErrProtocol)
}

// shellStream accumulates fragmented shell-v2 frames and a single terminal exit.
//
// Example: stdout and stderr share the same cumulative output budget.
type shellStream struct {
	pending []byte
	result  Result
	exited  bool
	total   int
}

// feed consumes complete shell frames while retaining at most one incomplete frame.
//
// Example: a two-byte partial header is completed by the next WRTE packet.
func (s *shellStream) feed(data []byte) error {
	if len(s.pending)+len(data) > maxOutput+maxPayload+5 {
		return fmt.Errorf("%w: shell buffer bound", ErrProtocol)
	}
	s.pending = append(s.pending, data...)
	for len(s.pending) >= 5 {
		length := binary.LittleEndian.Uint32(s.pending[1:5])
		if length > maxOutput || s.exited {
			return fmt.Errorf("%w: oversized or post-exit shell frame", ErrProtocol)
		}
		if len(s.pending)-5 < int(length) {
			return nil
		}
		payload := s.pending[5 : 5+int(length)]
		switch s.pending[0] {
		case 1, 2:
			s.total += len(payload)
			if s.total > maxOutput {
				return fmt.Errorf("%w: cumulative shell output bound", ErrProtocol)
			}
			if s.pending[0] == 1 {
				s.result.Stdout += string(payload)
			} else {
				s.result.Stderr += string(payload)
			}
		case 3:
			if len(payload) != 1 {
				return fmt.Errorf("%w: invalid shell exit", ErrProtocol)
			}
			s.result.ExitCode = int(payload[0])
			s.exited = true
		default:
			return fmt.Errorf("%w: unsupported shell frame", ErrProtocol)
		}
		s.pending = s.pending[5+int(length):]
	}
	return nil
}

// finish requires a complete stream and reports nonzero exit distinctly from socket errors.
//
// Example: EOF or CLSE without an exit frame is never success.
func (s *shellStream) finish() (Result, error) {
	if !s.exited || len(s.pending) != 0 {
		return Result{}, fmt.Errorf("%w: missing exit or incomplete shell frame", ErrProtocol)
	}
	if s.result.ExitCode != 0 {
		return s.result, &ExitError{Code: s.result.ExitCode}
	}
	return s.result, nil
}
