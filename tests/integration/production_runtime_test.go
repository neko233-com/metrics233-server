//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

// This suite deliberately exercises the release runtime rather than the root
// integration image, which contains databases and test orchestration tools.
// Enable it explicitly so the existing database suite keeps its own lifecycle.
func TestDockerProductionRuntime(t *testing.T) {
	if testing.Short() || os.Getenv("METRICSPANEL_DOCKER_RUNTIME_TEST") != "1" {
		t.Skip("use scripts/dockerfile_test.ps1 or METRICSPANEL_DOCKER_RUNTIME_TEST=1")
	}
	_, filename, _, _ := runtime.Caller(0)
	e := &productionRuntime{t: t, root: filepath.Clean(filepath.Join(filepath.Dir(filename), "../..")), client: &http.Client{Timeout: 20 * time.Second}}
	e.token = e.secret()
	e.password = e.secret()
	e.collectorSecret = e.secret()
	e.prefix = "metricspanel-runtime-" + strconv.Itoa(os.Getpid()) + "-" + e.secret()[:8]
	e.image = os.Getenv("METRICSPANEL_RUNTIME_TEST_IMAGE")
	if e.image == "" {
		e.image = e.prefix + ":test"
		t.Cleanup(func() { e.cleanup("image", "rm", e.image) })
		e.mustDocker("build", "--target", "runtime", "--label", "metricspanel.verify="+e.prefix, "-t", e.image, ".")
	}
	var image struct {
		Size   int64
		Config struct {
			User        string
			Healthcheck *json.RawMessage
		}
	}
	require.NoError(t, json.Unmarshal([]byte(e.mustDocker("image", "inspect", e.image, "--format", "{{json .}}")), &image))
	require.NotEmpty(t, image.Config.User)
	require.NotContains(t, []string{"root", "0", "0:0"}, image.Config.User)
	require.NotNil(t, image.Config.Healthcheck)
	require.LessOrEqual(t, image.Size, int64(256<<20), "production image exceeded the 256 MiB runtime budget")
	// Docker's containerd image store can report compressed content through
	// inspect.Size and a larger unpacked local footprint through image ls.
	// Report both instead of presenting the smaller number as disk usage.
	localFootprint := e.mustDocker("image", "ls", "--filter", "reference="+e.image, "--format", "{{.Size}}")
	t.Logf("production image content (inspect.Size): %d bytes (%.1f MiB); Docker local footprint: %s; configured user=%s", image.Size, float64(image.Size)/(1<<20), localFootprint, image.Config.User)
	// A public bind without an explicit workspace secret must fail closed.
	out, err := e.docker("run", "--rm", "--read-only", "--cap-drop=ALL", "--network=none", e.image)
	require.Error(t, err, "container accepted a non-loopback listener without a workspace token")
	require.Contains(t, out, "16")

	for _, mode := range []string{"token", "local"} {
		t.Run(mode, func(t *testing.T) {
			e := *e
			e.t = t
			e.name = e.prefix + "-" + mode
			e.volume = e.name + "-data"
			e.mustDocker("volume", "create", "--label", "metricspanel.verify="+e.prefix, e.volume)
			t.Cleanup(func() { e.cleanup("volume", "rm", e.volume) })
			t.Cleanup(func() { e.cleanup("rm", "-f", "--volumes", e.name) })
			e.start(mode)
			e.verifyConfinement()
			healthBody, _ := e.request("GET", "/api/health", nil, "", 200)
			var health map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(healthBody, &health))
			require.Len(t, health, 3, "public health must not expose internal diagnostics")
			for _, path := range []string{"/api/v1/health", "/api/v1/stats", "/metrics", "/prometheus/api/v1/query?query=up"} {
				e.request("GET", path, nil, "", 401)
			}
			e.request("GET", "/apis/notifications.alerting.grafana.app/v1beta1/namespaces/default/inhibitionrules", nil, "", 401)
			e.request("POST", "/api/v1/ingest", map[string]any{"samples": []any{}}, "", 401)
			e.request("POST", "/v1/metrics", map[string]any{}, "", 401)
			e.request("GET", "/", nil, "", 200)
			if mode == "local" {
				e.setupLocalAccounts()
			} else {
				e.request("GET", "/apis/notifications.alerting.grafana.app/v1beta1/namespaces/default/inhibitionrules", nil, e.bearer(), 200)
				e.mustDocker("exec", e.name, "sh", "-c", "test ! -e /data/identity.db && test ! -e /identity-ignored.db")
			}
			e.verifyBusinessMetrics()
			// Store encrypted collector configuration to prove that the data key
			// and ciphertext survive recovery, not merely a metric-only database.
			var target model.Target
			body, _ := e.request("POST", "/api/v1/targets", model.Target{Name: "backup-secret-probe", URL: "http://127.0.0.1:7333/metrics", IntervalSeconds: 60, Enabled: false, SecureSettings: map[string]string{"bearer_token": e.collectorSecret}}, e.bearer(), 200)
			require.NoError(t, json.Unmarshal(body, &target))
			require.True(t, target.SecureFields["bearer_token"])
			require.False(t, bytes.Contains(body, []byte(e.collectorSecret)), "collector API leaked a credential")
			files := e.mustDocker("exec", e.name, "sh", "-c", "test -s /etc/ssl/certs/ca-certificates.crt && test -s /data/secrets.key && stat -c '%u:%g %a' /data/secrets.key")
			require.Equal(t, "10001:10001 600", files)
			e.mustDocker("restart", "--timeout", "20", e.name)
			e.waitReady()
			e.verifyPersisted(mode)
			e.resources()
			e.verifyBackupRecovery(mode)
			logs := e.mustDocker("logs", e.name)
			for _, secret := range []string{e.token, e.password, e.collectorSecret} {
				require.False(t, strings.Contains(logs, secret), "server logged a credential")
			}
		})
	}
	t.Run("https-proxy-origin", func(t *testing.T) {
		e := *e
		e.t = t
		e.name = e.prefix + "-https-origin"
		e.volume = e.name + "-data"
		e.publicOrigin = "https://metrics.example.test"
		e.mustDocker("volume", "create", "--label", "metricspanel.verify="+e.prefix, e.volume)
		t.Cleanup(func() { e.cleanup("volume", "rm", e.volume) })
		t.Cleanup(func() { e.cleanup("rm", "-f", "--volumes", e.name) })
		e.start("local")
		e.verifyProxyOrigin()
		e.resources()
	})
	if os.Getenv("METRICSPANEL_DOCKER_BROWSER_TEST") == "1" {
		t.Run("browser", func(t *testing.T) {
			e := *e
			e.t = t
			e.name = e.prefix + "-browser"
			e.volume = e.name + "-data"
			e.mustDocker("volume", "create", "--label", "metricspanel.verify="+e.prefix, e.volume)
			t.Cleanup(func() { e.cleanup("volume", "rm", e.volume) })
			t.Cleanup(func() { e.cleanup("rm", "-f", "--volumes", e.name) })
			e.start("local")
			match := regexp.MustCompile(`Setup token: (\S+)`).FindStringSubmatch(e.mustDocker("logs", e.name))
			require.Len(t, match, 2, "browser instance did not provide a setup token")
			e.setupToken = match[1]
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			command := exec.CommandContext(ctx, "node", "scripts/auth-smoke.mjs")
			command.Dir = filepath.Join(e.root, "web")
			command.Env = append(os.Environ(), "METRICSPANEL_EXTERNAL_URL="+e.address, "METRICSPANEL_SETUP_TOKEN="+e.setupToken)
			out, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("browser authentication against release container failed: %v\n%s", err, e.redact(string(out)))
			}
			t.Log(e.redact(strings.TrimSpace(string(out))))
			e.resources()
		})
	}
}

type productionRuntime struct {
	t                                                         *testing.T
	root, prefix, image, name, volume, address, publicOrigin  string
	token, password, collectorSecret, setupToken, adminCookie string
	client                                                    *http.Client
}

func (e *productionRuntime) secret() string {
	e.t.Helper()
	var value [24]byte
	_, err := rand.Read(value[:])
	require.NoError(e.t, err)
	return hex.EncodeToString(value[:])
}

func (e *productionRuntime) redact(value string) string {
	for _, secret := range []string{e.token, e.password, e.collectorSecret, e.setupToken, e.adminCookie, "correct-horse-233", "viewer-password-233"} {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	return regexp.MustCompile(`Setup token: \S+`).ReplaceAllString(value, "Setup token: [redacted]")
}

func (e *productionRuntime) docker(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = e.root
	// Values are inherited by Docker from the child environment, never passed
	// as command arguments or printed in diagnostics.
	command.Env = append(os.Environ(), "METRICSPANEL_TOKEN="+e.token)
	out, err := command.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (e *productionRuntime) mustDocker(args ...string) string {
	e.t.Helper()
	out, err := e.docker(args...)
	if err != nil {
		e.t.Fatalf("docker %s failed: %v\n%s", strings.Join(args, " "), err, e.redact(out))
	}
	return out
}

func (e *productionRuntime) cleanup(args ...string) {
	e.t.Helper()
	if out, err := e.docker(args...); err != nil {
		e.t.Errorf("cleanup docker %s: %v: %s", strings.Join(args, " "), err, e.redact(out))
	}
}

func (e *productionRuntime) start(mode string) {
	args := []string{"run", "-d", "--name", e.name, "--label", "metricspanel.verify=" + e.prefix,
		"--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true", "--pids-limit=128", "--memory=256m", "--cpus=1",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=16m,mode=1777", "--publish", "127.0.0.1::7333", "--mount", "type=volume,src=" + e.volume + ",dst=/data",
		"--env", "METRICSPANEL_TOKEN", "--env", "METRICSPANEL_AUTH=" + mode, "--env", "METRICSPANEL_STORAGE=embedded"}
	if e.publicOrigin != "" {
		args = append(args, "--env", "METRICSPANEL_ROOT_URL="+e.publicOrigin)
	}
	if mode == "token" {
		// Identity options belong to local mode; token mode must not create or
		// open an identity catalog, including on a read-only root filesystem.
		args = append(args, "--env", "METRICSPANEL_IDENTITY_DB=/identity-ignored.db")
	}
	args = append(args, e.image)
	e.mustDocker(args...)
	port := e.mustDocker("port", e.name, "7333/tcp")
	require.True(e.t, strings.HasPrefix(port, "127.0.0.1:"), "test listener is not loopback")
	e.address = "http://" + port
	e.waitReady()
}

func (e *productionRuntime) waitReady() {
	e.t.Helper()
	require.Eventually(e.t, func() bool {
		out, err := e.docker("inspect", e.name, "--format", "{{.State.Health.Status}}")
		return err == nil && out == "healthy"
	}, 60*time.Second, 500*time.Millisecond, "release HEALTHCHECK did not become healthy")
	// Docker Desktop may allocate a different ephemeral published port after a
	// restart. Resolve it again instead of treating the old mapping as durable.
	port := e.mustDocker("port", e.name, "7333/tcp")
	require.True(e.t, strings.HasPrefix(port, "127.0.0.1:"), "test listener is not loopback")
	e.address = "http://" + port
	e.mustDocker("exec", e.name, "metricspanel", "health")
}

func (e *productionRuntime) bearer() string { return "Bearer " + e.token }

func (e *productionRuntime) request(method, path string, body any, auth string, status int) ([]byte, *http.Response) {
	e.t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(e.t, err)
	}
	return e.requestBytes(method, path, payload, "application/json", auth, status)
}

func (e *productionRuntime) requestBytes(method, path string, payload []byte, contentType, auth string, status int) ([]byte, *http.Response) {
	return e.requestHeaders(method, path, payload, contentType, auth, status, nil)
}

func (e *productionRuntime) requestHeaders(method, path string, payload []byte, contentType, auth string, status int, headers map[string]string) ([]byte, *http.Response) {
	e.t.Helper()
	req, err := http.NewRequest(method, e.address+path, bytes.NewReader(payload))
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", contentType)
	if e.publicOrigin != "" {
		public, err := url.Parse(e.publicOrigin)
		require.NoError(e.t, err)
		req.Host = public.Host
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if strings.HasSuffix(path, "/write") {
		req.Header.Set("Content-Encoding", "snappy")
		req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	}
	if strings.HasPrefix(auth, "Bearer ") {
		req.Header.Set("Authorization", auth)
	} else if auth != "" {
		req.Header.Set("Cookie", auth)
	}
	response, err := e.client.Do(req)
	require.NoError(e.t, err, "%s %s", method, path)
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	require.NoError(e.t, err)
	if response.StatusCode != status {
		e.t.Fatalf("%s %s: expected HTTP %d, received %d: %s", method, path, status, response.StatusCode, e.redact(string(data)))
	}
	return data, response
}

func (e *productionRuntime) setupLocalAccounts() {
	match := regexp.MustCompile(`Setup token: (\S+)`).FindStringSubmatch(e.mustDocker("logs", e.name))
	require.Len(e.t, match, 2, "first local start did not provide a setup token")
	e.setupToken = match[1]
	_, response := e.request("POST", "/api/setup", map[string]string{"token": e.setupToken, "login": "runtime-admin", "password": e.password}, "", 200)
	for _, cookie := range response.Cookies() {
		if cookie.HttpOnly {
			require.Equal(e.t, http.SameSiteLaxMode, cookie.SameSite)
			e.adminCookie = cookie.Name + "=" + cookie.Value
		}
	}
	require.True(e.t, e.adminCookie != "", "setup did not establish an HttpOnly session")
	e.request("POST", "/api/setup", map[string]string{"token": e.setupToken}, "", 409)
	e.request("POST", "/login", map[string]string{"user": "runtime-admin", "password": "deliberately-incorrect"}, "", 401)
	for _, role := range []string{"Viewer", "Editor"} {
		login := strings.ToLower(role)
		e.request("POST", "/api/admin/users", map[string]string{"login": login, "password": e.password, "role": role}, e.adminCookie, 201)
		_, response := e.request("POST", "/login", map[string]string{"user": login, "password": e.password}, "", 200)
		var session string
		for _, cookie := range response.Cookies() {
			if cookie.HttpOnly {
				session = cookie.Name + "=" + cookie.Value
			}
		}
		require.True(e.t, session != "", "login did not establish a session")
		e.request("GET", "/api/v1/stats", nil, session, 200)
		e.request("POST", "/api/admin/users", map[string]string{}, session, 403)
		status := 200
		if role == "Viewer" {
			status = 403
		}
		batch := map[string]any{"samples": []model.Sample{{Name: "role_probe", Value: 1}}}
		e.request("POST", "/api/v1/ingest", batch, session, status)
		e.request("POST", "/v1/metrics", map[string]any{}, session, status)
		e.request("POST", "/api/logout", nil, session, 200)
		e.request("GET", "/api/v1/stats", nil, session, 401)
	}
	// A cross-origin session mutation must fail before an ingest handler runs.
	req, err := http.NewRequest("POST", e.address+"/api/v1/ingest", strings.NewReader(`{"samples":[]}`))
	require.NoError(e.t, err)
	req.Header.Set("Cookie", e.adminCookie)
	req.Header.Set("Origin", "https://untrusted.invalid")
	res, err := e.client.Do(req)
	require.NoError(e.t, err)
	res.Body.Close()
	require.Equal(e.t, 403, res.StatusCode)
}

// Model the documented TLS proxy contract: preserve the public Host while the
// upstream transport is HTTP. This verifies composition with the real server;
// it does not claim a public TLS endpoint or certificate has been deployed.
func (e *productionRuntime) verifyProxyOrigin() {
	e.t.Helper()
	match := regexp.MustCompile(`Setup token: (\S+)`).FindStringSubmatch(e.mustDocker("logs", e.name))
	require.Len(e.t, match, 2)
	e.setupToken = match[1]
	request := func(method, path string, body any, origin, auth string, status int) *http.Response {
		var payload []byte
		if body != nil {
			var err error
			payload, err = json.Marshal(body)
			require.NoError(e.t, err)
		}
		_, response := e.requestHeaders(method, path, payload, "application/json", auth, status, map[string]string{
			"Origin": origin,
			// Untrusted forwarding metadata must never alter the origin policy.
			"X-Forwarded-Proto": "arbitrary-attacker-value",
		})
		return response
	}
	setup := map[string]string{"token": e.setupToken, "login": "runtime-admin", "password": e.password}
	request("POST", "/api/setup", setup, "https://foreign.invalid", "", 403)
	response := request("POST", "/api/setup", setup, e.publicOrigin, "", 200)
	for _, cookie := range response.Cookies() {
		if cookie.HttpOnly {
			require.True(e.t, cookie.Secure, "public HTTPS setup must issue a Secure cookie")
			require.Equal(e.t, http.SameSiteLaxMode, cookie.SameSite)
			e.adminCookie = cookie.Name + "=" + cookie.Value
		}
	}
	require.True(e.t, e.adminCookie != "")
	login := map[string]string{"user": "runtime-admin", "password": e.password}
	request("POST", "/login", login, "https://foreign.invalid", "", 403)
	response = request("POST", "/login", login, e.publicOrigin, "", 200)
	for _, cookie := range response.Cookies() {
		if cookie.HttpOnly {
			require.True(e.t, cookie.Secure)
			e.adminCookie = cookie.Name + "=" + cookie.Value
		}
	}
	const namespaces = "/apis/notifications.alerting.grafana.app/v1beta1/namespaces/default/inhibitionrules"
	request("GET", namespaces, nil, e.publicOrigin, "", 401)
	request("GET", namespaces, nil, e.publicOrigin, e.adminCookie, 200)
	batch := map[string]any{"samples": []model.Sample{{Name: "https_proxy_probe", Value: 1}}}
	request("POST", "/api/v1/ingest", batch, "https://foreign.invalid", e.adminCookie, 403)
	request("POST", "/api/v1/ingest", batch, e.publicOrigin, e.adminCookie, 200)
	request("GET", "/api/v1/stats", nil, e.publicOrigin, e.adminCookie, 200)
	request("POST", "/api/logout", nil, "https://foreign.invalid", e.adminCookie, 403)
	request("GET", "/api/v1/stats", nil, e.publicOrigin, e.adminCookie, 200)
	request("POST", "/api/logout", nil, e.publicOrigin, e.adminCookie, 200)
	request("GET", "/api/v1/stats", nil, e.publicOrigin, e.adminCookie, 401)
}

func (e *productionRuntime) verifyBusinessMetrics() {
	ts := time.Now().UnixMilli()
	for index, value := range []float64{233, 234} {
		e.request("POST", "/api/v1/ingest", map[string]any{"samples": []model.Sample{{Name: "business_orders_total", Labels: map[string]string{"service": "shop", "region": "cn"}, Value: value, Timestamp: ts + int64(index)}}}, e.bearer(), 200)
	}
	// Identical retransmission is idempotent; conflicting observations are not
	// an OLTP update operation and must not change the acknowledged value.
	e.request("POST", "/api/v1/ingest", map[string]any{"samples": []model.Sample{{Name: "business_orders_total", Labels: map[string]string{"service": "shop", "region": "cn"}, Value: 234, Timestamp: ts + 1}}}, e.bearer(), 200)
	e.request("POST", "/api/v1/ingest", map[string]any{"samples": []model.Sample{{Name: "business_orders_total", Labels: map[string]string{"service": "shop", "region": "cn"}, Value: 999, Timestamp: ts + 1}}}, e.bearer(), 400)
	batch := make([]model.Sample, 10000)
	for i := range batch {
		batch[i] = model.Sample{Name: "business_load_probe", Labels: map[string]string{"shard": strconv.Itoa(i % 20)}, Value: float64(i), Timestamp: ts - int64(i/20)*1000}
	}
	started := time.Now()
	e.request("POST", "/api/v1/ingest", map[string]any{"samples": batch}, e.bearer(), 200)
	elapsed := time.Since(started)
	e.t.Logf("durable JSON ingest: 10000 samples in %s (%.0f samples/s) under 1 CPU / 256 MiB", elapsed, 10000/elapsed.Seconds())
	otlp := json.RawMessage(fmt.Sprintf(`{"resourceMetrics":[{
		"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"shop"}}]},
		"scopeMetrics":[{"metrics":[{"name":"business.checkout.active","gauge":{"dataPoints":[{"timeUnixNano":"%d","asInt":"7"}]}}]}]
	}]}`, time.Now().UnixNano()))
	for _, path := range []string{"/v1/metrics", "/otlp/v1/metrics"} {
		e.request("POST", path, otlp, e.bearer(), 200)
	}
	remoteWrite := &prompb.WriteRequest{Timeseries: []prompb.TimeSeries{{Labels: []prompb.Label{{Name: "__name__", Value: "business_payments_total"}, {Name: "service", Value: "shop"}}, Samples: []prompb.Sample{{Timestamp: ts, Value: 12}}}}}
	protobuf, err := remoteWrite.Marshal()
	require.NoError(e.t, err)
	for _, path := range []string{"/api/v1/write", "/prometheus/api/v1/write"} {
		e.requestBytes("POST", path, snappy.Encode(nil, protobuf), "application/x-protobuf", e.bearer(), http.StatusNoContent)
	}
	e.verifyMetric("business_orders_total", 234)
	e.verifyMetric("business_checkout_active", 7)
	e.verifyMetric("business_payments_total", 12)
	promResult, _ := e.request("GET", "/prometheus/api/v1/query?query="+url.QueryEscape(`business_orders_total{service="shop",region="cn"}`), nil, e.bearer(), 200)
	require.Contains(e.t, string(promResult), `"234"`)
	timingResult, _ := e.request("POST", "/api/v1/alerting/mute-timings/preview", json.RawMessage(`{"at":"2026-10-21T01:30:00Z","timing":{"name":"runtime-timezone","time_intervals":[{"times":[{"start_time":"09:00","end_time":"10:00"}],"location":"Asia/Shanghai"}]}}`), e.bearer(), 200)
	require.Contains(e.t, string(timingResult), `"matches":true`, "release image lost embedded IANA timezone support")
	started = time.Now()
	e.request("GET", "/api/v1/query?metric=business_load_probe&range=15m&aggregation=sum", nil, e.bearer(), 200)
	e.t.Logf("10000-sample sum query: %s", time.Since(started))
}

func (e *productionRuntime) verifyMetric(metric string, value float64) {
	data, _ := e.request("GET", "/api/v1/query?metric="+metric+"&range=30m&aggregation=last", nil, e.bearer(), 200)
	var result model.QueryResult
	require.NoError(e.t, json.Unmarshal(data, &result))
	require.Len(e.t, result.Series, 1)
	require.NotEmpty(e.t, result.Series[0].Points)
	require.Equal(e.t, value, result.Series[0].Points[len(result.Series[0].Points)-1].Value)
}

func (e *productionRuntime) verifyConfinement() {
	require.Equal(e.t, "10001", e.mustDocker("exec", e.name, "id", "-u"))
	require.Equal(e.t, "true", e.mustDocker("inspect", e.name, "--format", "{{.HostConfig.ReadonlyRootfs}}"))
	out, err := e.docker("exec", e.name, "sh", "-c", "touch /must-not-write")
	require.Error(e.t, err, "container unexpectedly wrote to the root filesystem")
	require.Contains(e.t, out, "Read-only file system")
	e.mustDocker("exec", e.name, "sh", "-c", "test -w /data && test -w /tmp && grep -q 'CapEff:[[:space:]]*0000000000000000' /proc/1/status && grep -q 'NoNewPrivs:[[:space:]]*1' /proc/1/status")
}

func (e *productionRuntime) verifyPersisted(mode string) {
	e.verifyMetric("business_orders_total", 234)
	e.verifyMetric("business_checkout_active", 7)
	e.verifyMetric("business_payments_total", 12)
	data, _ := e.request("GET", "/api/v1/targets", nil, e.bearer(), 200)
	require.Contains(e.t, string(data), "backup-secret-probe")
	require.False(e.t, bytes.Contains(data, []byte(e.collectorSecret)), "collector API leaked a recovered credential")
	if mode == "local" {
		e.request("GET", "/api/auth/me", nil, e.adminCookie, 200)
		e.request("POST", "/login", map[string]string{"user": "runtime-admin", "password": e.password}, "", 200)
		data, _ := e.request("GET", "/api/auth/status", nil, "", 200)
		require.Contains(e.t, string(data), `"setupRequired":false`)
	}
}

func (e *productionRuntime) resources() {
	// memory.current includes the cgroup's page cache, unlike process RSS.
	current, err := strconv.ParseInt(e.mustDocker("exec", e.name, "cat", "/sys/fs/cgroup/memory.current"), 10, 64)
	require.NoError(e.t, err)
	for _, line := range strings.Split(e.mustDocker("exec", e.name, "cat", "/sys/fs/cgroup/memory.stat"), "\n") {
		if strings.HasPrefix(line, "anon ") || strings.HasPrefix(line, "file ") || strings.HasPrefix(line, "kernel ") {
			e.t.Logf("cgroup memory: %s", line)
		}
	}
	require.Less(e.t, current, int64(230<<20), "service approached its 256 MiB limit after a modest batch")
	stats := e.mustDocker("stats", "--no-stream", "--format", "{{.CPUPerc}} CPU, {{.MemUsage}}, {{.PIDs}} PIDs", e.name)
	e.t.Logf("runtime after ingest/restart: memory.current=%d bytes; %s", current, stats)
	require.Equal(e.t, "false", e.mustDocker("inspect", e.name, "--format", "{{.State.OOMKilled}}"))
}

func (e *productionRuntime) verifyBackupRecovery(mode string) {
	// An offline archive includes SQLite WAL files, identities, keys and plugins.
	// The production process is stopped before the volume is read.
	e.mustDocker("stop", "--time", "20", e.name)
	require.Equal(e.t, "0", e.mustDocker("inspect", e.name, "--format", "{{.State.ExitCode}}"), "SIGTERM did not exit gracefully")
	archive := e.prefix + "-" + mode + "-backup"
	restored := e.prefix + "-" + mode + "-restored"
	for _, volume := range []string{archive, restored} {
		e.mustDocker("volume", "create", "--label", "metricspanel.verify="+e.prefix, volume)
		e.t.Cleanup(func() { e.cleanup("volume", "rm", volume) })
	}
	// Docker initializes fresh volume ownership from /data in the release image.
	// Backup helpers inherit the non-root release user and have no network.
	e.mustDocker("run", "--rm", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true", "--network=none", "--mount", "type=volume,src="+e.volume+",dst=/source,readonly", "--mount", "type=volume,src="+archive+",dst=/data", "--entrypoint=sh", e.image, "-c", "tar -C /source -cf /data/data.tar . && sha256sum /data/data.tar | sed 's|/data/data.tar|/backup/data.tar|' > /data/data.sha256")
	e.mustDocker("run", "--rm", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true", "--network=none", "--mount", "type=volume,src="+archive+",dst=/backup,readonly", "--mount", "type=volume,src="+restored+",dst=/data", "--entrypoint=sh", e.image, "-c", "sha256sum -c /backup/data.sha256 && tar -C /data -xf /backup/data.tar")
	recovered := *e
	recovered.name = e.name + "-restore"
	recovered.volume = restored
	e.t.Cleanup(func() { recovered.cleanup("rm", "-f", "--volumes", recovered.name) })
	recovered.start(mode)
	recovered.verifyConfinement()
	recovered.verifyPersisted(mode)
	recovered.mustDocker("stop", "--time", "20", recovered.name)
	// Explicitly decrypt the restored collector secret through the real Store.
	// docker cp copies only offline data into a t.TempDir removed by Go testing.
	dataDir := e.t.TempDir()
	e.mustDocker("cp", recovered.name+":/data/.", dataDir)
	verifyRecoveredSecret(e.t, filepath.Join(dataDir, "control.db"), e.collectorSecret)
	e.t.Log("offline SHA-256-checked backup restored metrics, encrypted collector settings and local identities into a fresh volume")
}

func verifyRecoveredSecret(t *testing.T, path, expected string) {
	t.Helper()
	database, err := store.Open(path)
	require.NoError(t, err)
	defer database.DB.Close()
	targets, err := database.Targets(context.Background())
	require.NoError(t, err)
	require.Len(t, targets, 1)
	secrets, err := database.TargetSecrets(context.Background(), targets[0].ID)
	require.NoError(t, err)
	require.True(t, secrets["bearer_token"] == expected, "recovered collector credential could not be decrypted")
	require.NoError(t, database.DB.Close())
	// A recovery missing the key must fail closed, not regenerate a new key and
	// silently lose access to encrypted configuration. This is only a temp copy.
	require.NoError(t, os.Remove(filepath.Join(filepath.Dir(path), "secrets.key")))
	_, err = store.Open(path)
	require.ErrorContains(t, err, "secrets.key is missing")
}
