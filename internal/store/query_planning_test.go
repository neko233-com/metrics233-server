package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	promlabels "github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

func queryPlanStore(tb testing.TB) *Store {
	tb.Helper()
	s, err := Open(filepath.Join(tb.TempDir(), "query.db"))
	require.NoError(tb, err)
	tb.Cleanup(func() { require.NoError(tb, s.DB.Close()) })
	return s
}

// Query-only fixture generation deliberately bypasses ingestion validation and
// throughput. All values are persisted into the real SQLite samples table.
func fillQuerySeries(tb testing.TB, s *Store, labels string, count int, start, interval int64) int64 {
	tb.Helper()
	result, err := s.DB.Exec(`INSERT INTO series(name,labels) VALUES('query_probe',?)`, labels)
	require.NoError(tb, err)
	id, err := result.LastInsertId()
	require.NoError(tb, err)
	_, err = s.DB.Exec(`WITH RECURSIVE seq(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM seq WHERE i+1<?)
INSERT INTO samples(series_id,timestamp,value) SELECT ?,?+i*?,i%1000 FROM seq`, count, id, start, interval)
	require.NoError(tb, err)
	return id
}

func TestSQLiteNativeBucketPlanningHandlesDenseRangesAndOverwrites(t *testing.T) {
	s := queryPlanStore(t)
	const start = int64(1_700_000_000_000)
	id := fillQuerySeries(t, s, `{"service":"api"}`, 300_001, start, 100)
	q := model.Query{Metric: "query_probe", Start: start, End: start + 30_000_000, Step: 30_000, Aggregation: "last"}
	result, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, result.Series, 1)
	require.Len(t, result.Series[0].Points, 1001)
	require.Equal(t, model.Point{Timestamp: start, Value: 299}, result.Series[0].Points[0])
	_, err = s.DB.Exec(`UPDATE samples SET value=12345 WHERE series_id=? AND timestamp=?`, id, start+29_900)
	require.NoError(t, err)
	result, err = s.Query(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, float64(12345), result.Series[0].Points[0].Value)
	q.Aggregation = "rate"
	_, err = s.Query(context.Background(), q)
	require.ErrorContains(t, err, "250000 samples", "rate must not silently replace raw counter observations with a rollup")
}

func TestSQLiteNativeMinMaxKeepCrossSeriesLastValueContract(t *testing.T) {
	s := queryPlanStore(t)
	ctx := context.Background()
	for _, item := range []struct {
		label  string
		values []float64
	}{{"a", []float64{1, 999, 2}}, {"b", []float64{3, -999, 4}}} {
		for i, at := range []int64{1000, 1100, 1900} {
			require.NoError(t, s.Ingest(ctx, []model.Sample{{Name: "signal", Labels: map[string]string{"host": item.label}, Timestamp: at, Value: item.values[i]}}))
		}
	}
	for agg, expected := range map[string]float64{"min": 2, "max": 4, "sum": 6, "avg": 3} {
		result, err := s.Query(ctx, model.Query{Metric: "signal", Start: 1000, End: 2000, Step: 1000, Aggregation: agg})
		require.NoError(t, err)
		require.Equal(t, []model.Point{{Timestamp: 1000, Value: expected}}, result.Series[0].Points)
	}
	raw, err := s.LoadSeries(ctx, 1000, 2000, []Matcher{{Name: "__name__", Type: "=", Value: "signal"}})
	require.NoError(t, err)
	require.Equal(t, float64(999), raw[0].Points[1].Value)
	require.Equal(t, float64(-999), raw[1].Points[1].Value)
}

func TestSQLiteRawPlanningMatchesLabelsBeforeSampleBudget(t *testing.T) {
	s := queryPlanStore(t)
	const start = int64(1_700_000_000_000)
	fillQuerySeries(t, s, `{"environment":"test"}`, 1_000_001, start, 1)
	fillQuerySeries(t, s, `{"environment":"production"}`, 3, start, 1000)
	ctx := context.Background()
	for _, matcher := range []Matcher{{Name: "environment", Type: "=", Value: "production"}, {Name: "environment", Type: "=~", Value: "prod.*"}, {Name: "environment", Type: "!=", Value: "test"}, {Name: "environment", Type: "!~", Value: "test"}} {
		raw, err := s.LoadSeries(ctx, start, start+1_000_001, []Matcher{{Name: "__name__", Type: "=", Value: "query_probe"}, matcher})
		require.NoError(t, err)
		require.Len(t, raw, 1)
		require.Len(t, raw[0].Points, 3)
		require.Equal(t, "production", raw[0].Labels["environment"])
	}
	_, err := s.LoadSeries(ctx, start, start+1_000_001, nil)
	require.ErrorContains(t, err, "1000000 samples")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.LoadSeries(ctx, start, start+1_000_001, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSQLiteNativePlanBoundsSeriesAndUsesExistingSampleKey(t *testing.T) {
	s := queryPlanStore(t)
	for i := 0; i < 201; i++ {
		fillQuerySeries(t, s, fmt.Sprintf(`{"host":"%d"}`, i), 1, 1000, 1000)
	}
	q := model.Query{Metric: "query_probe", Start: 1000, End: 2000, Step: 1000, Aggregation: "sum"}
	_, err := s.Query(context.Background(), q)
	require.ErrorContains(t, err, "200 series")
	q.Labels = map[string]string{"host": "0"}
	result, err := s.Query(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, result.Series, 1)
	where, args := sqliteSampleRange([]int64{1}, 1000, 2000)
	args = append([]any{int64(1000), int64(1000)}, args...)
	args = append(args, int64(1000), int64(1000))
	rows, err := s.DB.Query(`EXPLAIN QUERY PLAN `+sqliteBucketSQL(where), args...)
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	joined := strings.Join(plan, "\n")
	require.Contains(t, joined, "SEARCH samples USING PRIMARY KEY")
	require.NotContains(t, joined, "SCAN samples")
	t.Log(joined)
}

func TestSQLiteNativeBucketPlanRejectsOversizedScanBeforeReduction(t *testing.T) {
	s := queryPlanStore(t)
	const start = int64(1_700_000_000_000)
	fillQuerySeries(t, s, `{}`, maxSQLiteBucketSamples+1, start, 1)
	_, err := s.Query(context.Background(), model.Query{Metric: "query_probe", Start: start, End: start + int64(maxSQLiteBucketSamples), Step: 10_000, Aggregation: "last"})
	require.ErrorContains(t, err, "5000000 samples")
	// No stale rollup survives retention: planning reads the same durable source.
	require.NoError(t, s.Prune(context.Background(), start+int64(maxSQLiteBucketSamples)))
	result, err := s.Query(context.Background(), model.Query{Metric: "query_probe", Start: start, End: start + int64(maxSQLiteBucketSamples), Step: 10_000, Aggregation: "last"})
	require.NoError(t, err)
	require.Len(t, result.Series, 1)
	require.Equal(t, []model.Point{{Timestamp: start + int64(maxSQLiteBucketSamples), Value: float64(maxSQLiteBucketSamples % 1000)}}, result.Series[0].Points)
}

func TestSQLiteRawMatchersKeepUpstreamNewlineAnchorAndAbsentLabelSemantics(t *testing.T) {
	s := queryPlanStore(t)
	ctx := context.Background()
	samples := []model.Sample{
		{Name: "multiline", Labels: map[string]string{"value": "a\nb"}, Timestamp: 1000, Value: 1},
		{Name: "plain", Labels: map[string]string{"value": "axxb"}, Timestamp: 1000, Value: 1},
		{Name: "prefix", Labels: map[string]string{"value": "prefixaxxb"}, Timestamp: 1000, Value: 1},
		{Name: "missing", Timestamp: 1000, Value: 1},
		{Name: "empty", Labels: map[string]string{"value": ""}, Timestamp: 1000, Value: 1},
		{Name: "line_end", Labels: map[string]string{"value": "a\n"}, Timestamp: 1000, Value: 1},
	}
	require.NoError(t, s.Ingest(ctx, samples))
	for _, check := range []struct {
		name, pattern string
		op            promlabels.MatchType
		wanted        []string
	}{
		{"newline-dot", "a.b", promlabels.MatchRegexp, []string{"multiline"}},
		{"newline-dot-star", "a.*b", promlabels.MatchRegexp, []string{"multiline", "plain"}},
		{"whole-string-anchor", "a", promlabels.MatchRegexp, nil},
		{"explicit-no-dotall", "(?-s:a.*b)", promlabels.MatchRegexp, []string{"plain"}},
		{"negative-newline", "a.*b", promlabels.MatchNotRegexp, []string{"prefix", "missing", "empty", "line_end"}},
		{"missing-label-empty-match", ".*", promlabels.MatchRegexp, []string{"multiline", "plain", "prefix", "missing", "empty", "line_end"}},
		{"negative-missing-label", ".+", promlabels.MatchNotRegexp, []string{"missing", "empty"}},
	} {
		t.Run(check.name, func(t *testing.T) {
			wanted := make(map[string]bool, len(check.wanted))
			for _, name := range check.wanted {
				wanted[name] = true
			}
			upstream, err := promlabels.NewMatcher(check.op, "value", check.pattern)
			require.NoError(t, err)
			matchers := []Matcher{{Name: "value", Type: check.op.String(), Value: check.pattern}}
			for _, sample := range samples {
				require.Equal(t, wanted[sample.Name], upstream.Matches(sample.Labels["value"]), "upstream oracle for %s", sample.Name)
				require.Equal(t, wanted[sample.Name], Matches(sample.Name, sample.Labels, matchers), "shared Go matcher for %s", sample.Name)
			}
			for _, load := range []func(context.Context, int64, int64, []Matcher) ([]RawSeries, error){s.SelectSeries, s.LoadSeries} {
				selected, err := load(ctx, 0, 2000, matchers)
				require.NoError(t, err)
				actual := make(map[string]bool, len(selected))
				for _, series := range selected {
					actual[series.Name] = true
				}
				require.Equal(t, wanted, actual)
			}
		})
	}
	invalid := []Matcher{{Name: "value", Type: "=~", Value: "["}}
	require.False(t, Matches("plain", samples[1].Labels, invalid))
	_, err := s.SelectSeries(ctx, 0, 2000, invalid)
	require.Error(t, err)
	_, err = s.LoadSeries(ctx, 0, 2000, invalid)
	require.Error(t, err)
}

func BenchmarkSQLiteQueryPlan30Days1M(b *testing.B) {
	s := queryPlanStore(b)
	const start = int64(1_700_000_000_000)
	const span = int64(30 * 24 * 60 * 60 * 1000)
	fillQuerySeries(b, s, `{"service":"api"}`, 1_000_000, start, span/1_000_000)
	q := model.Query{Metric: "query_probe", Start: start, End: start + span, Step: 30 * 60 * 1000, Aggregation: "last"}
	b.Run("SQLBucketLast", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result, err := s.Query(context.Background(), q)
			require.NoError(b, err)
			require.Len(b, result.Series[0].Points, 1440)
		}
	})
	b.Run("UncappedGoBucketReference", func(b *testing.B) {
		// The old production path rejected above 250k rows. This uncapped
		// reference measures its scan/driver/Go-bucketing pattern at 1M.
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rows, err := s.DB.Query(`SELECT series.id,series.labels,samples.timestamp,samples.value FROM series JOIN samples ON samples.series_id=series.id WHERE series.name=? AND samples.timestamp>=? AND samples.timestamp<=? ORDER BY series.id,samples.timestamp`, q.Metric, q.Start, q.End)
			require.NoError(b, err)
			buckets := map[int64]float64{}
			for rows.Next() {
				var id, at int64
				var labels string
				var value float64
				require.NoError(b, rows.Scan(&id, &labels, &at, &value))
				buckets[(at-start)/q.Step] = value
			}
			require.NoError(b, rows.Err())
			require.NoError(b, rows.Close())
			require.Len(b, buckets, 1440)
		}
	})
}
