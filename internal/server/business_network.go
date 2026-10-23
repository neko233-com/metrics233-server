package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/neko233-com/metrics233-server/internal/model"
)

func defaultNetworkDevice(devices []string) string {
	for _, device := range devices {
		lower := strings.ToLower(device)
		if strings.HasPrefix(lower, "eth") || strings.HasPrefix(lower, "en") || strings.HasPrefix(lower, "bond") {
			return device
		}
	}
	for _, device := range devices {
		lower := strings.ToLower(device)
		virtual := false
		for _, prefix := range []string{"docker", "veth", "br-", "bridge", "virbr", "cni", "flannel", "utun", "tun", "tap", "lo"} {
			if strings.HasPrefix(lower, prefix) {
				virtual = true
				break
			}
		}
		if !virtual {
			return device
		}
	}
	if len(devices) > 0 {
		return devices[0]
	}
	return ""
}

func networkPanel(id, title, kind, unit, description string, expressions, legends []string) model.Panel {
	targets := []any{}
	for i, expr := range expressions {
		targets = append(targets, map[string]any{"refId": string(rune('A' + i)), "expr": expr, "legendFormat": legends[i], "instant": kind == "stat", "range": kind != "stat", "datasource": map[string]string{"uid": "metricspanel", "type": "prometheus"}})
	}
	config, _ := json.Marshal(map[string]any{"type": kind, "title": title, "description": description, "targets": targets, "fieldConfig": map[string]any{"defaults": map[string]any{"unit": unit, "color": map[string]string{"mode": "palette-classic"}}, "overrides": []any{}}, "options": map[string]any{"reduceOptions": map[string]any{"calcs": []string{"lastNotNull"}}}})
	return model.Panel{ID: id, Title: title, Visualization: kind, Unit: unit, Aggregation: "last", Expr: expressions[0], Config: config}
}

func businessNetworkDashboard(item model.BusinessInstance, language string) model.Dashboard {
	zh := language == "zh"
	tr := func(en, cn string) string {
		if zh {
			return cn
		}
		return en
	}
	selector := "{" + businessSelector(item) + "}"
	role := item.Identity.Role
	d := model.Dashboard{ID: "network-" + item.ID[:24], Name: tr("Service traffic", "业务流量"), Panels: []model.Panel{}}
	description := tr("Actual service payload bytes; excludes other host processes, TLS/IP headers and monitoring uploads. Counter increases are estimates between observations, not billing totals.", "当前服务实际业务字节，不含整机其他进程、TLS/IP 包头和监控上报；窗口累计为采样间计数器增量估算，不作计费。")
	if role == "sdk" {
		description = tr("HTTP request/response body bytes actually read or written; excludes WebSockets, headers, TLS and other processes.", "实际读取和写出的 HTTP 请求体/响应体字节，不含 WebSocket、请求头、TLS 和其他进程。")
	}
	inTitle, outTitle := tr("Download · receive", "下行 · 接收"), tr("Upload · send", "上行 · 发送")
	inRate := businessTrafficExpr(role, "receive", selector, "2m", "rate")
	outRate := businessTrafficExpr(role, "transmit", selector, "2m", "rate")
	d.Panels = append(d.Panels,
		networkPanel("network-receive", inTitle, "stat", "Bps", description, []string{inRate}, []string{inTitle}),
		networkPanel("network-transmit", outTitle, "stat", "Bps", description, []string{outRate}, []string{outTitle}),
		networkPanel("network-receive-volume", tr("Received · last 60 min ≈", "最近60分钟接收 ≈"), "stat", "bytes", description, []string{businessTrafficExpr(role, "receive", selector, "1h", "increase")}, []string{inTitle}),
		networkPanel("network-transmit-volume", tr("Sent · last 60 min ≈", "最近60分钟发送 ≈"), "stat", "bytes", description, []string{businessTrafficExpr(role, "transmit", selector, "1h", "increase")}, []string{outTitle}),
		networkPanel("network-history", tr("Service traffic · last hour", "业务收发趋势 · 最近1小时"), "timeseries", "Bps", description, []string{inRate, outRate}, []string{inTitle, outTitle}),
	)
	return d
}

func hostNetworkDashboard(kind, id, device, language string, target *model.Target) model.Dashboard {
	zh := language == "zh"
	tr := func(en, cn string) string {
		if zh {
			return cn
		}
		return en
	}
	hash := sha256.Sum256([]byte(kind + ":" + id))
	d := model.Dashboard{ID: "host-network-" + hex.EncodeToString(hash[:12]), Name: tr("Host interface traffic", "所在主机 · 网卡流量"), Panels: []model.Panel{}}
	selector := `{managed_host=` + strconv.Quote(id) + `,device=` + strconv.Quote(device) + `}`
	in, out := "host_network_receive_bytes_per_second"+selector, "host_network_transmit_bytes_per_second"+selector
	if kind == "collector" && target != nil {
		job, instance := target.Name, target.URL
		if v, ok := target.Labels["job"]; ok {
			job = v
		}
		if v, ok := target.Labels["instance"]; ok {
			instance = v
		}
		selector = `{job=` + strconv.Quote(job) + `,instance=` + strconv.Quote(instance) + `,device=` + strconv.Quote(device) + `}`
		in = "rate(node_network_receive_bytes_total" + selector + "[2m])"
		out = "rate(node_network_transmit_bytes_total" + selector + "[2m])"
	}
	desc := tr("One interface only. Physical, bridge and tunnel counters overlap and must not be summed.", "单张网卡口径。物理网卡、网桥和隧道可能经过同一批数据，不跨网卡相加。")
	d.Panels = append(d.Panels, networkPanel("host-network-current-in", tr("Interface receive", "网卡接收"), "stat", "Bps", desc, []string{in}, []string{device}), networkPanel("host-network-current-out", tr("Interface send", "网卡发送"), "stat", "Bps", desc, []string{out}, []string{device}), networkPanel("host-network-history", tr("Interface traffic · last hour", "网卡收发趋势 · 最近1小时"), "timeseries", "Bps", desc, []string{in, out}, []string{tr("Receive", "接收"), tr("Send", "发送")}))
	return d
}
