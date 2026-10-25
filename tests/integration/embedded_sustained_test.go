//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

// Opt-in operational profile: real release runtime, paced durable HTTP writes,
// concurrent PromQL and exact restart readback. It deliberately sets no timing
// SLO, and normal CI never spends ten minutes on a workstation-specific number.
func TestDockerEmbeddedSustainedProfile(t *testing.T) {
	if testing.Short() || os.Getenv("METRICS233_EMBEDDED_SOAK") != "1" {
		t.Skip("set METRICS233_EMBEDDED_SOAK=1 for the isolated sustained profile")
	}
	seconds := 600
	if value := os.Getenv("METRICS233_EMBEDDED_SOAK_SECONDS"); value != "" {
		parsed, err := strconv.Atoi(value)
		require.NoError(t, err)
		require.True(t, parsed >= 30 && parsed <= 3600, "profile duration must be 30..3600 seconds")
		seconds = parsed
	}
	_, source, _, _ := runtime.Caller(0)
	e := &productionRuntime{t: t, root: filepath.Clean(filepath.Join(filepath.Dir(source), "../..")), client: &http.Client{Timeout: 20 * time.Second}}
	e.token = e.secret()
	e.prefix = "metricspanel-embedded-soak-" + e.secret()[:12]
	e.name, e.volume = e.prefix, e.prefix+"-data"
	e.image = os.Getenv("METRICSPANEL_RUNTIME_TEST_IMAGE")
	if e.image == "" {
		e.image = e.prefix + ":test"
		t.Cleanup(func() { e.cleanup("image", "rm", e.image) })
		e.mustDocker("build", "--target", "runtime", "--label", "metricspanel.verify="+e.prefix, "-t", e.image, ".")
	}
	e.mustDocker("volume", "create", "--label", "metricspanel.verify="+e.prefix, e.volume)
	t.Cleanup(func() {
		require.Equal(t, e.prefix, e.mustDocker("volume", "inspect", e.volume, "--format", `{{index .Labels "metricspanel.verify"}}`))
		e.cleanup("volume", "rm", e.volume)
	})
	t.Cleanup(func() {
		require.Equal(t, e.prefix, e.mustDocker("inspect", e.name, "--format", `{{index .Config.Labels "metricspanel.verify"}}`))
		if t.Failed() {
			t.Log(e.redact(e.mustDocker("logs", "--tail", "30", e.name)))
		}
		e.cleanup("rm", "--force", "--volumes", e.name)
	})
	e.start("token")
	e.verifyConfinement()
	t.Logf("runtime identity: %s", e.mustDocker("exec", e.name, "metricspanel", "version"))

	const cardinality, batchSize = 4000, 500
	const metric = "metrics233_sustained_profile_value"
	want := make([]float64, cardinality)
	sent := 0
	var writeTimes, queryTimes []time.Duration
	ingest := func(first, cycle int) {
		samples := make([]model.Sample, batchSize)
		at := time.Now().UnixMilli()
		for offset := range samples {
			identity := (first + offset) % cardinality
			value := float64(cycle*10 + identity%10)
			want[identity] = value
			samples[offset] = model.Sample{Name: metric, Labels: map[string]string{"series": strconv.Itoa(identity), "env": "profile"}, Timestamp: at, Value: value}
		}
		started := time.Now()
		body, _ := e.request("POST", "/api/v1/ingest", map[string]any{"samples": samples}, e.bearer(), 200)
		writeTimes = append(writeTimes, time.Since(started))
		var ack struct{ Accepted int }
		require.NoError(t, json.Unmarshal(body, &ack))
		require.Equal(t, batchSize, ack.Accepted)
		sent += len(samples)
	}
	for first := 0; first < cardinality; first += batchSize {
		ingest(first, 0)
	}
	read := func(ctx context.Context, expression string) (float64, error) {
		request, err := http.NewRequestWithContext(ctx, "GET", e.address+"/prometheus/api/v1/query?query="+url.QueryEscape(expression), nil)
		if err != nil {
			return 0, err
		}
		request.Header.Set("Authorization", e.bearer())
		response, err := e.client.Do(request)
		if err != nil {
			return 0, err
		}
		defer response.Body.Close()
		var result struct {
			Status string
			Data   struct {
				Result []struct{ Value []json.RawMessage }
			}
		}
		if response.StatusCode != 200 {
			return 0, fmt.Errorf("PromQL returned HTTP %d", response.StatusCode)
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
			return 0, err
		}
		if result.Status != "success" || len(result.Data.Result) != 1 || len(result.Data.Result[0].Value) != 2 {
			return 0, fmt.Errorf("unexpected PromQL scalar-vector result")
		}
		var text string
		if err := json.Unmarshal(result.Data.Result[0].Value[1], &text); err != nil {
			return 0, err
		}
		value, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, fmt.Errorf("invalid PromQL numeric result")
		}
		return value, nil
	}
	queryContext, stopQueries := context.WithCancel(t.Context())
	defer stopQueries()
	queryDone, queryError := make(chan struct{}), make(chan error, 1)
	go func() {
		defer close(queryDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			at := time.Now()
			_, err := read(queryContext, "sum("+metric+")")
			if queryContext.Err() != nil {
				return
			}
			if err != nil {
				queryError <- err
				return
			}
			queryTimes = append(queryTimes, time.Since(at))
			select {
			case <-queryContext.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	started := time.Now()
	for cycle := 0; cycle < seconds; cycle++ {
		select {
		case err := <-queryError:
			require.NoError(t, err)
		default:
		}
		ingest(cycle*batchSize, cycle/8+1)
		if cycle%60 == 59 {
			t.Logf("sustained profile: %d/%d paced batches, %d accepted samples", cycle+1, seconds, sent)
		}
		timer := time.NewTimer(time.Until(started.Add(time.Duration(cycle+1) * time.Second)))
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatal("profile cancelled")
		case <-timer.C:
		}
	}
	stopQueries()
	select {
	case <-queryDone:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent query did not stop")
	}
	select {
	case err := <-queryError:
		require.NoError(t, err)
	default:
	}
	require.NotEmpty(t, queryTimes)
	var sum float64
	for _, value := range want {
		sum += value
	}
	verify := func() {
		got, err := read(t.Context(), "sum("+metric+")")
		require.NoError(t, err)
		require.Equal(t, sum, got, "latest observations changed")
		got, err = read(t.Context(), fmt.Sprintf("sum(count_over_time(%s[%ds]))", metric, seconds+300))
		require.NoError(t, err)
		require.Equal(t, float64(sent), got, "acknowledged sample count changed")
	}
	verify()
	percentiles := func(values []time.Duration) map[string]float64 {
		sorted := append([]time.Duration(nil), values...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		result := map[string]float64{}
		for name, quantile := range map[string]float64{"p50": .5, "p95": .95, "p99": .99, "max": 1} {
			index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
			result[name] = float64(sorted[index]) / float64(time.Millisecond)
		}
		return result
	}
	report := map[string]any{"duration_seconds": time.Since(started).Seconds(), "series": cardinality,
		"target_samples_per_second": batchSize, "accepted_samples": sent, "query_count": len(queryTimes),
		"ingest_ms": percentiles(writeTimes), "query_ms": percentiles(queryTimes),
		"cgroup_memory_peak_bytes":      e.mustDocker("exec", e.name, "cat", "/sys/fs/cgroup/memory.peak"),
		"cgroup_cpu_stat":               e.mustDocker("exec", e.name, "cat", "/sys/fs/cgroup/cpu.stat"),
		"active_directory_allocated_kb": strings.Fields(e.mustDocker("exec", e.name, "du", "-sk", "/data"))[0]}
	require.Equal(t, "false", e.mustDocker("inspect", e.name, "--format", "{{.State.OOMKilled}}"))
	e.mustDocker("restart", "--timeout", "20", e.name)
	e.waitReady()
	verify()
	report["restart_exact_readback"] = true
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	t.Logf("embedded sustained profile: %s", encoded)
}
