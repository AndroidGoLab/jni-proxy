package acl

import "time"

// ClientInfo describes a registered client.
type ClientInfo struct {
	ClientID     string
	CertPEM      string
	Fingerprint  string
	RegisteredAt time.Time
}
