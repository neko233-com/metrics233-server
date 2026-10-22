// Package telemetry owns the small wire contracts used by remote collectors.
// It must remain independent of the control-plane store, Grafana and web UI.
package telemetry

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var metricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]{0,199}$`)
var labelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,99}$`)

type Sample struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels,omitempty"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp,omitempty"` // Unix milliseconds; zero means now.
}

func ValidateLabels(labels map[string]string) error {
	if len(labels) > 32 {
		return errors.New("at most 32 labels per series")
	}
	for k, v := range labels {
		if !labelName.MatchString(k) || k == "__name__" || len(v) > 512 {
			return fmt.Errorf("invalid label %q (value limit: 512 bytes)", k)
		}
	}
	return nil
}

func (s Sample) Validate() error {
	if !metricName.MatchString(s.Name) {
		return fmt.Errorf("invalid metric name %q", s.Name)
	}
	if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
		return errors.New("sample value must be finite")
	}
	if s.Timestamp < 0 || s.Timestamp > time.Now().Add(5*time.Minute).UnixMilli() {
		return errors.New("timestamp must be Unix milliseconds, not more than 5 minutes in the future")
	}
	return ValidateLabels(s.Labels)
}

type Target struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	URL             string            `json:"url"`
	IntervalSeconds int               `json:"interval_seconds"`
	Labels          map[string]string `json:"labels"`
	Enabled         bool              `json:"enabled"`
	LastScrape      int64             `json:"last_scrape"`
	LastError       string            `json:"last_error"`
	Samples         int               `json:"samples"`
	DurationMS      int64             `json:"duration_ms"`
	Kind            string            `json:"kind,omitempty"`
	Username        string            `json:"username,omitempty"`
	Database        string            `json:"database,omitempty"`
	TLSMode         string            `json:"tls_mode,omitempty"`
	SecureSettings  map[string]string `json:"secure_settings,omitempty"`
	SecureFields    map[string]bool   `json:"secure_fields,omitempty"`
}

func (t Target) Validate() error {
	u, err := url.Parse(t.URL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(t.URL) > 2048 {
		return errors.New("invalid collector URL; credentials belong in secure_settings")
	}
	scheme := "http"
	switch t.Kind {
	case "", "prometheus", "clickhouse", "elasticsearch", "hadoop", "hdfs", "hive", "kafka", "spark", "flink":
		if u.Scheme != "http" && u.Scheme != "https" {
			return errors.New("HTTP collector URL must use http or https")
		}
	case "mysql", "postgresql":
		scheme = t.Kind
		if u.Scheme != scheme || u.RawQuery != "" || u.Path != "" {
			return fmt.Errorf("%s URL must be %s://host:port; use database and tls_mode fields", scheme, scheme)
		}
	case "redis":
		if (u.Scheme != "redis" && u.Scheme != "rediss") || u.RawQuery != "" || u.Path != "" {
			return errors.New("Redis URL must be redis(s)://host:port")
		}
	default:
		return errors.New("unknown collector kind")
	}
	if t.TLSMode != "" && t.TLSMode != "disable" && t.TLSMode != "require" && t.TLSMode != "verify-full" {
		return errors.New("tls_mode must be disable, require or verify-full")
	}
	if t.TLSMode != "" && t.Kind != "mysql" && t.Kind != "postgresql" {
		return errors.New("tls_mode is only supported by SQL collectors; Redis uses rediss and HTTP uses https")
	}
	if len(t.Username) > 256 || len(t.Database) > 256 {
		return errors.New("username and database limit: 256 bytes")
	}
	for key, value := range t.SecureSettings {
		if (key != "password" && key != "bearer_token") || len(value) > 8192 {
			return errors.New("secure_settings supports password and bearer_token, at most 8192 bytes each")
		}
	}
	if len(t.Name) == 0 || len(t.Name) > 100 {
		return errors.New("name must contain 1–100 bytes")
	}
	if t.IntervalSeconds < 5 || t.IntervalSeconds > 86400 {
		return errors.New("interval_seconds must be between 5 and 86400")
	}
	return ValidateLabels(t.Labels)
}

// ManagedHost is control-plane metadata. Credentials are always write-only.
type ManagedHost struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Address        string            `json:"address"`
	Port           int               `json:"port"`
	User           string            `json:"user"`
	HostKey        string            `json:"host_key"`
	Sudo           bool              `json:"sudo"`
	Enabled        bool              `json:"enabled"`
	Revoked        bool              `json:"revoked"`
	Version        int64             `json:"version"`
	Target         Target            `json:"target"`
	SecureSettings map[string]string `json:"secure_settings,omitempty"`
	SecureFields   map[string]bool   `json:"secure_fields,omitempty"`
	LastSeen       int64             `json:"last_seen"`
	AgentVersion   string            `json:"agent_version"`
	AgentState     string            `json:"agent_state"`
	AgentOS        string            `json:"agent_os"`
	AgentArch      string            `json:"agent_arch"`
	AgentSHA       string            `json:"agent_sha"`
	AppliedVersion int64             `json:"applied_version"`
	Samples        int               `json:"samples"`
	CollectHost    *bool             `json:"collect_host,omitempty"`
	Resources      *HostResources    `json:"resources,omitempty"`
}

var managedUser = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,63}$`)
var managedDNS = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,252}$`)
var managedFingerprint = regexp.MustCompile(`^SHA256:[a-zA-Z0-9+/]{43}$`)

func (h ManagedHost) Validate() error {
	if strings.TrimSpace(h.Name) == "" || len(h.Name) > 100 {
		return errors.New("host name must contain 1–100 bytes")
	}
	if net.ParseIP(h.Address) == nil && !managedDNS.MatchString(h.Address) {
		return errors.New("host address must be a literal IP or DNS name")
	}
	if h.Port < 1 || h.Port > 65535 || !managedUser.MatchString(h.User) {
		return errors.New("invalid SSH port or user")
	}
	if h.HostKey != "" && !managedFingerprint.MatchString(h.HostKey) {
		return errors.New("host_key must be an SHA256 SSH fingerprint")
	}
	if h.Version < 0 {
		return errors.New("invalid host version")
	}
	if h.Target.SecureSettings != nil || h.Target.ID != 0 {
		return errors.New("collector credentials belong in host secure_settings")
	}
	if err := h.Target.Validate(); err != nil {
		return err
	}
	if len(h.Target.Labels) > 28 {
		return errors.New("managed collectors allow at most 28 configured labels")
	}
	for k, v := range h.SecureSettings {
		if (k != "ssh_password" && k != "ssh_private_key" && k != "collector_password" && k != "collector_bearer_token") || len(v) > 16384 {
			return errors.New("unsupported or oversized host secret")
		}
		if strings.HasPrefix(k, "collector_") && len(v) > 8192 {
			return errors.New("collector secret exceeds 8192 bytes")
		}
	}
	return nil
}

type HostAgentConfig struct {
	Version     int64  `json:"version"`
	Enabled     bool   `json:"enabled"`
	Target      Target `json:"target"`
	CollectHost bool   `json:"collect_host"`
}

type HostHeartbeat struct {
	Version        string   `json:"version"`
	OS             string   `json:"os"`
	Arch           string   `json:"arch"`
	SHA            string   `json:"sha"`
	State          string   `json:"state"`
	AppliedVersion int64    `json:"applied_version"`
	Samples        []Sample `json:"samples"`
}

type HostBootstrap struct {
	Server string `json:"server"`
	HostID string `json:"host_id"`
	Token  string `json:"token"`
}
