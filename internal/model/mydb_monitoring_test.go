package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

func TestMyDBPlanUsesObservedMetricsAndSafeRecovery(t *testing.T) {
	input := setupInput("mydb")
	samples := []Sample{{Name: "mydb_up", Value: 1}, {Name: "mydb_connections_active", Value: 2}, {Name: "mydb_max_connections", Value: 100}}
	for _, spec := range myDBPanels {
		samples = append(samples, Sample{Name: spec.metric, Value: 1})
	}
	samples = append(samples, Sample{Name: "mydb_query_latency_microseconds_bucket", Value: 1, Labels: map[string]string{"le": "1000"}}, Sample{Name: "mydb_wal_sync_microseconds_total", Value: 10})
	plan, err := BuildConnectionPlan(input, samples)
	require.NoError(t, err)
	require.Equal(t, "mydb", plan.Profile)
	require.Len(t, plan.Rules, 11)
	require.Len(t, plan.Dashboard.Panels, 24)
	for _, panel := range plan.Dashboard.Panels {
		require.Contains(t, panel.Expr, ConnectionLabel)
		_, err = parser.NewParser(parser.Options{}).ParseExpr(panel.Expr)
		require.NoError(t, err, panel.Expr)
	}
	authRecoveryChecked := false
	for _, rule := range plan.Rules {
		require.Equal(t, 3600, rule.NotificationCooldownSeconds)
		require.Equal(t, "KeepLast", rule.NoDataState)
		require.Contains(t, string(rule.Grafana), "timestamp(")
		if strings.HasSuffix(rule.UID, "-mydb-auth") {
			require.Contains(t, string(rule.Grafana), "0.000001")
			authRecoveryChecked = true
		}
		var graph map[string]any
		require.NoError(t, json.Unmarshal(rule.Grafana, &graph))
	}
	require.True(t, authRecoveryChecked)
	// An older MyDB export must not acquire fictitious security panels/rules.
	plan, err = BuildConnectionPlan(input, []Sample{{Name: "mydb_up", Value: 1}})
	require.NoError(t, err)
	require.Len(t, plan.Rules, 1)
	require.Len(t, plan.Dashboard.Panels, 2)
}
