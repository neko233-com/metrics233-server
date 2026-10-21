package server

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// ValidateIngestTokenConfig validates environment-only startup configuration.
// A reporting token must never leave management APIs in loopback's unauthenticated
// development mode. Diagnostics contain names and constraints, never values.
func ValidateIngestTokenConfig(authMode, managementToken, ingestToken string) error {
	if ingestToken == "" {
		return nil
	}
	if authMode != "token" {
		return errors.New("METRICSPANEL_INGEST_TOKEN requires --auth=token; local authentication is not supported")
	}
	if len(managementToken) < 16 {
		return errors.New("METRICSPANEL_INGEST_TOKEN requires a separate METRICSPANEL_TOKEN of at least 16 characters")
	}
	if len(ingestToken) < 32 || len(ingestToken) > 8192 {
		return errors.New("METRICSPANEL_INGEST_TOKEN must contain 32–8192 printable ASCII characters without whitespace")
	}
	for _, character := range []byte(ingestToken) {
		if character < 0x21 || character > 0x7e {
			return errors.New("METRICSPANEL_INGEST_TOKEN must contain 32–8192 printable ASCII characters without whitespace")
		}
	}
	if subtle.ConstantTimeCompare([]byte(managementToken), []byte(ingestToken)) == 1 {
		return errors.New("METRICSPANEL_INGEST_TOKEN must differ from METRICSPANEL_TOKEN")
	}
	return nil
}

// ConfigureIngestToken is an immutable startup step, before Handler is built.
func (s *Server) ConfigureIngestToken(authMode, token string) error {
	if err := ValidateIngestTokenConfig(authMode, s.Token, token); err != nil {
		return err
	}
	if token != "" && s.Authorizer != nil {
		return errors.New("ingest-only token cannot bypass a configured authorization provider")
	}
	s.ingestToken = token
	return nil
}

func exactMetricIngestRequest(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/v1/ingest", "/api/v1/write", "/prometheus/api/v1/write", "/v1/metrics", "/otlp/v1/metrics":
		return true
	}
	return false
}

// Values from query strings, cookies, Basic auth or an unprefixed header never
// count as this credential. Mixed/duplicate Authorization fields fail closed.
func (s *Server) ingestOnlyCredential(r *http.Request) (matched, malformed bool) {
	if s.ingestToken == "" {
		return false, false
	}
	values := r.Header.Values("Authorization")
	for _, value := range values {
		if strings.HasPrefix(value, "Bearer ") && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(value, "Bearer ")), []byte(s.ingestToken)) == 1 {
			return true, len(values) != 1
		}
	}
	return false, false
}

func (s *Server) restrictIngestOnlyCredential(w http.ResponseWriter, r *http.Request) bool {
	matched, malformed := s.ingestOnlyCredential(r)
	if !matched {
		return false
	}
	if !malformed && exactMetricIngestRequest(r) {
		return false
	}
	s.secureHeaders(w, r)
	w.Header().Set("Cache-Control", "no-store")
	if malformed {
		fail(w, http.StatusUnauthorized, errors.New("one Bearer authorization field is required"))
	} else {
		fail(w, http.StatusForbidden, errors.New("ingest-only token may only report metrics"))
	}
	return true
}
