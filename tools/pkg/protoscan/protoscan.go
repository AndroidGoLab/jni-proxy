// Package protoscan reads compiled proto Go stubs to resolve actual
// Go names for services, RPCs, and messages. Protoc-gen-go may rename
// identifiers (e.g., P2p → P2P, A2dp → A2Dp) relative to the proto
// source names. This package bridges that gap.
package protoscan

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	newClientRe = regexp.MustCompile(`^func New(\w+Client)\(`)
	// Match Go interface method declarations: MethodName(ctx context.Context, ...
	goMethodRe = regexp.MustCompile(`^\t(\w+)\(ctx context\.Context,`)
	// Match Go message struct declarations: type FooRequest struct {
	msgTypeRe = regexp.MustCompile(`^type (\w+) struct \{`)
)

// GoNames holds the resolved Go identifiers for a proto package.
// Each map stores both the exact name and a lowercased key for
// case-insensitive fallback. Exact matches take priority to avoid
// collisions when two names differ only in casing (e.g., A2dp vs A2DP).
type GoNames struct {
	// ServiceClients maps service name → actual Go service name.
	ServiceClients map[string]string

	// RPCMethods maps RPC name → actual Go RPC name.
	RPCMethods map[string]string

	// MessageTypes maps message name → actual Go message type name.
	MessageTypes map[string]string
}

// Scan reads the *_grpc.pb.go files in a proto package directory and
// extracts the actual Go names for services and RPC methods.
func Scan(protoPackageDir string) GoNames {
	names := GoNames{
		ServiceClients: make(map[string]string),
		RPCMethods:     make(map[string]string),
		MessageTypes:   make(map[string]string),
	}

	// Scan both _grpc.pb.go (for service/RPC names) and .pb.go (for message names).
	matches, _ := filepath.Glob(filepath.Join(protoPackageDir, "*.pb.go"))
	for _, path := range matches {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()

			// Match client constructors: func NewXxxClient(
			if m := newClientRe.FindStringSubmatch(line); m != nil {
				goName := strings.TrimSuffix(m[1], "Client")
				names.ServiceClients[goName] = goName
				names.ServiceClients[strings.ToLower(goName)] = goName
			}

			// Skip wire names from FullMethodName constants — they are
			// proto source names, not Go names. Using them causes
			// exact-match collisions (e.g., IsBluetoothA2dpOn vs
			// the Go name IsBluetoothA2DpOn).

			// Match Go interface method declarations (actual Go names).
			// These are the authoritative names — protoc-gen-go may
			// rename (e.g., A2dp → A2Dp). Store with both the exact
			// Go name and the lowercase key for lookup.
			if m := goMethodRe.FindStringSubmatch(line); m != nil {
				goMethod := m[1]
				names.RPCMethods[goMethod] = goMethod
				names.RPCMethods[strings.ToLower(goMethod)] = goMethod
			}

			// Match Go message struct type declarations.
			if m := msgTypeRe.FindStringSubmatch(line); m != nil {
				goType := m[1]
				names.MessageTypes[goType] = goType
				names.MessageTypes[strings.ToLower(goType)] = goType
			}
		}
		// Check for scanner I/O errors; silently swallowed errors could
		// cause incomplete name resolution and broken generated code.
		if scanErr := scanner.Err(); scanErr != nil {
			_ = f.Close()
			continue
		}
		_ = f.Close()
	}

	return names
}

// resolve tries exact match, case-insensitive, and proto-CamelCase conversion.
func resolve(m map[string]string, name string) string {
	if resolved, ok := m[name]; ok {
		return resolved
	}
	if resolved, ok := m[strings.ToLower(name)]; ok {
		return resolved
	}
	// Try converting proto source name (with underscores) to Go CamelCase.
	cc := protocCamelCase(name)
	if resolved, ok := m[cc]; ok {
		return resolved
	}
	if resolved, ok := m[strings.ToLower(cc)]; ok {
		return resolved
	}
	return name
}

// protocCamelCase converts a proto-style name to the CamelCase that
// protoc-gen-go uses. This replicates the GoCamelCase algorithm from
// google.golang.org/protobuf/internal/strs.
func protocCamelCase(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' && (i == 0 || s[i-1] == '.'):
			b = append(b, 'X')
		case c == '_' && i+1 < len(s) && isASCIILower(s[i+1]):
			// Skip underscore before lowercase.
		case c >= '0' && c <= '9':
			b = append(b, c)
		default:
			if isASCIILower(c) {
				c -= 'a' - 'A'
			}
			b = append(b, c)
			for i+1 < len(s) && isASCIILower(s[i+1]) {
				i++
				b = append(b, s[i])
			}
		}
	}
	return string(b)
}

func isASCIILower(c byte) bool {
	return 'a' <= c && c <= 'z'
}

// ResolveService returns the actual Go service name.
func (n GoNames) ResolveService(protoServiceName string) string {
	return resolve(n.ServiceClients, protoServiceName)
}

// ResolveRPC returns the actual Go RPC method name.
func (n GoNames) ResolveRPC(rpcName string) string {
	return resolve(n.RPCMethods, rpcName)
}

// ResolveMessage returns the actual Go message type name.
func (n GoNames) ResolveMessage(msgName string) string {
	return resolve(n.MessageTypes, msgName)
}
