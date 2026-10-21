package collector

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusParserBodySampleAndHistogramBoundaries(t *testing.T) {
	_, err := ParsePrometheus(bytes.NewReader(bytes.Repeat([]byte("#"), maxBody+1)), model.Target{})
	require.ErrorContains(t, err, "4 MiB")
	var many strings.Builder
	for index := 0; index < 10001; index++ {
		fmt.Fprintf(&many, "metric{index=\"%d\"} 1\n", index)
	}
	_, err = ParsePrometheus(strings.NewReader(many.String()), model.Target{})
	require.ErrorContains(t, err, "10000 samples")
	_, err = ParsePrometheus(strings.NewReader("# TYPE duration histogram\nduration_bucket{le=\"-Inf\"} 0\nduration_bucket{le=\"+Inf\"} 3\nduration_count 3\n"), model.Target{})
	require.NoError(t, err, "the first histogram bound may be negative infinity")
	for _, histogram := range []string{
		"duration_bucket{le=\"0.1\"} 3\nduration_bucket{le=\"+Inf\"} 2\nduration_count 2\n",
		"duration_bucket{le=\"0.1\"} 3\nduration_bucket{le=\"0.5\"} 2\nduration_count 3\n",
		"duration_bucket{le=\"+Inf\"} 2\nduration_count 3\n",
		"duration_bucket{le=\"0.1\"} 1\n",
	} {
		_, err = ParsePrometheus(strings.NewReader("# TYPE duration histogram\n"+histogram), model.Target{})
		require.ErrorContains(t, err, "invalid classic histogram")
	}
}

func TestPullProtocolIdentityLabelsHaveConsistentPrecedence(t *testing.T) {
	for _, contentType := range []string{"text/plain;version=0.0.4", "application/openmetrics-text;version=1.0.0"} {
		t.Run(contentType, func(t *testing.T) {
			body := "# TYPE duration histogram\nduration_bucket{job=\"spoofed\",instance=\"spoofed\",le=\"0.1\"} 2\nduration_bucket{job=\"spoofed\",instance=\"spoofed\",le=\"+Inf\"} 3\nduration_sum{job=\"spoofed\",instance=\"spoofed\"} 0.25\nduration_count{job=\"spoofed\",instance=\"spoofed\"} 3\n"
			if strings.HasPrefix(contentType, "application/openmetrics") {
				body += "# EOF\n"
			}
			samples, err := ParseExposition(strings.NewReader(body), contentType, model.Target{Name: "authoritative-job", URL: "http://target/metrics", Labels: map[string]string{"cluster": "prod", "le": "must-not-collapse-buckets"}})
			require.NoError(t, err)
			bounds := map[string]bool{}
			for _, sample := range samples {
				assert.Equal(t, "authoritative-job", sample.Labels["job"])
				assert.Equal(t, "http://target/metrics", sample.Labels["instance"])
				assert.Equal(t, "prod", sample.Labels["cluster"])
				if sample.Name == "duration_bucket" {
					bounds[sample.Labels["le"]] = true
				}
			}
			assert.Equal(t, map[string]bool{"0.1": true, "+Inf": true}, bounds)
		})
	}
}

func TestScrapeHTTPAuthenticationRedirectCancellationAndNoPartialMetrics(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer db.DB.Close()
	token := rand.Text()
	var redirected atomic.Int32
	cancelled := make(chan struct{})
	exporter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, "business_received_total 23\n")
		case "/redirect":
			http.Redirect(w, r, "/destination", http.StatusFound)
		case "/destination":
			redirected.Add(1)
			fmt.Fprint(w, "redirected_metric 1\n")
		case "/cancel":
			<-r.Context().Done()
			close(cancelled)
		case "/malformed":
			fmt.Fprint(w, "partial_business_metric 1\ninvalid not_a_number\n")
		}
	}))
	defer exporter.Close()
	c := New(db)
	target, err := db.SaveTarget(context.Background(), model.Target{Name: "auth", URL: exporter.URL + "/auth", IntervalSeconds: 5, SecureSettings: map[string]string{"bearer_token": token}})
	require.NoError(t, err)
	count, err := c.Scrape(context.Background(), target)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	target.SecureSettings = map[string]string{"bearer_token": ""}
	target, err = db.SaveTarget(context.Background(), target)
	require.NoError(t, err)
	_, err = c.Scrape(context.Background(), target)
	require.ErrorContains(t, err, "401")
	assert.False(t, strings.Contains(err.Error(), token), "authorization must not appear in errors")
	for _, path := range []string{"/redirect", "/malformed"} {
		fixture, err := db.SaveTarget(context.Background(), model.Target{Name: strings.TrimPrefix(path, "/"), URL: exporter.URL + path, IntervalSeconds: 5})
		require.NoError(t, err)
		count, err := c.Scrape(context.Background(), fixture)
		require.Error(t, err)
		assert.Zero(t, count)
	}
	assert.Zero(t, redirected.Load(), "collector must not forward credentials through redirects")
	fixture, err := db.SaveTarget(context.Background(), model.Target{Name: "cancel", URL: exporter.URL + "/cancel", IntervalSeconds: 5})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = c.Scrape(ctx, fixture)
	require.Error(t, err)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("scrape did not cancel the HTTP upstream")
	}
	targets, err := db.Targets(context.Background())
	require.NoError(t, err)
	for _, saved := range targets {
		require.NotEmpty(t, saved.LastError)
		assert.Zero(t, saved.Samples)
		assert.False(t, strings.Contains(saved.LastError, token), "stored failure must not contain authorization")
	}
	metrics, err := db.Metrics(context.Background())
	require.NoError(t, err)
	for _, metric := range metrics {
		assert.NotEqual(t, "partial_business_metric", metric.Name)
		assert.NotEqual(t, "redirected_metric", metric.Name)
	}
}
