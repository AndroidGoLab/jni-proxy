package acl

import "time"

// RequestInfo describes a pending (or resolved) access request.
type RequestInfo struct {
	ID          int64
	ClientID    string
	Methods     []string
	RequestedAt time.Time
	Status      string
}
