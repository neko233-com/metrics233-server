// Package hostagent implements an outbound-only, single-host collector. The
// control server can change collection settings or pause it, never run shell.
package hostagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neko233-com/metrics233-server/internal/collector"
	model "github.com/neko233-com/metrics233-server/internal/telemetry"
)

const MaxConfigBytes = 128 * 1024

func ValidateBootstrap(cfg model.HostBootstrap) error {
	u, err := url.Parse(cfg.Server)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("invalid control server origin")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return errors.New("control server requires HTTPS or a literal loopback HTTP address")
	}
	if _, err := uuid.Parse(cfg.HostID); err != nil {
		return errors.New("invalid host identity")
	}
	if len(cfg.Token) != 43 || strings.ContainsAny(cfg.Token, "\r\n\x00 ") {
		return errors.New("invalid host credential")
	}
	return nil
}

func ReadBootstrap(path string) (model.HostBootstrap, error) {
	var cfg model.HostBootstrap
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return cfg, errors.New("bootstrap must be a protected regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return cfg, errors.New("cannot open protected bootstrap")
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return cfg, errors.New("protected bootstrap changed while opening")
	}
	dec := json.NewDecoder(io.LimitReader(f, 16385))
	dec.DisallowUnknownFields()
	if dec.Decode(&cfg) != nil || dec.Decode(new(any)) != io.EOF {
		return cfg, errors.New("invalid protected bootstrap document")
	}
	return cfg, ValidateBootstrap(cfg)
}

func NewClient() *http.Client {
	// An inherited development proxy must never receive host credentials.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func Exchange(ctx context.Context, client *http.Client, cfg model.HostBootstrap, method, suffix string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return errors.New("cannot encode agent report")
		}
		body = bytes.NewReader(encoded)
	}
	r, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(cfg.Server, "/")+"/agent/v1/hosts/"+cfg.HostID+suffix, body)
	if err != nil {
		return errors.New("invalid agent request")
	}
	r.Header.Set("Authorization", "Bearer "+cfg.Token)
	r.Header.Set("Content-Type", "application/json")
	response, err := client.Do(r)
	if err != nil {
		return errors.New("control server unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("control server rejected agent request")
	}
	limit := MaxConfigBytes
	if output == nil {
		limit = 4096
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit+1)))
	if err != nil || len(data) > limit {
		return errors.New("agent response budget exceeded")
	}
	if output != nil && json.Unmarshal(data, output) != nil {
		return errors.New("invalid control server response")
	}
	return nil
}

func executableSHA() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}

type Agent struct {
	Bootstrap  model.HostBootstrap
	Client     *http.Client
	Version    string
	SHA        string
	nextScrape time.Time
	revision   int64
	lastState  string
}

func New(cfg model.HostBootstrap, version string) (*Agent, error) {
	if err := ValidateBootstrap(cfg); err != nil {
		return nil, err
	}
	return &Agent{Bootstrap: cfg, Client: NewClient(), Version: version, SHA: executableSHA()}, nil
}

// Step never retries a write with an uncertain result. The next cycle collects
// fresh gauge/counter observations; it never replays a previous interval.
func (a *Agent) Step(ctx context.Context) (time.Duration, string, error) {
	var cfg model.HostAgentConfig
	if err := Exchange(ctx, a.Client, a.Bootstrap, "GET", "/configuration", nil, &cfg); err != nil {
		return 15 * time.Second, "disconnected", err
	}
	interval := time.Duration(cfg.Target.IntervalSeconds) * time.Second
	if interval < 5*time.Second || interval > 24*time.Hour || cfg.Version < 1 {
		return 30 * time.Second, "invalid_config", errors.New("invalid collection configuration")
	}
	report := model.HostHeartbeat{Version: a.Version, OS: runtime.GOOS, Arch: runtime.GOARCH, SHA: a.SHA, AppliedVersion: cfg.Version, State: "paused"}
	if cfg.Version != a.revision {
		a.nextScrape = time.Time{}
		a.revision = cfg.Version
	}
	if cfg.Enabled && !time.Now().Before(a.nextScrape) {
		started := time.Now()
		secrets := cfg.Target.SecureSettings
		cfg.Target.SecureSettings = nil
		if err := cfg.Target.Validate(); err != nil {
			return interval, "invalid_config", errors.New("invalid collection configuration")
		}
		samples, err := collector.CollectOnce(ctx, cfg.Target, secrets)
		up := 1.0
		report.State = "collecting"
		if err != nil {
			up = 0
			report.State = "collection_error"
			samples = nil
		}
		labels := map[string]string{"job": cfg.Target.Name, "instance": cfg.Target.URL}
		for k, v := range cfg.Target.Labels {
			labels[k] = v
		}
		samples = append(samples, model.Sample{Name: "up", Value: up, Labels: labels}, model.Sample{Name: "scrape_duration_seconds", Value: time.Since(started).Seconds(), Labels: labels}, model.Sample{Name: "scrape_samples_scraped", Value: float64(len(samples)), Labels: labels})
		if up == 0 && cfg.Target.Kind == "mysql" {
			samples = append(samples, model.Sample{Name: "mysql_up", Value: 0, Labels: labels})
		}
		report.Samples = samples
		a.nextScrape = started.Add(interval)
		a.lastState = report.State
	} else if cfg.Enabled {
		report.State = a.lastState
		if report.State == "" {
			report.State = "collecting"
		}
	}
	if err := Exchange(ctx, a.Client, a.Bootstrap, "POST", "/heartbeat", report, nil); err != nil {
		return interval, "disconnected", err
	}
	return min(interval, 30*time.Second), report.State, nil
}

func (a *Agent) Run(ctx context.Context, once bool, status func(string)) error {
	previous := ""
	failures := 0
	for {
		started := time.Now()
		delay, state, err := a.Step(ctx)
		if state != previous {
			status(state)
			previous = state
		}
		if once {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			failures = min(failures+1, 4)
			delay = time.Duration(5<<failures) * time.Second
		} else {
			failures = 0
		}
		wait := max(time.Second, delay-time.Since(started))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
