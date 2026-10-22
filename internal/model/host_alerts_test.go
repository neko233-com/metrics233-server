package model

import (
	"testing"

	"github.com/neko233-com/metrics233-server/internal/alertgraph"
	"github.com/stretchr/testify/require"
)

func TestHostAlertDefaultsSeparateMeasurementAndHysteresis(t *testing.T) {
	rules := HostAlertDefaults(ManagedHost{ID: "123e4567-e89b-12d3-a456-426614174000", Name: "Fixture host", Target: Target{IntervalSeconds: 30}}, "operations")
	require.Len(t, rules, 6)
	for _, rule := range rules {
		require.NoError(t, rule.Validate())
		plan, err := alertgraph.Parse(rule.Grafana)
		require.NoError(t, err)
		require.Equal(t, "B", plan.RecoveryRef)
		require.Equal(t, 3600, rule.NotificationCooldownSeconds)
		require.Equal(t, 300, rule.KeepFiringForSeconds)
		require.GreaterOrEqual(t, rule.ForSeconds, 300)
		require.Equal(t, "A", rule.Annotations["value_ref"])
		require.NotContains(t, string(rule.Grafana), "bool")
		require.Contains(t, string(rule.Grafana), "timestamp(")
	}
}
