package store

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func embeddedFixture(t *testing.T) (*Store, *Embedded, string) {
	t.Helper()
	root := t.TempDir()
	control, err := Open(filepath.Join(root, "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { control.DB.Close() })
	path := filepath.Join(root, "metrics")
	engine, err := OpenEmbedded(t.Context(), control, path, 15)
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })
	return control, engine, path
}

func TestEmbeddedNativePrecisionRestartAndIndexedQueries(t *testing.T) {
	control, engine, path := embeddedFixture(t)
	ctx := t.Context()
	at := time.Now().Add(-time.Hour).Truncate(time.Second).UnixMilli()
	samples := []model.Sample{}
	for i := 0; i < 120; i++ {
		for _, server := range []string{"one", "two"} {
			samples = append(samples, model.Sample{Name: "game_counter_total", Labels: map[string]string{"serverId": server}, Timestamp: at + int64(i)*30000, Value: float64(i) * 1.0000000000000002})
		}
	}
	require.NoError(t, engine.Ingest(ctx, samples))
	rows, err := engine.LoadSeries(ctx, at, at+3600000, []Matcher{{Name: "serverId", Type: "=", Value: "one"}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].Points, 120)
	for i, p := range rows[0].Points {
		require.Equal(t, math.Float64bits(float64(i)*1.0000000000000002), math.Float64bits(p.Value))
	}
	query := model.Query{Metric: "game_counter_total", Labels: map[string]string{"serverId": "one"}, Start: at, End: at + 3600000, Step: 60000, Aggregation: "last"}
	result, err := engine.Query(ctx, query)
	require.NoError(t, err)
	require.Len(t, result.Series, 1)
	require.Len(t, result.Series[0].Points, 60)
	require.Equal(t, rows[0].Points[1].Value, result.Series[0].Points[0].Value)
	stats, err := engine.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Series)
	require.Equal(t, 240, stats.Samples)
	require.NoError(t, engine.Close())
	reopened, err := OpenEmbedded(ctx, control, path, 15)
	require.NoError(t, err)
	defer reopened.Close()
	after, err := reopened.Query(ctx, query)
	require.NoError(t, err)
	require.Equal(t, result, after)
	metrics, err := reopened.Metrics(ctx)
	require.NoError(t, err)
	require.Equal(t, []Metric{{Name: "game_counter_total", SeriesCount: 2}}, metrics)
}

func TestEmbeddedInvalidBatchRollsBackAndRetainsCounterResetSemantics(t *testing.T) {
	_, engine, _ := embeddedFixture(t)
	at := time.Now().Add(-time.Minute).Truncate(time.Second).UnixMilli()
	base := []model.Sample{{Name: "requests_total", Timestamp: at, Value: 10}, {Name: "requests_total", Timestamp: at + 10000, Value: 20}, {Name: "requests_total", Timestamp: at + 20000, Value: 2}}
	require.NoError(t, engine.Ingest(t.Context(), base))
	require.NoError(t, engine.Ingest(t.Context(), base[2:]), "an identical retransmission is idempotent")
	err := engine.Ingest(t.Context(), []model.Sample{{Name: "must_not_commit", Timestamp: at + 30000, Value: 1}, {Name: "requests_total", Timestamp: at + 20000, Value: 99}})
	require.Error(t, err)
	rows, err := engine.SelectSeries(t.Context(), at, at+60000, []Matcher{{Name: "__name__", Type: "=", Value: "must_not_commit"}})
	require.NoError(t, err)
	require.Empty(t, rows)
	result, err := engine.Query(t.Context(), model.Query{Metric: "requests_total", Start: at, End: at + 30000, Step: 10000, Aggregation: "rate"})
	require.NoError(t, err)
	require.Len(t, result.Series, 1)
	require.Equal(t, 1.0, result.Series[0].Points[0].Value)
	require.Equal(t, .2, result.Series[0].Points[1].Value)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, engine.Ingest(ctx, base), context.Canceled)
}

func TestEmbeddedRefusesForeignOwnerAndUnmigratedLegacy(t *testing.T) {
	control, engine, path := embeddedFixture(t)
	require.NoError(t, engine.Close())
	control.organizationID = 2
	_, err := OpenEmbedded(t.Context(), control, path, 15)
	require.ErrorContains(t, err, "owner")
	control.organizationID = 1
	require.NoError(t, control.Ingest(t.Context(), []model.Sample{{Name: "legacy", Value: 1}}))
	_, err = OpenEmbedded(t.Context(), control, filepath.Join(t.TempDir(), "new"), 15)
	require.ErrorContains(t, err, "offline storage migrate")
}

func TestEmbeddedRetentionPrunesDataAndVectorsTogether(t *testing.T) {
	_, engine, _ := embeddedFixture(t)
	at := time.Now().Add(-time.Hour).UnixMilli()
	require.NoError(t, engine.Ingest(t.Context(), []model.Sample{{Name: "old", Timestamp: at, Value: 1}, {Name: "recent", Timestamp: at + 30000, Value: 2}}))
	pattern := model.Pattern{Metric: "old", Start: at - 64000, End: at, Normalization: "shape", Aggregation: "last", Coverage: 1, Values: make([]float32, 64)}
	require.NoError(t, engine.SavePatterns(t.Context(), []model.Pattern{pattern}))
	require.NoError(t, engine.Prune(t.Context(), at+1))
	rows, err := engine.LoadSeries(t.Context(), at-1, at+60000, nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "recent", rows[0].Name)
	patterns, err := engine.Patterns(t.Context(), 10)
	require.NoError(t, err)
	require.Empty(t, patterns)
}

func TestEmbeddedVersionedRetentionAndControlSeparation(t *testing.T) {
	control, engine, _ := embeddedFixture(t)
	control.Backend = engine
	now := time.Now()
	old := now.Add(-2 * 24 * time.Hour).UnixMilli()
	require.NoError(t, control.Ingest(t.Context(), []model.Sample{{Name: "old", Timestamp: old, Value: 1}, {Name: "recent", Timestamp: now.UnixMilli(), Value: 2}}))
	value, err := control.SaveMetricRetention(t.Context(), MetricRetention{Days: 1})
	require.NoError(t, err)
	require.Equal(t, int64(1), value.Version)
	_, err = control.SaveMetricRetention(t.Context(), MetricRetention{Days: 3})
	require.ErrorIs(t, err, ErrRetentionConflict)
	rows, err := engine.LoadSeries(t.Context(), old-1, now.UnixMilli(), nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "recent", rows[0].Name)
	require.NoError(t, control.PruneMetricRetention(t.Context(), 15, now))
	var count int
	require.NoError(t, control.DB.QueryRow(`SELECT count(*) FROM samples`).Scan(&count))
	require.Zero(t, count, "time series never enter the control row store")
	require.NoError(t, engine.Close())
	require.NoError(t, engine.Close())
}
