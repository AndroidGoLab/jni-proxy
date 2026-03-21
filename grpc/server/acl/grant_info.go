package acl

import "time"

// GrantInfo describes a method-access grant for a client.
type GrantInfo struct {
	ClientID      string
	MethodPattern string
	GrantedAt     time.Time
	GrantedBy     string
}
