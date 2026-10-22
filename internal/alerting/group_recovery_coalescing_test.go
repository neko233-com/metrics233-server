package alerting

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCooldownGroupCoalescesStaggeredResourceRecovery(t *testing.T) {
	f := newCooldownFixture(t, true)
	policy, err := f.e.Store.NotificationPolicy(t.Context())
	require.NoError(t, err)
	policy.GroupWait = "30s"
	policy.GroupInterval = "1m"
	_, err = f.e.Store.SaveNotificationPolicy(t.Context(), policy)
	require.NoError(t, err)
	evaluateGroup(t, f.e, f.at, map[string]float64{"cpu": 1, "memory": 1, "disk": 1})
	processGroup(t, f.e, f.at, false)
	processGroup(t, f.e, f.at.Add(30*time.Second), true)
	first := <-f.received
	require.Equal(t, "firing", first.Status)
	require.Len(t, first.Alerts, 3)
	// Independent resource evaluations can resolve a few seconds apart. Sending
	// the first result immediately creates two recovery cards one interval apart.
	evaluateGroup(t, f.e, f.at.Add(120*time.Second), map[string]float64{"cpu": 0, "memory": 1, "disk": 1})
	processGroup(t, f.e, f.at.Add(120*time.Second), false)
	evaluateGroup(t, f.e, f.at.Add(122*time.Second), map[string]float64{"cpu": 0, "memory": 0, "disk": 0})
	processGroup(t, f.e, f.at.Add(149*time.Second), false)
	processGroup(t, f.e, f.at.Add(153*time.Second), true)
	recovered := <-f.received
	require.Equal(t, "resolved", recovered.Status)
	require.Len(t, recovered.Alerts, 3)
	// A new period inside CD has no successful firing ACK. Its staggered recovery
	// must not create an orphan card, before or after the former group's interval.
	evaluateGroup(t, f.e, f.at.Add(300*time.Second), map[string]float64{"cpu": 1, "memory": 1, "disk": 1})
	processGroup(t, f.e, f.at.Add(331*time.Second), false)
	evaluateGroup(t, f.e, f.at.Add(420*time.Second), map[string]float64{"cpu": 0, "memory": 1, "disk": 1})
	evaluateGroup(t, f.e, f.at.Add(422*time.Second), map[string]float64{"cpu": 0, "memory": 0, "disk": 0})
	processGroup(t, f.e, f.at.Add(453*time.Second), false)
	processGroup(t, f.e, f.at.Add(520*time.Second), false)
	require.Empty(t, f.received)
}
