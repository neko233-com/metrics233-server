package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// HostDashboard pins the authenticated host ID, independent of display names or
// a database URL. Repeated provisioning returns the existing customized board.
func HostDashboard(host ManagedHost, language string) Dashboard {
	zh := language == "zh"
	tr := func(en, cn string) string {
		if zh {
			return cn
		}
		return en
	}
	d := Dashboard{ID: "host-" + strings.ReplaceAll(host.ID, "-", ""), Name: host.Name + tr(" · Host resources", " · 主机资源"), Panels: []Panel{}}
	selector := fmt.Sprintf(`{managed_host=%q}`, host.ID)
	add := func(title, kind, unit, expr, legend, description string, x, y, w, h int, warning, critical float64) {
		id := len(d.Panels) + 1
		field := map[string]any{"unit": unit, "color": map[string]string{"mode": "palette-classic"}}
		if unit == "percent" {
			field["min"] = 0
			field["max"] = 100
			field["color"] = map[string]string{"mode": "thresholds"}
			field["thresholds"] = map[string]any{"mode": "absolute", "steps": []any{map[string]any{"value": nil, "color": "green"}, map[string]any{"value": warning, "color": "yellow"}, map[string]any{"value": critical, "color": "red"}}}
			field["custom"] = map[string]any{"thresholdsStyle": map[string]string{"mode": "line+area"}}
		}
		cfg := map[string]any{"id": id, "type": kind, "title": title, "description": description, "gridPos": map[string]int{"x": x, "y": y, "w": w, "h": h}, "fieldConfig": map[string]any{"defaults": field, "overrides": []any{}}, "targets": []any{map[string]any{"refId": "A", "expr": expr, "legendFormat": legend, "range": kind == "timeseries", "instant": kind != "timeseries", "datasource": map[string]string{"uid": "metricspanel", "type": "prometheus"}}}, "options": map[string]any{"reduceOptions": map[string]any{"calcs": []string{"lastNotNull"}}}}
		encoded, _ := json.Marshal(cfg)
		d.Panels = append(d.Panels, Panel{ID: fmt.Sprintf("host-%d", id), Title: title, Visualization: kind, Aggregation: "last", Expr: expr, Unit: unit, Config: encoded})
	}
	titles := []string{tr("CPU usage", "CPU 使用率"), tr("Memory usage", "内存使用率"), tr("Disk usage", "磁盘使用率")}
	exprs := []string{"host_cpu_usage_percent" + selector, "host_memory_used_percent" + selector, "max(host_disk_used_percent" + selector + ")"}
	desc := []string{tr("CPU busy time between observations; no initial synthetic zero.", "相邻采样的 CPU 忙碌时间占比；首次采样建立基线，不显示伪零值。"), tr("1 - available / total. Reclaimable memory counts as available.", "1 - 可用内存 / 总内存；可回收内存算作可用。"), tr("Highest writable local volume usage. APFS helper volumes are excluded; shared capacity must not be summed.", "可写本地数据卷的最高占用；排除 APFS 辅助卷，共享容器容量不能相加。")}
	for i := 0; i < 3; i++ {
		warn := 85.0
		if i == 1 {
			warn = 90
		}
		add(titles[i], "gauge", "percent", exprs[i], titles[i], desc[i], i*8, 0, 8, 10, warn, 95)
		add(titles[i]+tr(" · last 60 min", " · 最近60分钟"), "timeseries", "percent", exprs[i], titles[i], desc[i], i*8, 10, 8, 10, warn, 95)
	}
	add(tr("Writable disks", "可写磁盘"), "bargauge", "percent", "host_disk_used_percent"+selector, "{{mountpoint}}", desc[2], 0, 20, 16, 8, 85, 95)
	add(tr("Host uptime", "主机运行时长"), "stat", "uptime", "host_uptime_seconds"+selector, tr("Host uptime", "主机运行时长"), tr("Operating system uptime, not the collector process age.", "操作系统运行时长，区别于采集器进程时长。"), 16, 20, 8, 8, 0, 0)
	return d
}
