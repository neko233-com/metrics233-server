package store_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func connectionFixture(t *testing.T) (model.ConnectionSetupInput, model.ConnectionPlan) {
	t.Helper()
	input := model.ConnectionSetupInput{ID: "23300000-0000-4000-8000-000000000003", Target: model.Target{Name: "connection fixture", Kind: "prometheus", URL: "http://127.0.0.1:19100/metrics", IntervalSeconds: 15, Enabled: true, SecureSettings: map[string]string{"password": "synthetic-protected-connection"}}, Language: "zh", Alerts: true}
	plan, err := model.BuildConnectionPlan(input, []model.Sample{{Name: "fixture_value", Value: 233}})
	require.NoError(t, err)
	return input, plan
}

func TestConnectionSetupAtomicSecretsDurableRetryAndNoClobber(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := store.Open(path)
	require.NoError(t, err)
	input, plan := connectionFixture(t)
	_, err = db.DB.Exec(`CREATE TRIGGER fail_connection_rule BEFORE INSERT ON alert_rules BEGIN SELECT RAISE(FAIL,'synthetic transactional failure'); END`)
	require.NoError(t, err)
	_, err = db.ApplyConnectionSetup(ctx, input, plan)
	require.Error(t, err)
	targets, err := db.Targets(ctx)
	require.NoError(t, err)
	require.Empty(t, targets)
	boards, err := db.Dashboards(ctx)
	require.NoError(t, err)
	require.Len(t, boards, 1)
	receipts, err := db.ConnectionSetups(ctx)
	require.NoError(t, err)
	require.Empty(t, receipts)
	_, err = db.DB.Exec(`DROP TRIGGER fail_connection_rule`)
	require.NoError(t, err)
	receipt, err := db.ApplyConnectionSetup(ctx, input, plan)
	require.NoError(t, err)
	require.Len(t, receipt.RuleUIDs, 1)
	secrets, err := db.TargetSecrets(ctx, receipt.TargetID)
	require.NoError(t, err)
	require.Equal(t, input.Target.SecureSettings, secrets)
	var stored string
	require.NoError(t, db.DB.QueryRow(`SELECT receipt FROM connection_setups`).Scan(&stored))
	require.NotContains(t, stored, "synthetic-protected-connection")
	var sealed []byte
	require.NoError(t, db.DB.QueryRow(`SELECT secrets FROM target_connections WHERE id=?`, receipt.TargetID).Scan(&sealed))
	require.NotContains(t, string(sealed), "synthetic-protected-connection")
	require.NoError(t, db.Close())
	db, err = store.Open(path)
	require.NoError(t, err)
	defer db.Close()
	retry, err := db.ConnectionRetry(ctx, input)
	require.NoError(t, err)
	require.Equal(t, receipt, retry)
	board, err := db.Dashboard(ctx, receipt.DashboardID)
	require.NoError(t, err)
	board.Name = "Operator custom title"
	_, err = db.SaveDashboard(ctx, board)
	require.NoError(t, err)
	retry, err = db.ApplyConnectionSetup(ctx, input, plan)
	require.NoError(t, err)
	require.Equal(t, receipt, retry)
	board, err = db.Dashboard(ctx, receipt.DashboardID)
	require.NoError(t, err)
	require.Equal(t, "Operator custom title", board.Name)
	input.Target.SecureSettings["password"] = "changed-synthetic-password"
	_, err = db.ConnectionRetry(ctx, input)
	require.ErrorIs(t, err, store.ErrConnectionConflict)
	input.Target.SecureSettings["password"] = "synthetic-protected-connection"
	require.NoError(t, db.DeleteDashboard(ctx, receipt.DashboardID))
	_, err = db.ConnectionRetry(ctx, input)
	require.ErrorIs(t, err, store.ErrConnectionConflict)
	_, err = db.ApplyConnectionSetup(ctx, input, plan)
	require.ErrorIs(t, err, store.ErrConnectionConflict)
}

func TestConnectionSetupConcurrentRetriesCreateExactlyOneBundle(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	defer db.Close()
	input, plan := connectionFixture(t)
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	receipts := make(chan model.ConnectionReceipt, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := db.ApplyConnectionSetup(ctx, input, plan)
			errors <- err
			receipts <- r
		}()
	}
	wg.Wait()
	close(errors)
	close(receipts)
	for err := range errors {
		require.NoError(t, err)
	}
	var first model.ConnectionReceipt
	for receipt := range receipts {
		if first.ID == "" {
			first = receipt
		}
		require.Equal(t, first, receipt)
	}
	targets, err := db.Targets(ctx)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	rules, err := db.AlertRules(ctx)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	input.ID = "23300000-0000-4000-8000-000000000004"
	plan, err = model.BuildConnectionPlan(input, []model.Sample{{Name: "fixture_value", Value: 1}})
	require.NoError(t, err)
	_, err = db.ApplyConnectionSetup(ctx, input, plan)
	require.ErrorIs(t, err, store.ErrConnectionConflict)
	raw, err := json.Marshal(targets)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "synthetic-protected-connection")
}
