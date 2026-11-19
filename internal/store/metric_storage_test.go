package store

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestMetricStorageStrategiesKeepReportingAndQueryContract(t *testing.T) {
	for _, strategy := range []string{"", "embedded", "sqlite"} {
		want := strategy
		if want == "" {
			want = DefaultMetricStorage
		}
		t.Run("configured_"+strategy, func(t *testing.T) {
			options := MetricStorageOptions{Strategy: strategy, ControlPath: filepath.Join(t.TempDir(), "control.db")}
			ctx := t.Context()
			control, err := OpenMetricStorage(ctx, options)
			require.NoError(t, err)
			defer func() { require.NoError(t, control.Close()) }()
			require.Equal(t, want, control.Kind())
			require.NoError(t, control.Health(ctx))
			now := time.Now().UnixMilli()
			require.NoError(t, control.Ingest(ctx, []model.Sample{{Name: "strategy_fixture", Value: 23, Timestamp: now}}))
			stats, err := control.Stats(ctx)
			require.NoError(t, err)
			require.Equal(t, want, stats.Backend)
			series, err := control.LoadSeries(ctx, now-1000, now+1000, []Matcher{{Name: "__name__", Type: "=", Value: "strategy_fixture"}})
			require.NoError(t, err)
			require.Len(t, series, 1)
			require.Len(t, series[0].Points, 1)
			require.Equal(t, 23.0, series[0].Points[0].Value)
		})
	}
}

func TestMetricStorageRejectsUnknownBeforeCreatingCatalogAndOpensClickHouseOnlyWhenSelected(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "control.db")
	_, err := OpenMetricStorage(ctx, MetricStorageOptions{Strategy: "typo", ControlPath: path})
	require.ErrorContains(t, err, "unknown metrics storage strategy")
	require.NoFileExists(t, path)

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fake.Close()
	control, err := OpenMetricStorage(ctx, MetricStorageOptions{
		Strategy: "clickhouse", ControlPath: path,
		ClickHouse: ClickHouseStorageOptions{URL: fake.URL, Database: "fixture"},
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, control.Close()) }()
	require.IsType(t, &ClickHouse{}, control.Backend)
	require.Equal(t, "clickhouse", control.Kind())
	require.NoError(t, control.Health(ctx))
}
