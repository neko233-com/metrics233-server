package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReportingReviewConcurrentObservationAndPolicyCAS(t *testing.T) {
	s, err := Open(t.TempDir() + "/sources.db")
	require.NoError(t, err)
	defer s.DB.Close()
	ctx := context.Background()
	view, err := s.ReportingSources(ctx)
	require.NoError(t, err)
	var workers sync.WaitGroup
	errorsOut := make(chan error, 32)
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			errorsOut <- s.RecordReportingSource(ctx, "192.0.2.17", "json", "synthetic-service", 3)
		}()
	}
	workers.Wait()
	close(errorsOut)
	for err := range errorsOut {
		require.NoError(t, err)
	}
	after, err := s.ReportingSources(ctx)
	require.NoError(t, err)
	require.Len(t, after.Sources, 1)
	require.EqualValues(t, 32, after.Sources[0].AcceptedBatches)
	require.EqualValues(t, 96, after.Sources[0].AcceptedSamples)
	errorsOut = make(chan error, 16)
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			errorsOut <- s.UpdateReportingPolicy(ctx, ReportingPolicyUpdate{Version: view.Version, AllowlistOnly: true, Address: "192.0.2.17", Action: "block"})
		}()
	}
	workers.Wait()
	close(errorsOut)
	winners, conflicts := 0, 0
	for err := range errorsOut {
		if err == nil {
			winners++
		} else {
			require.True(t, errors.Is(err, ErrReportingPolicyConflict))
			conflicts++
		}
	}
	require.Equal(t, 1, winners)
	require.Equal(t, 15, conflicts)
	after, err = s.ReportingSources(ctx)
	require.NoError(t, err)
	require.Equal(t, view.Version+1, after.Version)
	require.True(t, after.AllowlistOnly)
	allowed, err := s.ReportingSourceAllowed(ctx, "192.0.2.17")
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestReportingReviewCapacityFailureCannotDisableAllowlist(t *testing.T) {
	s, err := Open(t.TempDir() + "/sources.db")
	require.NoError(t, err)
	defer s.DB.Close()
	ctx := context.Background()
	view, err := s.ReportingSources(ctx)
	require.NoError(t, err)
	require.NoError(t, s.UpdateReportingPolicy(ctx, ReportingPolicyUpdate{Version: view.Version, AllowlistOnly: true, Address: "192.0.2.17", Action: "block"}))
	_, err = s.DB.Exec(`WITH RECURSIVE n(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<4094) INSERT INTO metric_reporting_policy(address,disposition) SELECT printf('198.18.%d.%d',x/256,x%256),'block' FROM n`)
	require.NoError(t, err)
	view, err = s.ReportingSources(ctx)
	require.NoError(t, err)
	require.True(t, view.AllowlistOnly)
	err = s.UpdateReportingPolicy(ctx, ReportingPolicyUpdate{Version: view.Version, AllowlistOnly: false, Address: "203.0.113.17", Action: "allow"})
	require.ErrorContains(t, err, "capacity")
	after, err := s.ReportingSources(ctx)
	require.NoError(t, err)
	require.Equal(t, view.Version, after.Version)
	require.True(t, after.AllowlistOnly)
	require.Len(t, after.Sources, ReportingPolicyLimit)
	allowed, err := s.ReportingSourceAllowed(ctx, "203.0.113.17")
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = s.ReportingSourceAllowed(ctx, "192.0.2.17")
	require.NoError(t, err)
	require.False(t, allowed)
}
