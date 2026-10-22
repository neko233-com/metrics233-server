package model

import (
	"encoding/json"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

func TestMySQLDashboardHasFixedHostScopeAndValidRateSemantics(t *testing.T) {
	h := ManagedHost{ID: "123e4567-e89b-12d3-a456-426614174000", Name: "Fixture MySQL"}
	d := MySQLDashboard(h)
	require.LessOrEqual(t, len(d.ID), 40)
	require.NoError(t, d.Validate())
	require.Len(t, d.Panels, 18)
	for _, p := range d.Panels {
		var config struct {
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
			FieldConfig struct {
				Defaults map[string]any `json:"defaults"`
			} `json:"fieldConfig"`
		}
		require.NoError(t, json.Unmarshal(p.Config, &config))
		require.NotEmpty(t, config.Targets)
		for _, target := range config.Targets {
			_, err := parser.NewParser(parser.Options{}).ParseExpr(target.Expr)
			require.NoError(t, err, p.Title)
			require.Contains(t, target.Expr, `managed_host="`+h.ID+`"`)
		}
		if p.Title == "MySQL uptime" {
			require.Equal(t, "stat", p.Visualization)
			require.Equal(t, "uptime", p.Unit)
		}
		if p.Title == "MySQL buffer pool usage" {
			thresholds := config.FieldConfig.Defaults["thresholds"].(map[string]any)
			require.Len(t, thresholds["steps"], 1, "a full cache is not a resource incident")
		}
	}
}

func TestMySQLDashboardChineseSavedContentIsExplicit(t *testing.T) {
	d := MySQLDashboard(ManagedHost{ID: "123e4567-e89b-12d3-a456-426614174000", Name: "Fixture"}, "zh")
	require.NoError(t, d.Validate())
	require.Contains(t, string(d.Panels[0].Config), "正常")
	require.Contains(t, string(d.Panels[5].Config), "已连接")
	require.NotContains(t, string(d.Panels[5].Config), `"legendFormat":"Connected"`)
}
