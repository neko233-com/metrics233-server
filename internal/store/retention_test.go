package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestMetricRetentionCASPersistsAndPreservesExplicitDefault(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	s, err := store.Open(path)
	require.NoError(t, err)
	value, err := s.MetricRetention(ctx, 15)
	require.NoError(t, err)
	require.Equal(t, store.MetricRetention{Days: 15}, value)
	_, err = s.SaveMetricRetention(ctx, store.MetricRetention{Days: 7, Version: 9})
	require.ErrorIs(t, err, store.ErrRetentionConflict)
	value, err = s.MetricRetention(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, 30, value.Days)
	require.Zero(t, value.Version)
	for _, days := range []int{0, -1, 3651} {
		_, err = s.SaveMetricRetention(ctx, store.MetricRetention{Days: days})
		require.Error(t, err)
	}
	value, err = s.SaveMetricRetention(ctx, store.MetricRetention{Days: 7})
	require.NoError(t, err)
	require.Equal(t, int64(1), value.Version)
	_, err = s.SaveMetricRetention(ctx, store.MetricRetention{Days: 1})
	require.ErrorIs(t, err, store.ErrRetentionConflict)
	require.NoError(t, s.DB.Close())
	s, err = store.Open(path)
	require.NoError(t, err)
	defer s.DB.Close()
	value, err = s.MetricRetention(ctx, 15)
	require.NoError(t, err)
	require.Equal(t, store.MetricRetention{Days: 7, Version: 1}, value)
	value, err = s.SaveMetricRetention(ctx, store.MetricRetention{Days: 3650, Version: 1})
	require.NoError(t, err)
	require.Equal(t, int64(2), value.Version)
	// Switching backend never silently rewrites a ClickHouse TTL or reuses a
	// SQLite-only override. No network call is needed for this configuration check.
	s.Backend = &store.ClickHouse{RetentionDays: 30}
	value, err = s.MetricRetention(ctx, 30)
	require.NoError(t, err)
	require.Equal(t, store.MetricRetention{Days: 30}, value)
	_, err = s.SaveMetricRetention(ctx, store.MetricRetention{Days: 15, Version: 2})
	require.Error(t, err)
	s.Backend = nil
	value, err = s.MetricRetention(ctx, 15)
	require.NoError(t, err)
	require.Equal(t, 3650, value.Days)
}
