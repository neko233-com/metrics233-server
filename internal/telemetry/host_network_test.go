package telemetry

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHostNetworkSnapshotKeepsUnknownDistinctFromActualZero(t *testing.T) {
	samples := []Sample{{Name: "host_network_receive_bytes_total", Value: 12, Labels: map[string]string{"device": "eth0"}}, {Name: "host_network_transmit_bytes_total", Value: 34, Labels: map[string]string{"device": "eth0"}}, {Name: "host_network_receive_bytes_per_second", Value: 0, Labels: map[string]string{"device": "eth0"}}}
	for _, sample := range samples {
		require.NoError(t, ValidateHostObservation(sample))
	}
	snapshot := HostResourceSnapshot(samples, 1234)
	require.Len(t, snapshot.Network, 1)
	n := snapshot.Network[0]
	require.Equal(t, 12.0, n.ReceiveBytes)
	require.NotNil(t, n.ReceivePerSecond)
	require.Equal(t, 0.0, *n.ReceivePerSecond)
	require.Nil(t, n.TransmitPerSecond)
	failed := HostResourceSnapshot([]Sample{{Name: "host_resource_collection_success", Labels: map[string]string{"resource": "network"}, Value: 0}}, 1235)
	require.Empty(t, failed.Network)
	for _, sample := range []Sample{{Name: "host_network_receive_bytes_total", Value: 1}, {Name: "host_network_receive_bytes_total", Value: -1, Labels: map[string]string{"device": "eth0"}}, {Name: "host_network_invented", Value: 0, Labels: map[string]string{"device": "eth0"}}} {
		require.Error(t, ValidateHostObservation(sample))
	}
}
