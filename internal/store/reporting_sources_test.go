package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReportingObservationEvictionNeverEvictsPolicy(t *testing.T) {
	s, err := Open(t.TempDir() + "/sources.db")
	require.NoError(t, err)
	defer s.DB.Close()
	ctx := context.Background()
	view, err := s.ReportingSources(ctx)
	require.NoError(t, err)
	require.NoError(t, s.UpdateReportingPolicy(ctx, ReportingPolicyUpdate{Version: view.Version, Address: "192.0.2.1", Action: "block"}))
	_, err = s.DB.Exec(`WITH RECURSIVE n(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<1023) INSERT INTO metric_reporting_observations(address,first_seen,last_seen,accepted_batches,accepted_samples,protocol_mask,last_service) SELECT printf('192.0.%d.%d',x/256,x%256),1,x+1,1,1,1,'' FROM n`)
	require.NoError(t, err)
	require.NoError(t, s.RecordReportingSource(ctx, "198.51.100.21", "json", "shop", 7))
	var count int
	require.NoError(t, s.DB.QueryRow(`SELECT count(*) FROM metric_reporting_observations`).Scan(&count))
	require.Equal(t, ReportingSourceLimit, count)
	allowed, err := s.ReportingSourceAllowed(ctx, "192.0.2.1")
	require.NoError(t, err)
	require.False(t, allowed)
	require.NoError(t, s.RecordReportingSource(ctx, "198.51.100.21", "otlp", "shop", 3))
	view, err = s.ReportingSources(ctx)
	require.NoError(t, err)
	require.Equal(t, "198.51.100.21", view.Sources[0].Address)
	require.EqualValues(t, 2, view.Sources[0].AcceptedBatches)
	require.EqualValues(t, 10, view.Sources[0].AcceptedSamples)
	require.ElementsMatch(t, []string{"json", "otlp"}, view.Sources[0].Protocols)
	// A full policy registry rejects new entries atomically; existing blocks
	// remain editable and cannot be evicted by a stream of new reporters.
	_, err = s.DB.Exec(`WITH RECURSIVE n(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<4094) INSERT INTO metric_reporting_policy(address,disposition) SELECT printf('198.18.%d.%d',x/256,x%256),'block' FROM n`)
	require.NoError(t, err)
	require.ErrorContains(t, s.UpdateReportingPolicy(ctx, ReportingPolicyUpdate{Version: view.Version, Address: "203.0.113.2", Action: "allow"}), "capacity")
	after, err := s.ReportingSources(ctx)
	require.NoError(t, err)
	require.Equal(t, view.Version, after.Version)
	require.NoError(t, s.UpdateReportingPolicy(ctx, ReportingPolicyUpdate{Version: view.Version, Address: "192.0.2.1", Action: "allow"}))
}
