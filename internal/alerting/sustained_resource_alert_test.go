package alerting

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Persisted evaluation and the real outbox/HTTP worker prove that a short spike
// never becomes an external notification, including across process restarts.
func TestSustainedResourceAlertDropsSpikesAndDebouncesRecoveryAcrossRestart(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(strconv.FormatBool(grouped), func(t *testing.T) {
			f := newCooldownFixture(t, grouped)
			rule, err := f.e.Store.AlertRule(t.Context(), "service")
			require.NoError(t, err)
			rule.ForSeconds, rule.KeepFiringForSeconds = 600, 300
			_, err = f.e.SaveRule(t.Context(), rule.AlertRule)
			require.NoError(t, err)
			step := func(seconds int, value float64, state string, send bool) {
				at := f.at.Add(time.Duration(seconds) * time.Second)
				evaluateGroup(t, f.e, at, map[string]float64{"host": value})
				current, err := f.e.Store.AlertRule(t.Context(), "service")
				require.NoError(t, err)
				require.Equal(t, state, current.Runtime.Instances[0].State)
				processGroup(t, f.e, at, send)
			}
			step(0, 1, "Pending", false)
			step(30, 0, "Normal", false)
			require.Empty(t, f.received)
			step(100, 1, "Pending", false)
			step(300, 1, "Pending", false)
			f.restart(t)
			step(699, 1, "Pending", false)
			step(700, 1, "Firing", true)
			require.Equal(t, "firing", (<-f.received).Status)
			step(710, 0, "Recovering", false)
			step(800, 1, "Firing", false)
			step(810, 0, "Recovering", false)
			f.restart(t)
			step(1109, 0, "Recovering", false)
			step(1110, 0, "Normal", true)
			require.Equal(t, "resolved", (<-f.received).Status)
			step(1200, 1, "Pending", false)
			step(1800, 1, "Firing", false)
			require.Empty(t, f.received)
		})
	}
}
