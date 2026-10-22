package hostagent

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/require"
)

func TestCPUDeltaRejectsResetAndEmptyInterval(t *testing.T) {
	before := cpu.TimesStat{User: 10, Idle: 80, Iowait: 10, Guest: 5}
	value, ok := cpuUsage(before, cpu.TimesStat{User: 30, Idle: 150, Iowait: 20, Guest: 15})
	require.True(t, ok)
	require.InDelta(t, 20, value, .000001)
	for _, after := range []cpu.TimesStat{before, {User: 9, Idle: 180, Iowait: 20}, {User: math.NaN(), Idle: 90}} {
		_, ok = cpuUsage(before, after)
		require.False(t, ok)
	}
}

func TestPersistentDisksExcludeReadonlyRemoteAndAPFSHelpers(t *testing.T) {
	parts := []disk.PartitionStat{
		{Device: "root", Mountpoint: "/", Fstype: "apfs", Opts: []string{"ro"}},
		{Device: "data", Mountpoint: "/System/Volumes/Data", Fstype: "apfs", Opts: []string{"rw"}},
		{Device: "helper", Mountpoint: "/System/Volumes/VM", Fstype: "apfs"},
		{Device: "remote", Mountpoint: "/mnt/nfs", Fstype: "nfs"},
		{Device: "data", Mountpoint: "/another", Fstype: "apfs"},
	}
	selected := persistentPartitions(parts)
	require.Len(t, selected, 1)
	require.Equal(t, "/System/Volumes/Data", selected[0].Mountpoint)
}

func TestSamplerNeverFakesZeroAndReportsIndependentFailures(t *testing.T) {
	broken := false
	observations := 0
	s := &resourceSampler{reader: resourceReader{
		cpu: func(context.Context, bool) ([]cpu.TimesStat, error) {
			observations++
			return []cpu.TimesStat{{User: float64(observations * 2), Idle: float64(observations * 8)}}, nil
		},
		memory: func(context.Context) (*mem.VirtualMemoryStat, error) {
			if broken {
				return nil, errors.New("not available")
			}
			return &mem.VirtualMemoryStat{Total: 1000, Available: 400}, nil
		},
		partitions: func(context.Context, bool) ([]disk.PartitionStat, error) {
			return []disk.PartitionStat{{Device: "disk", Mountpoint: "/data", Fstype: "ext4"}}, nil
		},
		usage: func(context.Context, string) (*disk.UsageStat, error) {
			return &disk.UsageStat{Total: 1000, Free: 250, Used: 100}, nil
		},
		uptime: func(context.Context) (uint64, error) { return 123, nil },
	}}
	samples, healthy := s.collect(context.Background())
	require.True(t, healthy)
	for _, sample := range samples {
		require.NotEqual(t, "host_cpu_usage_percent", sample.Name)
	}
	samples, healthy = s.collect(context.Background())
	require.True(t, healthy)
	values := map[string]float64{}
	for _, sample := range samples {
		values[sample.Name] = sample.Value
	}
	require.Equal(t, 20.0, values["host_cpu_usage_percent"])
	require.Equal(t, 60.0, values["host_memory_used_percent"])
	require.Equal(t, 75.0, values["host_disk_used_percent"], "capacity uses total minus available, including shared pool usage")
	broken = true
	samples, healthy = s.collect(context.Background())
	require.False(t, healthy)
	for _, sample := range samples {
		require.NotEqual(t, "host_memory_used_percent", sample.Name, "missing memory must not become zero or an old value")
	}
}
