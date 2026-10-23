package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/neko233-com/metrics233-server/internal/model"
)

// Only monitoring data crosses this projection. SSH endpoints, users, host
// keys, collector URLs, secure fields and credentials remain Admin-only.
type businessHostView struct {
	Binding   model.BusinessHostBinding `json:"binding"`
	Name      string                    `json:"name"`
	Enabled   bool                      `json:"enabled"`
	Available bool                      `json:"available"`
	LastSeen  int64                     `json:"last_seen"`
	Interval  int                       `json:"interval_seconds"`
	Resources *model.HostResources      `json:"resources,omitempty"`
	Dashboard *model.Dashboard          `json:"dashboard,omitempty"`
	Rules     []model.AlertRule         `json:"rules,omitempty"`
}

func (s *Server) businessHost(ctx context.Context, id, language string) (businessHostView, error) {
	b, err := s.Store.BusinessHostBinding(ctx, id)
	out := businessHostView{Binding: b}
	if err != nil || b.Kind == "" {
		return out, err
	}
	if b.Kind == "managed_host" {
		h, _, e := s.Store.ManagedHost(ctx, b.ID)
		if errors.Is(e, sql.ErrNoRows) {
			return out, nil
		}
		if e != nil {
			return out, e
		}
		out.Name, out.Enabled, out.Available, out.LastSeen, out.Interval = h.Name, h.Enabled && !h.Revoked && h.HostCollectionEnabled(), true, h.LastSeen, h.Target.IntervalSeconds
		out.Resources = h.Resources
		rules, e := s.Store.AlertRules(ctx)
		if e != nil {
			return out, e
		}
		for _, r := range rules {
			if r.Labels["managed_host"] == b.ID {
				out.Rules = append(out.Rules, model.AlertRule{UID: r.UID, Paused: r.Paused, Labels: r.Labels, Annotations: map[string]string{"unit": r.Annotations["unit"]}, ForSeconds: r.ForSeconds, KeepFiringForSeconds: r.KeepFiringForSeconds, NotificationCooldownSeconds: r.NotificationCooldownSeconds, Grafana: r.Grafana})
			}
		}
		return out, nil
	}
	targets, err := s.Store.Targets(ctx)
	if err != nil {
		return out, err
	}
	for _, t := range targets {
		if strconv.FormatInt(t.ID, 10) == b.ID {
			out.Name, out.Enabled, out.Available, out.LastSeen, out.Interval = t.Name, t.Enabled, true, t.LastScrape, t.IntervalSeconds
			d := businessCollectorDashboard(t, language)
			out.Dashboard = &d
			return out, nil
		}
	}
	return out, nil
}

func businessCollectorDashboard(t model.Target, language string) model.Dashboard {
	// Collector-owned labels overwrite exporter labels during scrape. Freeze the
	// same job/instance identity; an observed public IP is never a host selector.
	labels := map[string]string{"job": t.Name, "instance": t.URL}
	for k, v := range t.Labels {
		labels[k] = v
	}
	selector := "{job=" + strconv.Quote(labels["job"]) + ",instance=" + strconv.Quote(labels["instance"]) + "}"
	cpuSelector := strings.TrimSuffix(selector, "}") + `,mode="idle"}`
	diskSelector := strings.TrimSuffix(selector, "}") + `,fstype!~"tmpfs|devtmpfs|squashfs|overlay|proc|sysfs|cgroup.*"}`
	d := model.HostDashboard(model.ManagedHost{ID: "collector-" + strconv.FormatInt(t.ID, 10), Name: t.Name}, language)
	d.ID = "business-host-collector-" + strconv.FormatInt(t.ID, 10)
	for i := range d.Panels {
		p := &d.Panels[i]
		expr := p.Expr
		for old, replacement := range map[string]string{
			"host_cpu_usage_percent{managed_host=\"collector-" + strconv.FormatInt(t.ID, 10) + "\"}":                 "100 * (1 - avg(rate(node_cpu_seconds_total" + cpuSelector + "[5m])))",
			"host_memory_used_percent{managed_host=\"collector-" + strconv.FormatInt(t.ID, 10) + "\"}":               "100 * (1 - node_memory_MemAvailable_bytes" + selector + " / node_memory_MemTotal_bytes" + selector + ")",
			"host_disk_used_percent{managed_host=\"collector-" + strconv.FormatInt(t.ID, 10) + "\"}":                 "(100 * (1 - node_filesystem_avail_bytes" + diskSelector + " / node_filesystem_size_bytes" + diskSelector + "))",
			"host_uptime_seconds{managed_host=\"collector-" + strconv.FormatInt(t.ID, 10) + "\"}":                    "(time() - node_boot_time_seconds" + selector + ")",
			"host_network_receive_bytes_per_second{managed_host=\"collector-" + strconv.FormatInt(t.ID, 10) + "\"}":  "rate(node_network_receive_bytes_total" + selector + "[2m])",
			"host_network_transmit_bytes_per_second{managed_host=\"collector-" + strconv.FormatInt(t.ID, 10) + "\"}": "rate(node_network_transmit_bytes_total" + selector + "[2m])",
		} {
			expr = strings.ReplaceAll(expr, old, replacement)
		}
		p.Expr = expr
		var config map[string]any
		_ = json.Unmarshal(p.Config, &config)
		if targets, ok := config["targets"].([]any); ok {
			for _, v := range targets {
				if target, ok := v.(map[string]any); ok {
					target["expr"] = expr
				}
			}
		}
		// Linux exporter semantics differ from a managed Darwin/Linux agent.
		delete(config, "description")
		p.Config, _ = json.Marshal(config)
	}
	return d
}

func (s *Server) businessHostRoutes(api routeRegistrar) {
	api.HandleFunc("GET /api/v1/business/host-options", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		type option struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		options := []option{}
		hosts, err := s.Store.ManagedHosts(r.Context())
		if err != nil {
			fail(w, 503, errors.New("host catalog unavailable"))
			return
		}
		for _, h := range hosts {
			options = append(options, option{"managed_host", h.ID, h.Name})
		}
		targets, err := s.Store.Targets(r.Context())
		if err != nil {
			fail(w, 503, errors.New("collector catalog unavailable"))
			return
		}
		for _, t := range targets {
			if t.Kind == "" || t.Kind == "prometheus" {
				options = append(options, option{"collector", strconv.FormatInt(t.ID, 10), t.Name})
			}
		}
		writeJSON(w, 200, options)
	})
	api.HandleFunc("PUT /api/v1/business/instances/{id}/host", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		var input struct {
			Version *int64  `json:"version"`
			Kind    *string `json:"kind"`
			ID      *string `json:"id"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		if input.Version == nil || input.Kind == nil || input.ID == nil {
			fail(w, 400, errors.New("version, kind and id are required"))
			return
		}
		out, err := s.Store.BindBusinessHost(r.Context(), r.PathValue("id"), model.BusinessHostBinding{Version: *input.Version, Kind: *input.Kind, ID: *input.ID})
		if err != nil {
			businessError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
}
