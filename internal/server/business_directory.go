package server

import (
	"context"
	"errors"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/prometheus/prometheus/promql"
)

type businessCardSummary struct {
	HostName        string   `json:"host_name,omitempty"`
	CPU             *float64 `json:"cpu_percent"`
	Memory          *float64 `json:"memory_percent"`
	Disk            *float64 `json:"disk_percent"`
	MemoryTotal     float64  `json:"memory_total,omitempty"`
	MemoryAvailable float64  `json:"memory_available,omitempty"`
	HostObservedAt  int64    `json:"host_observed_at,omitempty"`
	HostExpiresAt   int64    `json:"host_expires_at,omitempty"`
	Receive         *float64 `json:"receive_per_second"`
	Transmit        *float64 `json:"transmit_per_second"`
	TrafficScope    string   `json:"traffic_scope"`
}

func (s *Server) businessDirectoryRoutes(api routeRegistrar) {
	api.HandleFunc("GET /api/v1/business/directory", func(w http.ResponseWriter, r *http.Request) {
		if s.Authorizer != nil {
			p, ok := identity.FromContext(r.Context())
			if !ok || !p.Role.AtLeast(identity.RoleViewer) {
				writeResourceReadError(w, identity.ErrForbidden)
				return
			}
		}
		q := r.URL.Query()
		limit := 50
		var err error
		if q.Get("limit") != "" {
			limit, err = strconv.Atoi(q.Get("limit"))
		}
		if err != nil {
			fail(w, 400, errors.New("invalid limit"))
			return
		}
		page, err := s.Store.BusinessDirectory(r.Context(), store.BusinessDirectoryFilter{Query: q.Get("q"), Environment: q.Get("environment"), Role: q.Get("role"), Sort: q.Get("sort"), Cursor: q.Get("cursor"), Limit: limit})
		if err != nil {
			fail(w, 400, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		summaries := s.businessCardSummaries(ctx, page.Items, time.Now())
		writeJSON(w, 200, struct {
			store.BusinessDirectoryPage
			Summaries map[string]*businessCardSummary `json:"summaries"`
		}{page, summaries})
	})
	api.HandleFunc("PUT /api/v1/business/instances/{id}/note", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Version     *int64  `json:"version"`
			Description *string `json:"description"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		if input.Version == nil || input.Description == nil {
			fail(w, 400, errors.New("version and description are required"))
			return
		}
		note, err := s.Store.SaveBusinessNote(r.Context(), r.PathValue("id"), model.BusinessNote{Version: *input.Version, Description: *input.Description})
		if err != nil {
			businessError(w, err)
			return
		}
		writeJSON(w, 200, note)
	})
	api.HandleFunc("GET /api/v1/business/instances/{id}/network", func(w http.ResponseWriter, r *http.Request) {
		item, err := s.Store.BusinessInstance(r.Context(), r.PathValue("id"))
		if err != nil {
			businessError(w, err)
			return
		}
		language := r.URL.Query().Get("language")
		d := businessNetworkDashboard(item, language)
		out := struct {
			Business  model.Dashboard  `json:"business"`
			Host      *model.Dashboard `json:"host,omitempty"`
			Devices   []string         `json:"devices"`
			Device    string           `json:"device"`
			HostName  string           `json:"host_name"`
			HostStale bool             `json:"host_stale"`
		}{Business: d, Devices: []string{}}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		b, e := s.Store.BusinessHostBinding(ctx, item.ID)
		if e != nil {
			writeJSON(w, 200, out)
			return
		}
		var target *model.Target
		if b.Kind == "managed_host" {
			if h, _, e := s.Store.ManagedHost(ctx, b.ID); e == nil && h.HostCollectionEnabled() {
				out.HostName = h.Name
				out.HostStale = !h.Enabled || h.Revoked || h.Resources == nil || time.Now().UnixMilli()-h.Resources.ObservedAt > int64(max(90, h.Target.IntervalSeconds*3))*1000
				if h.Resources != nil {
					for _, n := range h.Resources.Network {
						out.Devices = append(out.Devices, n.Device)
					}
				}
			}
		}
		if b.Kind == "collector" {
			targets, _ := s.Store.Targets(ctx)
			for _, t := range targets {
				if strconv.FormatInt(t.ID, 10) == b.ID {
					target = &t
					out.HostName = t.Name
					out.HostStale = !t.Enabled || time.Now().UnixMilli()-t.LastScrape > int64(max(90, t.IntervalSeconds*3))*1000
					break
				}
			}
			if target != nil {
				job, instance := target.Name, target.URL
				if v, ok := target.Labels["job"]; ok {
					job = v
				}
				if v, ok := target.Labels["instance"]; ok {
					instance = v
				}
				selector := `{job=` + strconv.Quote(job) + `,instance=` + strconv.Quote(instance) + `}`
				s.cardVector(ctx, "node_network_receive_bytes_total"+selector, time.Now(), func(labels map[string]string, _ float64) {
					v := labels["device"]
					if v != "" && v != "lo" && len(v) <= 128 && len(out.Devices) < 64 {
						out.Devices = append(out.Devices, v)
					}
				})
			}
		}
		sort.Strings(out.Devices)
		requested := r.URL.Query().Get("device")
		for _, device := range out.Devices {
			if device == requested {
				out.Device = device
				break
			}
		}
		if out.Device == "" {
			out.Device = defaultNetworkDevice(out.Devices)
		}
		if out.Device != "" {
			host := hostNetworkDashboard(b.Kind, b.ID, out.Device, language, target)
			if out.HostStale {
				host.Panels = host.Panels[2:]
			}
			out.Host = &host
		}
		writeJSON(w, 200, out)
	})
}

// Copy numeric values while the PromQL query owns its result buffers.
func (s *Server) cardVector(ctx context.Context, expr string, at time.Time, consume func(map[string]string, float64)) {
	if ctx.Err() != nil || s.Prometheus == nil {
		return
	}
	q, err := s.Prometheus.Engine.NewInstantQuery(ctx, s.Prometheus.Queryable, promql.NewPrometheusQueryOpts(false, 0), expr, at)
	if err != nil {
		return
	}
	defer q.Close()
	result := q.Exec(ctx)
	if result.Err != nil {
		return
	}
	vector, ok := result.Value.(promql.Vector)
	if !ok || len(vector) > 10000 {
		return
	}
	for _, sample := range vector {
		if sample.H != nil || math.IsNaN(sample.F) || math.IsInf(sample.F, 0) || sample.F < 0 {
			continue
		}
		labels := map[string]string{}
		for _, key := range []string{"service", "role", "serverId", "environment", "channel", "instance", "implementation", "job", "device"} {
			labels[key] = sample.Metric.Get(key)
		}
		consume(labels, sample.F)
	}
}

const businessGroupLabels = "service,role,serverId,environment,channel,instance,implementation"

func businessTrafficMetric(role, direction string) []string {
	if role == "sdk" {
		if direction == "receive" {
			return []string{"http_request_body_bytes_total"}
		}
		return []string{"http_response_body_bytes_total"}
	}
	legacy := "game_bytes_"
	if role == "gateway" {
		legacy = "gateway_bytes_"
	}
	suffix := "received_bytes_total"
	old := "received_total"
	if direction == "transmit" {
		suffix = "sent_bytes_total"
		old = "sent_total"
	}
	return []string{"service_payload_" + suffix, legacy + old}
}

// Unified and legacy counters are alternatives, not additive aliases.
func businessTrafficExpr(role, direction, selector, window, operation string) string {
	parts := []string{}
	for _, name := range businessTrafficMetric(role, direction) {
		m := name + selector
		fresh := "(timestamp(" + m + ") >= time() - 90)"
		unique := "(count by (" + businessGroupLabels + ") (" + m + " and " + fresh + ") == 1)"
		parts = append(parts, "(sum by ("+businessGroupLabels+") ("+operation+"("+m+"["+window+"]) and "+fresh+") and on ("+businessGroupLabels+") "+unique+")")
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

func (s *Server) businessCardSummaries(ctx context.Context, items []model.BusinessInstance, at time.Time) map[string]*businessCardSummary {
	out := map[string]*businessCardSummary{}
	if len(items) == 0 {
		return out
	}
	for _, item := range items {
		scope := "payload"
		if item.Identity.Role == "sdk" {
			scope = "http"
		}
		out[item.ID] = &businessCardSummary{TrafficScope: scope}
	}
	bindings, err := s.Store.BusinessHostBindings(ctx)
	if err != nil {
		return out
	}
	hosts, _ := s.Store.ManagedHosts(ctx)
	hostMap := map[string]model.ManagedHost{}
	for _, host := range hosts {
		hostMap[host.ID] = host
	}
	targets, _ := s.Store.Targets(ctx)
	targetMap := map[string]model.Target{}
	for _, t := range targets {
		targetMap[strconv.FormatInt(t.ID, 10)] = t
	}
	collectorCards := map[string][]*businessCardSummary{}
	instances := []string{}
	for _, item := range items {
		card := out[item.ID]
		b := bindings[item.ID]
		if b.Kind == "managed_host" {
			h, ok := hostMap[b.ID]
			if !ok || !h.Enabled || h.Revoked || !h.HostCollectionEnabled() || h.Resources == nil {
				continue
			}
			res := h.Resources
			if res.ObservedAt > at.UnixMilli()+30000 || at.UnixMilli()-res.ObservedAt > int64(max(90, h.Target.IntervalSeconds*3))*1000 {
				continue
			}
			card.HostName = h.Name
			card.CPU = res.CPUPercent
			card.Memory = res.MemoryPercent
			card.MemoryTotal = res.MemoryTotal
			card.MemoryAvailable = res.MemoryAvailable
			card.HostObservedAt = res.ObservedAt
			card.HostExpiresAt = res.ObservedAt + int64(max(90, h.Target.IntervalSeconds*3))*1000
			for _, disk := range res.Disks {
				if card.Disk == nil || disk.UsedPercent > *card.Disk {
					v := disk.UsedPercent
					card.Disk = &v
				}
			}
		} else if b.Kind == "collector" {
			t, ok := targetMap[b.ID]
			if !ok || !t.Enabled || t.LastScrape <= 0 || t.LastScrape > at.UnixMilli()+30000 || at.UnixMilli()-t.LastScrape > int64(max(90, t.IntervalSeconds*3))*1000 {
				continue
			}
			job, instance := t.Name, t.URL
			if v, ok := t.Labels["job"]; ok {
				job = v
			}
			if v, ok := t.Labels["instance"]; ok {
				instance = v
			}
			key := job + "\x00" + instance
			collectorCards[key] = append(collectorCards[key], card)
			instances = append(instances, regexp.QuoteMeta(instance))
			card.HostName = t.Name
			card.HostObservedAt = t.LastScrape
			card.HostExpiresAt = t.LastScrape + int64(max(90, t.IntervalSeconds*3))*1000
		}
	}
	if len(instances) > 0 {
		selector := `{instance=~` + strconv.Quote(strings.Join(instances, "|")) + `}`
		fresh := func(metric string) string {
			m := metric + selector
			return "(" + m + " and (timestamp(" + m + ") >= time() - 90))"
		}
		idle := strings.TrimSuffix(selector, "}") + `,mode="idle"}`
		queries := []struct{ kind, expr string }{
			{"cpu", "100 * (1 - avg by(job,instance) (rate(node_cpu_seconds_total" + idle + "[5m]) and (timestamp(node_cpu_seconds_total" + idle + ") >= time() - 90)))"},
			{"memory", "100 * (1 - " + fresh("node_memory_MemAvailable_bytes") + " / " + fresh("node_memory_MemTotal_bytes") + ")"},
			{"total", fresh("node_memory_MemTotal_bytes")}, {"available", fresh("node_memory_MemAvailable_bytes")},
			{"disk", "max by(job,instance) (100 * (1 - (node_filesystem_avail_bytes" + strings.TrimSuffix(selector, "}") + `,fstype!~"tmpfs|devtmpfs|squashfs|overlay|proc|sysfs|cgroup.*"} / node_filesystem_size_bytes` + strings.TrimSuffix(selector, "}") + `,fstype!~"tmpfs|devtmpfs|squashfs|overlay|proc|sysfs|cgroup.*"}) and (timestamp(node_filesystem_size_bytes` + selector + ") >= time() - 90)))"},
		}
		for _, q := range queries {
			s.cardVector(ctx, q.expr, at, func(labels map[string]string, v float64) {
				for _, card := range collectorCards[labels["job"]+"\x00"+labels["instance"]] {
					switch q.kind {
					case "cpu":
						if v <= 100 {
							card.CPU = &v
						}
					case "memory":
						if v <= 100 {
							card.Memory = &v
						}
					case "disk":
						if v <= 100 {
							card.Disk = &v
						}
					case "total":
						card.MemoryTotal = v
					case "available":
						card.MemoryAvailable = v
					}
				}
			})
		}
	}
	// At most two bounded queries cover every service role on the visible page.
	for _, direction := range []string{"receive", "transmit"} {
		parts := []string{}
		for _, role := range []string{"logic", "game", "gateway", "sdk"} {
			parts = append(parts, businessTrafficExpr(role, direction, `{role=`+strconv.Quote(role)+`}`, "2m", "rate"))
		}
		s.cardVector(ctx, strings.Join(parts, " or "), at, func(labels map[string]string, v float64) {
			i := model.BusinessIdentity{Service: labels["service"], Role: labels["role"], ServerID: labels["serverId"], Environment: labels["environment"], Channel: labels["channel"], Instance: labels["instance"], Implementation: labels["implementation"]}
			if card := out[store.BusinessInstanceID(i)]; card != nil {
				if direction == "receive" {
					card.Receive = &v
				} else {
					card.Transmit = &v
				}
			}
		})
	}
	return out
}
