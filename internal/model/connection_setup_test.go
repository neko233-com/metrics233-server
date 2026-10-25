package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

func setupInput(kind string) ConnectionSetupInput {
	address := "http://127.0.0.1:9999/metrics"
	if kind == "mysql" {
		address = "mysql://127.0.0.1:3306"
	}
	if kind == "redis" {
		address = "redis://127.0.0.1:6379"
	}
	if kind == "postgresql" {
		address = "postgresql://127.0.0.1:5432"
	}
	return ConnectionSetupInput{ID: "23300000-0000-4000-8000-000000000002", Target: Target{Name: "测试采集", Kind: kind, URL: address, IntervalSeconds: 15, Enabled: true, SecureSettings: map[string]string{"password": "synthetic-probe-password"}}, Language: "zh", Alerts: true}
}

func TestConnectionPlansAllImplementedProtocolsAndObservedProfiles(t *testing.T) {
	for _, kind := range []string{"prometheus", "mysql", "redis", "postgresql", "clickhouse", "elasticsearch", "hadoop", "hdfs", "hive", "kafka", "spark", "flink"} {
		t.Run(kind, func(t *testing.T) {
			input := setupInput(kind)
			names := []string{"fixture_value"}
			switch kind {
			case "mysql":
				names = []string{"mysql_up", "mysql_global_status_uptime", "mysql_global_status_threads_connected", "mysql_global_variables_max_connections"}
			case "redis":
				names = []string{"redis_up", "redis_used_memory", "redis_maxmemory", "redis_total_commands_processed"}
			case "postgresql":
				names = []string{"pg_up", "pg_stat_database_numbackends", "pg_stat_database_xact_commit"}
			}
			samples := []Sample{}
			for _, name := range names {
				samples = append(samples, Sample{Name: name, Value: 1})
			}
			plan, err := BuildConnectionPlan(input, samples)
			require.NoError(t, err)
			require.Equal(t, kind, plan.Profile)
			require.NotEmpty(t, plan.Dashboard.Panels)
			require.NotEmpty(t, plan.Rules)
			raw, err := json.Marshal(plan)
			require.NoError(t, err)
			require.NotContains(t, string(raw), "synthetic-probe-password")
			require.True(t, plan.Target.SecureFields["password"])
			for _, panel := range plan.Dashboard.Panels {
				var config struct {
					Targets []struct {
						Expr string `json:"expr"`
					} `json:"targets"`
				}
				require.NoError(t, json.Unmarshal(panel.Config, &config))
				require.NotEmpty(t, config.Targets)
				for _, target := range config.Targets {
					ast, err := parser.NewParser(parser.Options{}).ParseExpr(target.Expr)
					require.NoError(t, err, target.Expr)
					parser.Inspect(ast, func(node parser.Node, _ []parser.Node) error {
						if v, ok := node.(*parser.VectorSelector); ok {
							found := false
							for _, m := range v.LabelMatchers {
								if m.Name == ConnectionLabel && m.Value == input.ID {
									found = true
								}
							}
							require.True(t, found, target.Expr)
						}
						return nil
					})
				}
			}
			for _, rule := range plan.Rules {
				require.GreaterOrEqual(t, rule.ForSeconds, 120)
				require.Equal(t, 3600, rule.NotificationCooldownSeconds)
				require.Equal(t, 60, rule.KeepFiringForSeconds)
				require.Equal(t, "KeepLast", rule.NoDataState)
				require.Contains(t, string(rule.Grafana), "timestamp(")
			}
		})
	}
}

func TestConnectionPlanNodeWindowsFreshnessAndNoUnobservedPanels(t *testing.T) {
	for _, profile := range []string{"node", "windows"} {
		input := setupInput("prometheus")
		metric := "node_cpu_seconds_total"
		total := "node_memory_MemTotal_bytes"
		free := "node_memory_MemAvailable_bytes"
		if profile == "windows" {
			metric = "windows_cpu_time_total"
			total = "windows_cs_physical_memory_bytes"
			free = "windows_os_physical_memory_free_bytes"
		}
		plan, err := BuildConnectionPlan(input, []Sample{{Name: metric, Value: 10}, {Name: total, Value: 100}, {Name: free, Value: 20}, {Name: "go_goroutines", Value: 5}})
		require.NoError(t, err)
		require.Equal(t, profile, plan.Profile)
		found := map[string]bool{}
		for _, p := range plan.Dashboard.Panels {
			found[p.ID] = true
			if p.ID == "memory" {
				require.Contains(t, p.Expr, "timestamp("+total)
				require.Contains(t, p.Expr, "timestamp("+free)
			}
		}
		require.True(t, found["cpu"])
		require.True(t, found["memory"])
		require.False(t, found["disk"])
		require.Len(t, plan.Dashboard.Panels, 3, "host health must not be buried under exporter runtime internals")
		positions := map[string]struct{ X, Y, W, H int }{}
		for _, panel := range plan.Dashboard.Panels {
			var config struct {
				Grid struct{ X, Y, W, H int } `json:"gridPos"`
			}
			require.NoError(t, json.Unmarshal(panel.Config, &config))
			positions[panel.ID] = config.Grid
		}
		require.Equal(t, 0, positions["cpu"].Y)
		require.Equal(t, 0, positions["memory"].Y)
		require.Equal(t, 12, positions["memory"].X)
		require.GreaterOrEqual(t, positions["cpu"].H, 12, "instrument scale must have readable space")
		require.GreaterOrEqual(t, positions["availability"].Y, positions["cpu"].H)
		require.Len(t, plan.Rules, 5)
	}
	input := setupInput("mysql")
	plan, err := BuildConnectionPlan(input, []Sample{{Name: "mysql_up", Value: 1}, {Name: "mysql_global_status_bytes_received", Value: 20}})
	require.NoError(t, err)
	for _, p := range plan.Dashboard.Panels {
		require.NotEqual(t, "mysql-12", p.ID, "a two-query IO panel cannot be provisioned with only one metric")
	}
}

func TestConnectionPlanStableAcrossCardinalityNotAcrossConfiguration(t *testing.T) {
	input := setupInput("prometheus")
	a, err := BuildConnectionPlan(input, []Sample{{Name: "events_total", Value: 5, Labels: map[string]string{"worker": "1"}}})
	require.NoError(t, err)
	b, err := BuildConnectionPlan(input, []Sample{{Name: "events_total", Value: 8}, {Name: "events_total", Value: 4, Labels: map[string]string{"worker": "2"}}})
	require.NoError(t, err)
	require.Equal(t, a.PlanHash, b.PlanHash)
	input.Target.IntervalSeconds = 30
	c, err := BuildConnectionPlan(input, []Sample{{Name: "events_total", Value: 8}})
	require.NoError(t, err)
	require.NotEqual(t, a.PlanHash, c.PlanHash)
	input.Target.Labels = map[string]string{ConnectionLabel: "attacker"}
	require.Error(t, input.Validate())
	input = setupInput("prometheus")
	input.Target.Labels = map[string]string{"job": "merge-other-source"}
	require.Error(t, input.Validate())
	input = setupInput("prometheus")
	_, err = BuildConnectionPlan(input, nil)
	require.Error(t, err)
	samples := make([]Sample, 1000)
	for i := range samples {
		samples[i] = Sample{Name: fmt.Sprintf("observed_%04d", i), Value: 1}
	}
	d, err := BuildConnectionPlan(input, samples)
	require.NoError(t, err)
	require.Len(t, d.Metrics, 256)
	require.Equal(t, 1000, d.MetricCount)
	require.LessOrEqual(t, len(d.Dashboard.Panels), 24)
	require.Equal(t, "synthetic-probe-password", input.Target.SecureSettings["password"])
	for _, p := range d.Dashboard.Panels {
		require.False(t, strings.Contains(p.Expr, "attacker"))
	}
}
