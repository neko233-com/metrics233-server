package telemetry

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHostSnapshotRetainsMissingAndObservationTime(t *testing.T) {
	r := HostResourceSnapshot([]Sample{{Name: "host_cpu_usage_percent", Value: 0, Timestamp: 123}, {Name: "host_memory_total_bytes", Value: 100}, {Name: "host_resource_collection_success", Value: 0, Labels: map[string]string{"resource": "memory"}}}, 1000)
	require.EqualValues(t, 123, r.ObservedAt)
	require.NotNil(t, r.CPUPercent)
	require.Zero(t, *r.CPUPercent)
	require.Nil(t, r.MemoryPercent)
	require.Nil(t, HostResourceSnapshot([]Sample{{Name: "mysql_up", Value: 1}}, 1000))
	for _, sample := range []Sample{{Name: "host_cpu_usage_percent", Value: 101}, {Name: "host_disk_used_percent", Value: -1}, {Name: "host_unregistered", Value: 1}, {Name: "host_resource_collection_success", Value: 2}} {
		require.Error(t, ValidateHostObservation(sample))
	}
}
