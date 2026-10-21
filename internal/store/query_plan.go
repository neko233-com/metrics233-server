package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/neko233-com/metrics233-server/internal/model"
)

const (
	maxNativeSeries        = 200
	maxSQLiteBucketSamples = 5_000_000
	maxSQLiteRateSamples   = 250_000
)

type metricQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Resolve labels before touching sample values. EXISTS uses the existing
// (series_id,timestamp) primary key; cold/out-of-range series do not use a slot.
func nativeSeriesIDs(ctx context.Context, db metricQueryer, q model.Query, from int64) ([]int64, error) {
	query := `SELECT id FROM series WHERE name=? AND EXISTS(SELECT 1 FROM samples WHERE series_id=series.id AND timestamp>=? AND timestamp<=?)`
	args := []any{q.Metric, from, q.End}
	keys := make([]string, 0, len(q.Labels))
	for key := range q.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		query += ` AND EXISTS(SELECT 1 FROM json_each(series.labels) WHERE key=? AND value=?)`
		args = append(args, key, q.Labels[key])
	}
	rows, err := db.QueryContext(ctx, query+` ORDER BY id LIMIT 201`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		if len(ids) >= maxNativeSeries {
			return nil, errors.New("query exceeds 200 series; add label filters")
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func sqliteSampleRange(ids []int64, start, end int64) (string, []any) {
	args := make([]any, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, start, end)
	return `series_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + `) AND timestamp>=? AND timestamp<=?`, args
}

// SQL does the temporal last-value reduction before rows cross the driver.
// Joining each group's max timestamp to the primary key gives its exact value;
// this does not reinterpret the API's cross-series min/max as temporal extrema.
func sqliteBucketSQL(where string) string {
	return `WITH buckets AS (SELECT series_id,(timestamp-?)/? AS bucket,max(timestamp) AS last_timestamp FROM samples WHERE ` + where + ` GROUP BY series_id,bucket)
SELECT series.id,series.labels,?+buckets.bucket*?,samples.value FROM buckets
JOIN samples ON samples.series_id=buckets.series_id AND samples.timestamp=buckets.last_timestamp
JOIN series ON series.id=buckets.series_id ORDER BY series.id,buckets.bucket`
}
