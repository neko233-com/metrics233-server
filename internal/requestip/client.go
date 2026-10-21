// Package requestip resolves a client address at the configured proxy boundary.
package requestip

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP accepts forwarding fields only from a trusted socket peer, then
// selects the rightmost untrusted hop across all fields. Its result still needs
// validation before it is used as a persistent access-control identity.
func ClientIP(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !isTrusted(host, trusted) {
		return host
	}
	headers := r.Header.Values("X-Forwarded-For")
	for h := len(headers) - 1; h >= 0; h-- {
		parts := strings.Split(headers[h], ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			if net.ParseIP(candidate) != nil && !isTrusted(candidate, trusted) {
				return candidate
			}
		}
	}
	return host
}

func isTrusted(value string, trusted []*net.IPNet) bool {
	ip := net.ParseIP(value)
	if ip == nil {
		return false
	}
	for _, network := range trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
