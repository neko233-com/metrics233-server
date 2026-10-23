package server

import (
	"encoding/json"

	"github.com/neko233-com/metrics233-server/internal/model"
)

func businessProcessPanels(item model.BusinessInstance) []model.Panel {
	selector := "{${business_scope:raw}}"
	latest := func(metric string) string {
		m := metric + selector
		return "(" + m + " and (timestamp(" + m + ") == scalar(max(timestamp(" + m + ")))) and (timestamp(" + m + ") >= time() - 90))"
	}
	current := func(metric string) string { l := latest(metric); return "sum(" + l + ") and (count(" + l + ") == 1)" }
	rate := func(metric string) string {
		m := metric + selector
		return "sum(rate(" + m + "[5m]) and (timestamp(" + m + ") == scalar(max(timestamp(" + m + "))))) and (count(" + latest(metric) + ") == 1)"
	}
	panels := []model.Panel{}
	add := func(id, title, kind, unit, expr, description string) {
		cfg, _ := json.Marshal(map[string]any{"type": kind, "description": description, "targets": []any{map[string]any{"refId": "A", "expr": expr, "legendFormat": title, "instant": kind == "stat", "range": kind != "stat", "datasource": map[string]string{"uid": "metricspanel", "type": "prometheus"}}}, "options": map[string]any{"reduceOptions": map[string]any{"calcs": []string{"lastNotNull"}}}, "fieldConfig": map[string]any{"defaults": map[string]any{"unit": unit, "color": map[string]string{"mode": "palette-classic"}}, "overrides": []any{}}})
		panels = append(panels, model.Panel{ID: id, Title: title, Visualization: kind, Unit: unit, Aggregation: "last", Expr: expr, Config: cfg})
	}
	cpu := "100 * (" + rate("process_cpu_seconds_total") + ")"
	add("process-cpu", "Process CPU · one core = 100%", "stat", "percent", cpu, "100% means one fully utilized CPU core; a multithreaded process can exceed 100%. This is not whole-host utilization.")
	add("process-memory", "Process memory", "stat", "bytes", current("process_resident_memory_bytes"), "Resident physical memory used by this server process, not all memory used on the host.")
	add("process-uptime", "Process uptime", "stat", "uptime", "(time() - ("+current("process_start_time_seconds")+")) >= 0", "Time since this process started; resets on restart.")
	add("process-cpu-trend", "Process CPU over time", "timeseries", "percent", cpu, "Five-minute counter rate. 100% means one fully utilized CPU core.")
	add("process-memory-trend", "Process memory over time", "timeseries", "bytes", current("process_resident_memory_bytes"), "Resident physical memory of this process.")
	if item.Identity.Implementation == "go" {
		add("process-go-heap", "Go heap memory", "stat", "bytes", current("go_memstats_heap_alloc_bytes"), "Allocated Go heap; excludes stacks and other resident process memory.")
		add("process-goroutines", "Go goroutines", "stat", "short", current("go_goroutines"), "Current goroutines in this server process.")
	}
	if item.Identity.Implementation == "rust" {
		add("process-tokio-tasks", "Tokio active tasks", "stat", "short", current("tokio_tasks_alive"), "Actual live Tokio tasks; not Go goroutines.")
		add("process-tokio-workers", "Tokio worker threads", "stat", "short", current("tokio_workers"), "Runtime worker threads, independent of task count.")
	}
	add("process-http-connections", "HTTP connections", "stat", "short", current("http_connections_active"), "Accepted HTTP connections currently owned by the HTTP server; upgraded WebSockets are counted separately. HTTP/2 streams are requests, not connections.")
	add("process-websocket-connections", "WebSocket connections", "stat", "short", current("websocket_connections_active"), "All established inbound WebSockets, including client and service links. SDK outbound gateway links use sdk_gateway_links_connected.")
	add("process-http-inflight", "HTTP requests in flight", "stat", "short", current("http_requests_inflight"), "Handlers currently executing. Connection idle time is not an in-flight request.")
	add("process-http-rps", "HTTP requests / second", "timeseries", "ops", rate("http_requests_total"), "Completed handler calls per second, including upgrade handshakes.")
	add("process-http-errors", "HTTP server errors / second", "timeseries", "ops", rate("http_requests_failed_total"), "Responses with status 500–599, independently of successful requests.")
	if item.Identity.Role == "gateway" {
		add("gateway-connections", "Active gateway connections", "stat", "short", current("gateway_connections_active"), "Currently established client connections.")
		add("gateway-queue", "Pending gateway writes", "stat", "short", current("gateway_client_write_queue_depth"), "Messages currently waiting in client write queues.")
		add("gateway-receive", "Gateway received messages / second", "timeseries", "ops", rate("gateway_messages_received_total"), "Actual received message counter rate over five minutes.")
		add("gateway-send", "Gateway sent messages / second", "timeseries", "ops", rate("gateway_messages_sent_total"), "Actual sent message counter rate over five minutes.")
		add("gateway-dropped", "Dropped gateway writes / second", "timeseries", "ops", rate("gateway_client_write_queue_dropped_total"), "Client write queue drops per second over five minutes.")
	}
	if item.Identity.Role == "sdk" {
		add("process-sdk-links", "SDK outbound gateway links", "stat", "short", current("sdk_gateway_links_connected"), "Established SDK-to-gateway service WebSockets; separate from inbound sockets.")
		add("process-sdk-rpc-pending", "SDK pending RPC", "stat", "short", current("sdk_rpc_pending"), "Bounded requests currently awaiting authoritative replies.")
		return panels
	}
	prefix := "game_"
	if item.Identity.Role == "gateway" {
		prefix = "gateway_"
	}
	add("traffic-in", "Received bytes / second", "timeseries", "Bps", rate(prefix+"bytes_received_total"), "Inbound traffic rate over five minutes.")
	add("traffic-out", "Sent bytes / second", "timeseries", "Bps", rate(prefix+"bytes_sent_total"), "Outbound traffic rate over five minutes.")
	return panels
}
