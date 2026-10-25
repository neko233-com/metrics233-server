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
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/server"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

const tempoLiveImage = "grafana/tempo:3.1.1@sha256:64e2fbd56d9c2b3412a4b430a58ea59e91a670b1e75f920a0c702213ad145044"

type liveTempoResult struct {
	Error  string `json:"error"`
	Frames []struct {
		Schema struct {
			Fields []struct{ Name string } `json:"fields"`
		} `json:"schema"`
		Data struct{ Values [][]any } `json:"data"`
	} `json:"frames"`
}

func liveTempoFields(t *testing.T, result liveTempoResult) map[string][]any {
	t.Helper()
	require.Empty(t, result.Error)
	require.Len(t, result.Frames, 1)
	frame := result.Frames[0]
	require.Len(t, frame.Data.Values, len(frame.Schema.Fields))
	fields := map[string][]any{}
	for index, field := range frame.Schema.Fields {
		fields[field.Name] = frame.Data.Values[index]
	}
	return fields
}

// This contract uses a real, immutable upstream daemon, not recorded JSON or a
// mocked query response. All published ports bind loopback and cleanup uses the
// exact container ID and ownership label returned by this invocation.
func TestTempoLiveDataSource(t *testing.T) {
	if testing.Short() {
		t.Skip("live Tempo requires Docker and full integration mode")
	}
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	e := environment{root: root, t: t, client: &http.Client{Timeout: 8 * time.Second}}
	docker := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, "docker", args...)
		command.Dir = root
		var diagnostics bytes.Buffer
		command.Stderr = &diagnostics
		output, err := command.Output()
		if err != nil || (len(args) > 0 && args[0] == "logs") {
			output = append(output, diagnostics.Bytes()...)
		}
		// Cold pulls print progress to stderr even when `run --detach` succeeds.
		// Inspect/port/run identities must use stdout alone; retain both on error.
		return strings.TrimSpace(string(output)), err
	}
	random := make([]byte, 16)
	_, err := rand.Read(random)
	require.NoError(t, err)
	// Tempo search intentionally trims leading zeroes while OTLP/detail IDs use
	// all 16 bytes. Exercise that valid shape deterministically, not by chance.
	random[0] &= 0x0f
	random[len(random)-1] |= 1
	traceID := hex.EncodeToString(random)
	searchID := strings.TrimLeft(traceID, "0")
	owner := traceID[:12]
	config := filepath.Join(t.TempDir(), "tempo.yaml")
	// Derived from the upstream v3.1.1 single-binary example. No metrics-generator,
	// telemetry exporter or external storage is needed for this read-path check.
	require.NoError(t, os.WriteFile(config, []byte(`server:
  http_listen_port: 3200
  log_level: warn
distributor:
  receivers:
    otlp:
      protocols:
        http:
          endpoint: "0.0.0.0:4318"
storage:
  trace:
    backend: local
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/blocks
usage_report:
  reporting_enabled: false
`), 0644))
	image := os.Getenv("METRICSPANEL_TEST_TEMPO_IMAGE")
	if image == "" {
		image = tempoLiveImage
	}
	name := "metrics233-tempo-live-" + owner
	t.Cleanup(func() {
		owned, check := docker("inspect", "--format", `{{.Id}} {{index .Config.Labels "metrics233.tempo.verify"}}`, name)
		if check != nil {
			remaining, listError := docker("ps", "--all", "--quiet", "--filter", "name=^/"+name+"$")
			if listError != nil || remaining != "" {
				t.Errorf("cannot establish Tempo cleanup ownership: %v / %v", check, listError)
			}
			return // A failed pull/create may never have created a container.
		}
		identity := strings.Fields(owned)
		if len(identity) != 2 || len(identity[0]) != 64 || identity[1] != owner {
			t.Error("refusing cleanup without matching Tempo ID and ownership")
			return
		}
		if t.Failed() {
			logs, _ := docker("logs", "--tail", "50", identity[0])
			t.Log(logs)
		}
		_, cleanup := docker("rm", "--force", identity[0])
		require.NoError(t, cleanup)
	})
	id, err := docker("run", "--detach", "--name", name,
		"--label", "metrics233.tempo.verify="+owner,
		"--user", "10001:10001", "--read-only", "--cap-drop=ALL", "--security-opt", "no-new-privileges:true",
		"--pids-limit", "128", "--cpus", "1", "--memory", "512m",
		"--tmpfs", "/var/tempo:rw,noexec,nosuid,size=256m,mode=1777",
		"--mount", "type=bind,source="+config+",target=/etc/tempo.yaml,readonly",
		"--publish", "127.0.0.1::3200", "--publish", "127.0.0.1::4318",
		image, "-target=all", "-config.file=/etc/tempo.yaml")
	require.NoError(t, err, id)
	require.Len(t, id, 64)
	address := func(port string) string {
		published, err := docker("port", id, port)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(published, "127.0.0.1:"), published)
		return "http://" + published
	}
	tempo, ingest := address("3200"), address("4318")
	require.Eventually(t, func() bool {
		response, err := e.client.Get(tempo + "/ready")
		if err != nil {
			return false
		}
		defer response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, 90*time.Second, 500*time.Millisecond, "real Tempo did not become ready")

	start := time.Now().Add(-30 * time.Second)
	span := func(id, parent, name string, offset, duration time.Duration, failed bool) map[string]any {
		code, message := 1, ""
		if failed {
			code, message = 2, "database deadline exceeded"
		}
		return map[string]any{"traceId": traceID, "spanId": id, "parentSpanId": parent, "name": name,
			"kind": 2, "startTimeUnixNano": fmt.Sprint(start.Add(offset).UnixNano()),
			"endTimeUnixNano": fmt.Sprint(start.Add(offset + duration).UnixNano()),
			"status":          map[string]any{"code": code, "message": message}}
	}
	resource := func(service string, spans ...map[string]any) map[string]any {
		return map[string]any{"resource": map[string]any{"attributes": []any{map[string]any{
			"key": "service.name", "value": map[string]string{"stringValue": service}}}},
			"scopeSpans": []any{map[string]any{"scope": map[string]string{"name": "metrics233-live-check"}, "spans": spans}}}
	}
	payload, err := json.Marshal(map[string]any{"resourceSpans": []any{
		resource("metrics233-live-check", span("0000000000000001", "", "POST /checkout", 0, 120500*time.Microsecond, false)),
		resource("orders-db", span("0000000000000002", "0000000000000001", "INSERT orders", 10*time.Millisecond, 25250*time.Microsecond, true),
			span("0000000000000003", "0000000000000002", "save entity", 12*time.Millisecond, 5*time.Millisecond, false)),
	}})
	require.NoError(t, err)
	response, err := e.client.Post(ingest+"/v1/traces", "application/json", bytes.NewReader(payload))
	require.NoError(t, err)
	ack, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	response.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(ack))

	database, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	app := httptest.NewServer(server.New(database, token, 15).Handler())
	t.Cleanup(app.Close)
	const uid = "live-tempo"
	e.must(app.URL, "POST", "/api/datasources", map[string]any{"uid": uid, "name": "Live Tempo", "type": "tempo", "url": tempo, "access": "proxy"})
	e.must(app.URL, "GET", "/api/datasources/uid/"+uid+"/health", nil)
	query := func(target map[string]any) liveTempoResult {
		target["refId"], target["datasource"] = "A", map[string]string{"uid": uid}
		body := e.must(app.URL, "POST", "/api/ds/query", map[string]any{"from": fmt.Sprint(start.Add(-time.Minute).UnixMilli()),
			"to": fmt.Sprint(time.Now().UnixMilli()), "queries": []any{target}})
		var result struct{ Results map[string]liveTempoResult }
		require.NoError(t, json.Unmarshal(body, &result))
		return result.Results["A"]
	}
	var search liveTempoResult
	require.Eventually(t, func() bool {
		search = query(map[string]any{"query": `{resource.service.name="metrics233-live-check"}`, "limit": 10})
		return search.Error == "" && len(search.Frames) == 1 && len(search.Frames[0].Data.Values) > 0 && len(search.Frames[0].Data.Values[0]) == 1
	}, 90*time.Second, time.Second, "TraceQL could not find the actually ingested trace")
	fields := liveTempoFields(t, search)
	require.Equal(t, []any{searchID}, fields["traceID"])
	require.Equal(t, []any{"metrics233-live-check"}, fields["rootServiceName"])
	fields = liveTempoFields(t, query(map[string]any{"queryType": "traceId", "traceId": traceID}))
	require.Equal(t, []any{"0000000000000001", "0000000000000002", "0000000000000003"}, fields["spanID"])
	require.Equal(t, []any{"", "0000000000000001", "0000000000000002"}, fields["parentSpanID"])
	require.Equal(t, []any{120.5, 25.25, float64(5)}, fields["durationMs"])
	require.Equal(t, []any{"ok", "error", "ok"}, fields["status"])
	require.Equal(t, "database deadline exceeded", fields["statusMessage"][1])
	require.Equal(t, []any{"metrics233-live-check", "orders-db", "orders-db"}, fields["serviceName"])
	require.Equal(t, []any{traceID, traceID, traceID}, fields["traceID"])
	short := liveTempoFields(t, query(map[string]any{"queryType": "traceId", "traceId": searchID}))
	require.Equal(t, fields["spanID"], short["spanID"], "the search ID must resolve to the same canonical spans")
	missing := liveTempoFields(t, query(map[string]any{"queryType": "traceId", "traceId": strings.Repeat("0", 32)}))
	require.Empty(t, missing["spanID"])
	unauthenticated, err := e.client.Post(app.URL+"/api/ds/query", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	unauthenticated.Body.Close()
	require.Equal(t, http.StatusUnauthorized, unauthenticated.StatusCode)

	var dashboard struct{ ID string }
	require.NoError(t, json.Unmarshal(e.must(app.URL, "POST", "/api/v1/dashboards", map[string]any{
		"name": "Live Tempo verification", "panels": []any{map[string]any{"id": "live-traces", "title": "Live Tempo traces", "metric": "", "unit": "", "aggregation": "last", "visualization": "traces",
			"config": map[string]any{"id": 1, "type": "traces", "gridPos": map[string]int{"x": 0, "y": 0, "w": 24, "h": 20},
				"targets": []any{map[string]any{"refId": "A", "datasource": map[string]string{"uid": uid, "type": "tempo"}, "query": `{resource.service.name="metrics233-live-check"}`}}}}},
	}), &dashboard))
	require.NotEmpty(t, dashboard.ID)
	if os.Getenv("METRICSPANEL_TEMPO_BROWSER_TEST") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, "node", "node_modules/@playwright/test/cli.js", "test", "e2e/tempo-live.spec.ts", "--retries=0", "--reporter=list", "--output", filepath.Join(t.TempDir(), "browser"))
		command.Dir = filepath.Join(root, "web")
		command.Env = append(os.Environ(), "METRICSPANEL_TEMPO_E2E_URL="+app.URL, "METRICSPANEL_TEMPO_E2E_TOKEN="+token,
			"METRICSPANEL_TEMPO_E2E_TRACE="+traceID, "METRICSPANEL_TEMPO_E2E_DASHBOARD="+dashboard.ID)
		output, err := command.CombinedOutput()
		require.NoError(t, err, "live Tempo browser failed:\n%s", strings.ReplaceAll(string(output), token, "[redacted]"))
		t.Logf("live Tempo browser:\n%s", strings.ReplaceAll(string(output), token, "[redacted]"))
	}
	if readyFile := os.Getenv("METRICSPANEL_TEMPO_QA_READY_FILE"); readyFile != "" {
		// Optional operator Edge inspection of this exact live fixture. The receipt
		// contains no credentials and must be a new file below the OS temp folder.
		clean := filepath.Clean(readyFile)
		require.True(t, filepath.IsAbs(clean))
		parent, err := filepath.EvalSymlinks(filepath.Dir(clean))
		require.NoError(t, err)
		temp, err := filepath.EvalSymlinks(os.TempDir())
		require.NoError(t, err)
		clean = filepath.Join(parent, filepath.Base(clean))
		relative, err := filepath.Rel(temp, clean)
		require.NoError(t, err)
		require.True(t, relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), "QA receipt must stay below the resolved OS temp directory")
		receipt, err := json.Marshal(map[string]string{"url": app.URL, "dashboard": dashboard.ID, "trace_id": traceID})
		require.NoError(t, err)
		file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		require.NoError(t, err)
		_, err = file.Write(receipt)
		closeError := file.Close()
		require.NoError(t, err)
		require.NoError(t, closeError)
		t.Log("real Tempo Edge fixture ready; waiting for owned completion marker (maximum 5 minutes)")
		deadline := time.Now().Add(5 * time.Minute)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(clean + ".done"); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	t.Log("real Tempo 3.1.1 OTLP ingestion, TraceQL, hierarchy/duration/error detail, missing trace and authenticated HTTP query passed")
}
