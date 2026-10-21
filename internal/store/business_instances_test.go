package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBusinessInstancesAcceptedIdentityScopeAndBindingCAS(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	require.NoError(t, err)
	defer s.DB.Close()
	ctx := context.Background()
	labels := map[string]string{"service": "game", "role": "logic", "serverId": "0", "instance": "fixture-0", "environment": "test", "ip": "203.0.113.90"}
	samples := []model.Sample{{Name: "game_players_online", Labels: labels, Value: 3}}
	require.NoError(t, s.Ingest(ctx, samples))
	require.NoError(t, s.RecordBusinessInstances(ctx, samples, "127.0.0.1"))
	page, err := s.BusinessInstances(ctx, "", 50)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	first := page.Items[0]
	require.Equal(t, "0", first.Identity.ServerID)
	require.Equal(t, "203.0.113.90", first.ReportedIP)
	require.Equal(t, "127.0.0.1", first.ObservedIP)
	// Different environments and channels never collapse the same serverId.
	for _, key := range []string{"environment", "channel", "implementation"} {
		copy := map[string]string{}
		for k, v := range labels {
			copy[k] = v
		}
		copy[key] = "different"
		require.NoError(t, s.Ingest(ctx, []model.Sample{{Name: "game_players_online", Labels: copy, Value: 9}}))
	}
	page, err = s.BusinessInstances(ctx, "", 2)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.NotEmpty(t, page.Next)
	second, err := s.BusinessInstances(ctx, page.Next, 2)
	require.NoError(t, err)
	require.Len(t, second.Items, 2)
	require.Empty(t, second.Next)
	require.NotEqual(t, page.Items[1].ID, second.Items[0].ID)
	_, err = s.SaveDashboard(ctx, model.Dashboard{ID: "template", Name: "Template", Panels: []model.Panel{}})
	require.NoError(t, err)
	bound, err := s.BindBusinessDashboard(ctx, first.ID, model.BusinessBinding{Mode: "inherit", DashboardID: "template"})
	require.NoError(t, err)
	require.EqualValues(t, 1, bound.Binding.Version)
	_, err = s.BindBusinessDashboard(ctx, first.ID, model.BusinessBinding{Mode: "default", Version: 0})
	require.ErrorIs(t, err, ErrBusinessBindingConflict)
	_, err = s.BindBusinessDashboard(ctx, first.ID, model.BusinessBinding{Mode: "independent", DashboardID: "missing", Version: 1})
	require.Error(t, err)
	bound, err = s.BindBusinessDashboard(ctx, first.ID, model.BusinessBinding{Mode: "default", Version: 1})
	require.NoError(t, err)
	require.EqualValues(t, 2, bound.Binding.Version)
	// A later collector observation cannot erase the canonical source address.
	require.NoError(t, s.RecordBusinessInstances(ctx, samples, ""))
	fresh, err := s.BusinessInstance(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", fresh.ObservedIP)
}

func TestBusinessInstancesRejectInvalidBatchWithoutObservation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	require.NoError(t, err)
	defer s.DB.Close()
	ctx := context.Background()
	labels := map[string]string{"service": "game", "role": "logic", "serverId": "1", "instance": "fixture"}
	require.Error(t, s.Ingest(ctx, []model.Sample{{Name: "valid", Labels: labels, Value: 1}, {Name: "bad name", Labels: labels, Value: 1}}))
	page, err := s.BusinessInstances(ctx, "", 50)
	require.NoError(t, err)
	require.Empty(t, page.Items)
	for _, role := range []string{"", "unknown"} {
		labels["role"] = role
		require.NoError(t, s.Ingest(ctx, []model.Sample{{Name: "valid", Labels: labels, Value: 1}}))
	}
	page, err = s.BusinessInstances(ctx, "", 50)
	require.NoError(t, err)
	require.Empty(t, page.Items)
}

func TestBusinessInstancesCapacityRefreshAndAuxiliaryFailure(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	require.NoError(t, err)
	defer s.DB.Close()
	ctx := context.Background()
	labels := map[string]string{"service": "fixture", "role": "logic", "serverId": "1", "instance": "capacity"}
	sample := model.Sample{Name: "game_players_online", Labels: labels, Value: 1}
	require.NoError(t, s.RecordBusinessInstances(ctx, []model.Sample{sample}, "127.0.0.1"))
	page, err := s.BusinessInstances(ctx, "", 50)
	require.NoError(t, err)
	id := page.Items[0].ID
	_, err = s.DB.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<4095) INSERT INTO business_instances(id,identity,first_seen,last_seen) SELECT printf('%064d',i),'{}',1,1 FROM n`)
	require.NoError(t, err)
	require.NoError(t, s.RecordBusinessInstances(ctx, []model.Sample{sample}, "203.0.113.10"))
	fresh, err := s.BusinessInstance(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.10", fresh.ObservedIP)
	labels["instance"] = "new-identity"
	require.NoError(t, s.RecordBusinessInstances(ctx, []model.Sample{sample}, ""))
	var count int
	require.NoError(t, s.DB.QueryRow(`SELECT count(*) FROM business_instances`).Scan(&count))
	require.Equal(t, BusinessInstanceLimit, count)
	page, err = s.FindBusinessInstances(ctx, "", "capacity", 50)
	require.NoError(t, err)
	require.True(t, page.AtCapacity)
	require.Len(t, page.Items, 1)
	// Catalog failure cannot turn a committed sample into an ingest retry.
	_, err = s.DB.Exec(`DROP TABLE business_instances`)
	require.NoError(t, err)
	require.NoError(t, s.Ingest(ctx, []model.Sample{sample}))
	stats, err := s.Stats(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.Samples)
}
