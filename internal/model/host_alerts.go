package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// HostAlertDefaults separates raw observations (A) from the stateful threshold
// condition (B). Recovery cards retain actual values, including a genuine zero.
// This is a preview factory; callers persist through the ordinary versioned API.
func HostAlertDefaults(host ManagedHost, receiver string) []AlertRule {
	stem := "host-" + strings.ReplaceAll(host.ID, "-", "")
	out := []AlertRule{}
	for _, spec := range []struct {
		resource, metric, name, action string
		warning, recovery, pending     int
	}{
		{"cpu", "host_cpu_usage_percent", "CPU", "检查持续占用 CPU 的进程、构建任务和业务请求量。", 85, 80, 600},
		{"memory", "host_memory_used_percent", "内存", "检查进程常驻内存、缓存上限及交换空间压力。", 90, 85, 600},
		{"disk", "host_disk_used_percent", "磁盘", "检查数据、日志和备份的增长，清理前确认保留策略。", 85, 80, 900},
	} {
		for _, critical := range []bool{false, true} {
			threshold, recovery, pending, severity := spec.warning, spec.recovery, spec.pending, "warning"
			level := "预警"
			if critical {
				threshold, recovery, pending, severity, level = 95, 90, 300, "critical", "严重"
			}
			selector := fmt.Sprintf(`{managed_host=%q}`, host.ID)
			base := spec.metric + selector
			fresh := fmt.Sprintf(`(%s) and (timestamp(%s) > time() - %d) and on(managed_host) (host_resource_collection_success{managed_host=%q,resource=%q} == 1)`, base, base, max(90, host.Target.IntervalSeconds*3), host.ID, spec.resource)
			graph, _ := json.Marshal(map[string]any{"condition": "B", "data": []any{
				map[string]any{"refId": "A", "datasourceUid": "metricspanel", "relativeTimeRange": map[string]int{"from": 120, "to": 0}, "model": map[string]any{"expr": fresh, "instant": true, "intervalMs": 30000, "maxDataPoints": 1}},
				map[string]any{"refId": "B", "datasourceUid": "__expr__", "model": map[string]any{"type": "threshold", "expression": "A", "conditions": []any{map[string]any{"evaluator": map[string]any{"type": "gt", "params": []int{threshold}}, "unloadEvaluator": map[string]any{"type": "lt", "params": []int{recovery}}}}}},
			}})
			r := AlertRule{UID: stem + "-" + spec.resource + "-" + severity, Title: host.Name + " · " + spec.name + "持续偏高 · " + level, Execution: "grafana", Condition: "nonzero", Grafana: graph, IntervalSeconds: 30, ForSeconds: pending, KeepFiringForSeconds: 300, NotificationCooldownSeconds: 3600, NotificationReceiver: receiver, NoDataState: "KeepLast", ErrorState: "KeepLast", MissingSeriesEvaluations: 2, FolderUID: "general", Group: "managed-host-resources",
				Labels:      map[string]string{"managed_by": "metrics233-managed-host-alerts", "managed_host": host.ID, "host": host.Name, "resource": spec.resource, "severity": severity},
				Annotations: map[string]string{"summary": spec.name + "使用率持续偏高", "threshold": fmt.Sprintf(">%d%%，持续%d分钟", threshold, pending/60), "recovery_threshold": fmt.Sprintf("<%d%%，连续5分钟", recovery), "unit": "%", "value_ref": "A", "value_label": spec.name + "使用率", "action": spec.action, "__dashboardUid__": stem, "__panelId__": "1"},
			}
			out = append(out, r)
		}
	}
	return out
}
