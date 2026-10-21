package server

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSavedRetentionPrunesOnlyExpiredMetricsAndKeepsControlData(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer s.DB.Close()
	srv := New(s, "", 30)
	now := time.Now().Truncate(time.Millisecond)
	cutoff := now.Add(-15 * 24 * time.Hour).UnixMilli()
	require.NoError(t, s.Ingest(ctx, []model.Sample{{Name: "expired_metric", Timestamp: cutoff - 1, Value: 1}, {Name: "boundary_metric", Timestamp: cutoff, Value: 2}, {Name: "recent_metric", Timestamp: now.UnixMilli(), Value: 3}}))
	h := srv.Handler()
	call := func(method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/settings/retention", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"days":30`)
	for _, body := range []string{`{"days":0,"version":0}`, `{"days":1.5,"version":0}`, `{"days":15}`, `{"days":15,"version":0,"unknown":1}`} {
		require.Equal(t, 400, call("PUT", body).Code)
	}
	w = call("PUT", `{"days":15,"version":0}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"version":1`)
	require.Equal(t, 409, call("PUT", `{"days":1,"version":0}`).Code)
	require.NoError(t, srv.pruneExpiredMetrics(ctx, now))
	metrics, err := s.Metrics(ctx)
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	require.Equal(t, "boundary_metric", metrics[0].Name)
	require.Equal(t, "recent_metric", metrics[1].Name)
	dashboards, err := s.Dashboards(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, dashboards)
	setting, err := s.MetricRetention(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, 15, setting.Days)
	stats := httptest.NewRecorder()
	h.ServeHTTP(stats, httptest.NewRequest("GET", "/api/v1/stats", nil))
	require.Equal(t, 200, stats.Code)
	require.Contains(t, stats.Body.String(), `"retention_days":15`)
}
