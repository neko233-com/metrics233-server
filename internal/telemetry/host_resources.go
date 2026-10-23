package telemetry

import (
	"errors"
	"math"
	"sort"
	"strings"
)

// HostResources is a bounded, server-derived latest observation, not a second
// time-series store. Missing observations stay nil rather than looking healthy.
type HostResources struct {
	ObservedAt      int64         `json:"observed_at"`
	CPUPercent      *float64      `json:"cpu_percent"`
	Cores           float64       `json:"cores"`
	MemoryPercent   *float64      `json:"memory_percent"`
	MemoryTotal     float64       `json:"memory_total"`
	MemoryAvailable float64       `json:"memory_available"`
	Uptime          float64       `json:"uptime"`
	Disks           []HostDisk    `json:"disks"`
	Network         []HostNetwork `json:"network,omitempty"`
}

type HostNetwork struct {
	Device            string   `json:"device"`
	ReceiveBytes      float64  `json:"receive_bytes"`
	TransmitBytes     float64  `json:"transmit_bytes"`
	ReceivePerSecond  *float64 `json:"receive_per_second"`
	TransmitPerSecond *float64 `json:"transmit_per_second"`
}

type HostDisk struct {
	Device      string  `json:"device"`
	Mountpoint  string  `json:"mountpoint"`
	FSType      string  `json:"fstype"`
	Total       float64 `json:"total"`
	Available   float64 `json:"available"`
	UsedPercent float64 `json:"used_percent"`
}

func (h ManagedHost) HostCollectionEnabled() bool { return h.CollectHost == nil || *h.CollectHost }

// Host observations have a reserved finite namespace and fixed units. An agent
// cannot create arbitrary resource fields or claim an impossible utilization.
func ValidateHostObservation(s Sample) error {
	if !strings.HasPrefix(s.Name, "host_") {
		return nil
	}
	if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) || s.Value < 0 {
		return errors.New("invalid host observation")
	}
	switch s.Name {
	case "host_cpu_usage_percent", "host_memory_used_percent", "host_disk_used_percent":
		if s.Value > 100 {
			return errors.New("host utilization exceeds 100 percent")
		}
	case "host_cpu_logical_cores", "host_memory_total_bytes", "host_memory_available_bytes", "host_memory_used_bytes", "host_disk_total_bytes", "host_disk_available_bytes", "host_disk_used_bytes", "host_uptime_seconds":
	case "host_network_receive_bytes_total", "host_network_transmit_bytes_total", "host_network_receive_bytes_per_second", "host_network_transmit_bytes_per_second", "host_network_receive_packets_total", "host_network_transmit_packets_total", "host_network_receive_errors_total", "host_network_transmit_errors_total", "host_network_receive_dropped_total", "host_network_transmit_dropped_total":
		device := s.Labels["device"]
		if device == "" || len(device) > 128 || strings.ContainsAny(device, "\x00\r\n") {
			return errors.New("invalid network interface")
		}
	case "host_resource_collection_success":
		if s.Value != 0 && s.Value != 1 {
			return errors.New("invalid resource availability")
		}
		switch s.Labels["resource"] {
		case "cpu", "memory", "disk", "uptime", "network":
		default:
			return errors.New("unknown host resource")
		}
	default:
		return errors.New("unknown host observation")
	}
	if strings.HasPrefix(s.Name, "host_disk_") && (s.Labels["mountpoint"] == "" || len(s.Labels["mountpoint"]) > 512 || s.Labels["device"] == "" || len(s.Labels["device"]) > 256) {
		return errors.New("invalid disk identity")
	}
	return nil
}

// HostResourceSnapshot consumes only validated, namespace-scoped observations.
// A failed resource is absent in the new snapshot; an older value is not copied.
func HostResourceSnapshot(samples []Sample, at int64) *HostResources {
	r := &HostResources{ObservedAt: at, Disks: []HostDisk{}}
	found := false
	disks := map[string]*HostDisk{}
	network := map[string]*HostNetwork{}
	for _, s := range samples {
		if len(s.Name) < 5 || s.Name[:5] != "host_" {
			continue
		}
		found = true
		if s.Timestamp > 0 && s.Timestamp < r.ObservedAt {
			r.ObservedAt = s.Timestamp
		}
		v := s.Value
		switch s.Name {
		case "host_cpu_usage_percent":
			if v >= 0 && v <= 100 {
				r.CPUPercent = &v
			}
		case "host_cpu_logical_cores":
			r.Cores = v
		case "host_memory_used_percent":
			if v >= 0 && v <= 100 {
				r.MemoryPercent = &v
			}
		case "host_memory_total_bytes":
			r.MemoryTotal = v
		case "host_memory_available_bytes":
			r.MemoryAvailable = v
		case "host_uptime_seconds":
			r.Uptime = v
		case "host_network_receive_bytes_total", "host_network_transmit_bytes_total", "host_network_receive_bytes_per_second", "host_network_transmit_bytes_per_second":
			device := s.Labels["device"]
			n := network[device]
			if n == nil && device != "" && len(network) < 64 {
				n = &HostNetwork{Device: device}
				network[device] = n
			}
			if n == nil {
				continue
			}
			switch s.Name {
			case "host_network_receive_bytes_total":
				n.ReceiveBytes = v
			case "host_network_transmit_bytes_total":
				n.TransmitBytes = v
			case "host_network_receive_bytes_per_second":
				n.ReceivePerSecond = &v
			case "host_network_transmit_bytes_per_second":
				n.TransmitPerSecond = &v
			}
		case "host_disk_total_bytes", "host_disk_available_bytes", "host_disk_used_percent":
			key := s.Labels["mountpoint"] + "\x00" + s.Labels["device"]
			d := disks[key]
			if d == nil && len(disks) < 32 {
				d = &HostDisk{Device: s.Labels["device"], Mountpoint: s.Labels["mountpoint"], FSType: s.Labels["fstype"]}
				disks[key] = d
			}
			if d == nil {
				continue
			}
			switch s.Name {
			case "host_disk_total_bytes":
				d.Total = v
			case "host_disk_available_bytes":
				d.Available = v
			case "host_disk_used_percent":
				d.UsedPercent = v
			}
		}
	}
	for _, d := range disks {
		if d.Total > 0 && d.Available >= 0 && d.Available <= d.Total {
			r.Disks = append(r.Disks, *d)
		}
	}
	sort.Slice(r.Disks, func(i, j int) bool { return r.Disks[i].Mountpoint < r.Disks[j].Mountpoint })
	for _, n := range network {
		r.Network = append(r.Network, *n)
	}
	sort.Slice(r.Network, func(i, j int) bool { return r.Network[i].Device < r.Network[j].Device })
	if !found {
		return nil
	}
	return r
}
