package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

// TestBusinessDirectoryAcceptsCenterRole guards the center role introduced by
// the game backend: center instances must be catalogued, counted and filterable
// exactly like gateway/sdk rather than silently dropped at ingest.
func TestBusinessDirectoryAcceptsCenterRole(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	require.NoError(t, err)
	defer s.DB.Close()
	ctx := context.Background()
	samples := []model.Sample{
		{Name: "fixture", Value: 0, Labels: map[string]string{"service": "sfgo-center", "role": "center", "serverId": "center", "instance": "center-wechat-test", "environment": "test", "channel": "wechat", "implementation": "go"}},
		{Name: "fixture", Value: 0, Labels: map[string]string{"service": "sfgo-game", "role": "logic", "serverId": "10001", "instance": "logic-wechat-test", "environment": "test", "channel": "wechat", "implementation": "go"}},
	}
	require.NoError(t, s.RecordBusinessInstances(ctx, samples, ""))

	page, err := s.BusinessDirectory(ctx, BusinessDirectoryFilter{Limit: 10, Sort: "server"})
	require.NoError(t, err)
	require.Equal(t, 2, page.Total)
	require.Equal(t, 1, page.Counts["center"])
	require.Equal(t, 1, page.Counts["game"])

	page, err = s.BusinessDirectory(ctx, BusinessDirectoryFilter{Limit: 10, Sort: "server", Role: "center"})
	require.NoError(t, err)
	require.Equal(t, 1, page.Matched)
	require.Equal(t, "center", page.Items[0].Identity.Role)

	_, err = s.BusinessDirectory(ctx, BusinessDirectoryFilter{Limit: 10, Sort: "server", Role: "unknown-role"})
	require.Error(t, err, "unsupported roles must still fail closed")
}
