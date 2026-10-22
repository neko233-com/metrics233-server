package collector

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestFailedMySQLScrapePublishesAvailabilityZero(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer db.DB.Close()
	target, err := db.SaveTarget(context.Background(), model.Target{Name: "unreachable-fixture", Kind: "mysql", URL: "mysql://" + address, IntervalSeconds: 30})
	require.NoError(t, err)
	_, err = New(db).Scrape(context.Background(), target)
	require.Error(t, err)
	data, err := db.Query(context.Background(), model.Query{Metric: "mysql_up", Start: time.Now().Add(-time.Minute).UnixMilli(), End: time.Now().UnixMilli() + 1, Step: 1000, Aggregation: "last"})
	require.NoError(t, err)
	require.Len(t, data.Series, 1)
	require.Equal(t, 0.0, data.Series[0].Points[0].Value)
}
