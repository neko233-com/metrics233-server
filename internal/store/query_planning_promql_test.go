package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/promcompat"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/require"
)

func TestSQLiteQueryPlanPreservesRawPromQLCounterResets(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	points := []model.Sample{
		{Name: "query_plan_counter", Timestamp: 1000, Value: 10},
		{Name: "query_plan_counter", Timestamp: 1100, Value: 100},
		{Name: "query_plan_counter", Timestamp: 1900, Value: 20},
		{Name: "query_plan_counter", Timestamp: 2000, Value: 25},
		{Name: "query_plan_counter", Timestamp: 2999, Value: 30},
		{Name: "query_plan_counter", Timestamp: 3000, Value: 5},
	}
	require.NoError(t, s.Ingest(ctx, points))
	result, err := s.Query(ctx, model.Query{Metric: "query_plan_counter", Start: 1000, End: 3000, Step: 1000, Aggregation: "last"})
	require.NoError(t, err)
	require.Len(t, result.Series, 1)
	require.Equal(t, []model.Point{{Timestamp: 1000, Value: 20}, {Timestamp: 2000, Value: 30}, {Timestamp: 3000, Value: 5}}, result.Series[0].Points)
	raw, err := s.LoadSeries(ctx, 1000, 3000, []store.Matcher{{Name: "__name__", Type: "=", Value: "query_plan_counter"}})
	require.NoError(t, err)
	require.Len(t, raw, 1)
	require.Len(t, raw[0].Points, len(points))
	api := promcompat.New(s)
	query, err := api.Engine.NewInstantQuery(ctx, api.Queryable, promql.NewPrometheusQueryOpts(false, 0), "resets(query_plan_counter[3s])", time.UnixMilli(3000))
	require.NoError(t, err)
	defer query.Close()
	evaluated := query.Exec(ctx)
	require.NoError(t, evaluated.Err)
	vector, ok := evaluated.Value.(promql.Vector)
	require.True(t, ok)
	require.Len(t, vector, 1)
	require.Equal(t, float64(2), vector[0].F, "the reset inside the first display bucket must remain visible to upstream PromQL")
}
