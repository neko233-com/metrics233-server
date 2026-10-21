package store

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

// Opt-in against a disposable ClickHouse HTTP endpoint. Only this test's
// randomly named database is created/dropped; no application database is used.
func TestClickHouseQueryPlanningMillionSamples(t *testing.T) {
	endpoint := os.Getenv("METRICSPANEL_QUERY_CLICKHOUSE_URL")
	if endpoint == "" {
		t.Skip("set METRICSPANEL_QUERY_CLICKHOUSE_URL to a disposable ClickHouse HTTP endpoint")
	}
	ctx := context.Background()
	database := "queryplan_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	cleanupBackend := &ClickHouse{URL: endpoint, Database: database, User: "default", Client: &http.Client{Timeout: 30 * time.Second}}
	t.Cleanup(func() {
		_, err := cleanupBackend.execute(context.Background(), "DROP DATABASE IF EXISTS "+database, nil, nil, false)
		require.NoError(t, err)
	})
	c, err := OpenClickHouse(ctx, endpoint, database, "default", "", 365)
	require.NoError(t, err)
	const span = int64(30 * 24 * 60 * 60 * 1000)
	start := time.Now().UnixMilli() - span
	params := map[string]string{"start": strconv.FormatInt(start, 10)}
	_, err = c.execute(ctx, `INSERT INTO `+database+`.samples SELECT 'query_probe','{"host":"api"}',map('host','api'),{start:Int64}+toInt64(number)*2592,toFloat64(number%1000),toUInt64(1) FROM numbers(1000000)`, params, nil, false)
	require.NoError(t, err)
	q := model.Query{Metric: "query_probe", Start: start, End: start + span, Step: 30 * 60 * 1000, Aggregation: "last"}
	for i := 0; i < 3; i++ {
		started := time.Now()
		result, err := c.Query(ctx, q)
		require.NoError(t, err)
		require.Len(t, result.Series, 1)
		require.Len(t, result.Series[0].Points, 1440)
		t.Logf("native bucket last, 30 days/1M samples, run=%d: %s, points=%d", i+1, time.Since(started), len(result.Series[0].Points))
	}
	// Versioned corrections must win before temporal aggregation, even before
	// asynchronous ReplacingMergeTree background merges have deduplicated them.
	at := start + 543210*2592
	_, err = c.execute(ctx, `INSERT INTO `+database+`.samples VALUES ('query_probe','{"host":"api"}',map('host','api'),`+strconv.FormatInt(at, 10)+`,99999,2)`, nil, nil, false)
	require.NoError(t, err)
	raw, err := c.LoadSeries(ctx, at-1, at+1, []Matcher{{Name: "__name__", Type: "=", Value: "query_probe"}})
	require.NoError(t, err)
	require.Len(t, raw, 1)
	require.Equal(t, []model.Point{{Timestamp: at, Value: 99999}}, raw[0].Points)
	window := `timestamp>=` + strconv.FormatInt(start+span-5*60*1000, 10) + ` AND timestamp<=` + strconv.FormatInt(start+span, 10)
	read := `SELECT count() FROM ` + database + `.samples FINAL WHERE ` + window
	before, err := c.execute(ctx, `EXPLAIN indexes=1 `+read, nil, nil, false)
	require.NoError(t, err)
	_, err = c.execute(ctx, `ALTER TABLE `+database+`.samples ADD INDEX IF NOT EXISTS timestamp_minmax timestamp TYPE minmax GRANULARITY 1`, nil, nil, false)
	require.NoError(t, err)
	_, err = c.execute(ctx, `ALTER TABLE `+database+`.samples MATERIALIZE INDEX timestamp_minmax SETTINGS mutations_sync=1`, nil, nil, false)
	require.NoError(t, err)
	after, err := c.execute(ctx, `EXPLAIN indexes=1 `+read, nil, nil, false)
	require.NoError(t, err)
	t.Logf("5-minute FINAL range plan before index:\n%s\nafter timestamp_minmax:\n%s", before, after)
	for _, setting := range []string{"use_skip_indexes=0", "use_skip_indexes=1"} {
		for i := 0; i < 3; i++ {
			started := time.Now()
			data, err := c.execute(ctx, read+` SETTINGS `+setting, nil, nil, false)
			require.NoError(t, err)
			require.Equal(t, "115", strings.TrimSpace(string(data)))
			t.Logf("5-minute range %s run=%d: %s", setting, i+1, time.Since(started))
		}
	}
	ddl, err := c.execute(ctx, `SHOW CREATE TABLE `+database+`.samples`, nil, nil, false)
	require.NoError(t, err)
	require.Contains(t, string(ddl), "365")
	t.Log(fmt.Sprintf("ClickHouse fixture database %s dropped by test cleanup; TTL unchanged by query planning", database))
}
