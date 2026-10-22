package model

import (
	"encoding/json"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

func TestHostDashboardIsScopedNativeAndHasNoFakeZeroFallback(t *testing.T) {
	h := ManagedHost{ID: "123e4567-e89b-12d3-a456-426614174000", Name: "Test host"}
	for _, language := range []string{"en", "zh"} {
		d := HostDashboard(h, language)
		require.NoError(t, d.Validate())
		require.LessOrEqual(t, len(d.ID), 40)
		require.Len(t, d.Panels, 8)
		gauges := 0
		for _, p := range d.Panels {
			require.Contains(t, p.Expr, `managed_host="`+h.ID+`"`)
			require.NotContains(t, p.Expr, "vector(0)")
			_, err := parser.NewParser(parser.Options{}).ParseExpr(p.Expr)
			require.NoError(t, err)
			if p.Visualization == "gauge" {
				gauges++
				var cfg map[string]any
				require.NoError(t, json.Unmarshal(p.Config, &cfg))
				require.EqualValues(t, 100, cfg["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["max"])
			}
		}
		require.Equal(t, 3, gauges)
	}
}
