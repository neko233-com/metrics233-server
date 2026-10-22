package hostagent

import (
	"context"
	"math"
	"runtime"
	"slices"
	"sort"
	"strings"

	model "github.com/neko233-com/metrics233-server/internal/telemetry"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

type resourceReader struct {
	cpu        func(context.Context, bool) ([]cpu.TimesStat, error)
	memory     func(context.Context) (*mem.VirtualMemoryStat, error)
	partitions func(context.Context, bool) ([]disk.PartitionStat, error)
	usage      func(context.Context, string) (*disk.UsageStat, error)
	uptime     func(context.Context) (uint64, error)
}

type resourceSampler struct {
	reader   resourceReader
	previous *cpu.TimesStat
}

func newResourceSampler() *resourceSampler {
	return &resourceSampler{reader: resourceReader{cpu.TimesWithContext, mem.VirtualMemoryWithContext, localPartitions, disk.UsageWithContext, host.UptimeWithContext}}
}

// CPU counters include guest time in user/nice on Linux. I/O wait is idle.
func cpuUsage(before, after cpu.TimesStat) (float64, bool) {
	values := func(t cpu.TimesStat) []float64 {
		return []float64{t.User, t.System, t.Nice, t.Idle, t.Iowait, t.Irq, t.Softirq, t.Steal}
	}
	a, b := values(before), values(after)
	total, idle := 0.0, 0.0
	for i, v := range b {
		d := v - a[i]
		if math.IsNaN(d) || math.IsInf(d, 0) || d < 0 {
			return 0, false
		}
		total += d
		if i == 3 || i == 4 {
			idle += d
		}
	}
	if total <= 0 {
		return 0, false
	}
	return max(0, min(100, 100*(total-idle)/total)), true
}

func persistentPartitions(input []disk.PartitionStat) []disk.PartitionStat {
	allowed := map[string]bool{"apfs": true, "hfs": true, "hfs+": true, "ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "zfs": true, "ufs": true, "ntfs": true, "refs": true, "exfat": true, "vfat": true, "overlay": true}
	sort.Slice(input, func(i, j int) bool { return input[i].Mountpoint < input[j].Mountpoint })
	out := []disk.PartitionStat{}
	seen := map[string]bool{}
	for _, p := range input {
		if !allowed[strings.ToLower(p.Fstype)] || slices.Contains(p.Opts, "ro") || len(p.Mountpoint) > 512 || len(p.Device) > 256 || p.Mountpoint == "" {
			continue
		}
		// APFS helper volumes share a container and are not independent capacity.
		if strings.HasPrefix(p.Mountpoint, "/System/Volumes/") && p.Mountpoint != "/System/Volumes/Data" {
			continue
		}
		if seen[p.Device] {
			continue
		}
		seen[p.Device] = true
		out = append(out, p)
		if len(out) == 32 {
			break
		}
	}
	return out
}

func (s *resourceSampler) collect(ctx context.Context) ([]model.Sample, bool) {
	out := []model.Sample{}
	healthy := true
	add := func(name string, v float64, labels map[string]string) {
		out = append(out, model.Sample{Name: name, Value: v, Labels: labels})
	}
	status := func(resource string, ok bool) {
		v := 0.0
		if ok {
			v = 1
		} else {
			healthy = false
		}
		add("host_resource_collection_success", v, map[string]string{"resource": resource})
	}
	times, err := s.reader.cpu(ctx, false)
	if err == nil && len(times) == 1 {
		valid := s.previous == nil // first observation is a baseline, not 0% usage
		if s.previous != nil {
			if value, ok := cpuUsage(*s.previous, times[0]); ok {
				add("host_cpu_usage_percent", value, nil)
				valid = true
			}
		}
		s.previous = &times[0]
		add("host_cpu_logical_cores", float64(runtime.NumCPU()), nil)
		status("cpu", valid)
	} else {
		s.previous = nil
		status("cpu", false)
	}
	m, err := s.reader.memory(ctx)
	if err == nil && m != nil && m.Total > 0 && m.Available <= m.Total {
		add("host_memory_total_bytes", float64(m.Total), nil)
		add("host_memory_available_bytes", float64(m.Available), nil)
		add("host_memory_used_bytes", float64(m.Total-m.Available), nil)
		add("host_memory_used_percent", 100*float64(m.Total-m.Available)/float64(m.Total), nil)
		status("memory", true)
	} else {
		status("memory", false)
	}
	parts, err := s.reader.partitions(ctx, false)
	disksOK := err == nil
	count := 0
	if err == nil {
		for _, p := range persistentPartitions(parts) {
			if ctx.Err() != nil {
				disksOK = false
				break
			}
			u, e := s.reader.usage(ctx, p.Mountpoint)
			if e != nil || u == nil || u.Total == 0 || u.Free > u.Total {
				disksOK = false
				continue
			}
			labels := map[string]string{"mountpoint": p.Mountpoint, "device": p.Device, "fstype": p.Fstype}
			// Total-Free measures space unavailable to this writable volume, including
			// other APFS container consumers. It must not be summed across volumes.
			add("host_disk_total_bytes", float64(u.Total), labels)
			add("host_disk_available_bytes", float64(u.Free), labels)
			add("host_disk_used_bytes", float64(u.Total-u.Free), labels)
			add("host_disk_used_percent", 100*float64(u.Total-u.Free)/float64(u.Total), labels)
			count++
		}
	}
	status("disk", disksOK && count > 0)
	if up, e := s.reader.uptime(ctx); e == nil {
		add("host_uptime_seconds", float64(up), nil)
		status("uptime", true)
	} else {
		status("uptime", false)
	}
	return out, healthy
}
