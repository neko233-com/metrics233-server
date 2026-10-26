package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/promql/parser"
)

const ConnectionLabel = "metrics233_connection"

// ConnectionSetupInput is deliberately shared by preview/apply. No raw sample
// values or connection secrets are returned by a preview or saved in receipts.
type ConnectionSetupInput struct {
	ID               string `json:"id"`
	ExistingTargetID int64  `json:"existing_target_id,omitempty"`
	Target           Target `json:"target"`
	Language         string `json:"language"`
	Alerts           bool   `json:"alerts"`
	Receiver         string `json:"receiver"`
}

func (v ConnectionSetupInput) Validate() error {
	id, err := uuid.Parse(v.ID)
	if err != nil || id == uuid.Nil || id.String() != v.ID {
		return errors.New("id must be a canonical nonzero UUID")
	}
	if v.Language != "en" && v.Language != "zh" {
		return errors.New("language must be en or zh")
	}
	if !v.Target.Enabled || v.Target.ID != 0 || v.Target.LastScrape != 0 || v.Target.LastError != "" || v.Target.Samples != 0 || v.Target.DurationMS != 0 || len(v.Target.SecureFields) != 0 {
		return errors.New("quick connection requires a new enabled collector")
	}
	if v.ExistingTargetID < 0 {
		return errors.New("invalid existing target identity")
	}
	if _, present := v.Target.Labels[ConnectionLabel]; present || v.ExistingTargetID == 0 && len(v.Target.Labels) > 31 {
		return errors.New("quick connection reserves one identity label")
	}
	// A user label must not merge independently configured collector identities.
	for _, key := range []string{"job", "instance"} {
		if _, present := v.Target.Labels[key]; present && v.ExistingTargetID == 0 {
			return errors.New("quick connection reserves job and instance labels")
		}
	}
	if len(v.Receiver) > 100 || strings.ContainsAny(v.Receiver, "\x00\r\n") || !v.Alerts && v.Receiver != "" {
		return errors.New("invalid notification receiver")
	}
	return v.Target.Validate()
}

func (v ConnectionSetupInput) CollectionTarget() Target {
	t := v.Target
	t.Labels = maps.Clone(t.Labels)
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	if v.ExistingTargetID == 0 {
		t.Labels[ConnectionLabel] = v.ID
	}
	return t
}

// Existing collectors retain their original series identity and credentials.
// A new quick connection gets its own reserved identity from its first scrape.
func ConnectionSelector(input ConnectionSetupInput) string {
	if input.ExistingTargetID == 0 {
		return fmt.Sprintf(`{%s=%q}`, ConnectionLabel, input.ID)
	}
	labels := maps.Clone(input.Target.Labels)
	if labels == nil {
		labels = map[string]string{}
	}
	if _, ok := labels["job"]; !ok {
		labels["job"] = input.Target.Name
	}
	if _, ok := labels["instance"]; !ok {
		labels["instance"] = input.Target.URL
	}
	parts := []string{}
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		parts = append(parts, fmt.Sprintf("%s=%q", key, labels[key]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

type ConnectionPlan struct {
	ID          string      `json:"id"`
	Profile     string      `json:"profile"`
	Samples     int         `json:"samples"`
	MetricCount int         `json:"metric_count"`
	Metrics     []string    `json:"metrics"`
	Target      Target      `json:"target"`
	Dashboard   Dashboard   `json:"dashboard"`
	Rules       []AlertRule `json:"rules"`
	PlanHash    string      `json:"plan_hash"`
}

type ConnectionReceipt struct {
	ID          string   `json:"id"`
	TargetID    int64    `json:"target_id"`
	DashboardID string   `json:"dashboard_id"`
	RuleUIDs    []string `json:"rule_uids"`
	Profile     string   `json:"profile"`
	CreatedAt   int64    `json:"created_at"`
}

// BuildConnectionPlan selects panels from actual observations. Unknown metrics
// remain accessible through Explore; a starter board is bounded to 24 panels.
func BuildConnectionPlan(input ConnectionSetupInput, samples []Sample) (ConnectionPlan, error) {
	if err := input.Validate(); err != nil {
		return ConnectionPlan{}, err
	}
	if len(samples) == 0 || len(samples) > 10000 {
		return ConnectionPlan{}, errors.New("connection must return 1–10000 finite samples")
	}
	names := map[string]bool{}
	for _, sample := range samples {
		if err := sample.Validate(); err != nil {
			return ConnectionPlan{}, errors.New("collector returned an invalid sample")
		}
		names[sample.Name] = true
	}
	all := slices.Sorted(maps.Keys(names))
	target := input.CollectionTarget()
	target.SecureSettings = nil
	target.SecureFields = map[string]bool{}
	for key, value := range input.Target.SecureSettings {
		if value != "" {
			target.SecureFields[key] = true
		}
	}
	profile := connectionProfile(input.Target.Kind, names)
	p := ConnectionPlan{ID: input.ID, Profile: profile, Samples: len(samples), MetricCount: len(all), Metrics: all[:min(len(all), 256)], Target: target, Rules: []AlertRule{}}
	p.Dashboard = connectionDashboard(input, profile, names)
	for i := range p.Dashboard.Panels {
		panel := &p.Dashboard.Panels[i]
		panel.Expr = connectionFreshExpression(panel.Expr, input)
		var cfg map[string]any
		if json.Unmarshal(panel.Config, &cfg) == nil {
			cfg["id"] = i + 1
			if targets, ok := cfg["targets"].([]any); ok {
				for _, raw := range targets {
					query := raw.(map[string]any)
					query["expr"] = connectionFreshExpression(query["expr"].(string), input)
				}
			}
			panel.Config, _ = json.Marshal(cfg)
		}
	}
	if input.Alerts {
		p.Rules = connectionAlertRules(input, p.Dashboard, profile, names)
	}
	if err := p.Dashboard.Validate(); err != nil {
		return p, err
	}
	for _, rule := range p.Rules {
		if err := rule.Validate(); err != nil {
			return p, err
		}
	}
	// Cardinality/current values may change between probes; the reviewed public
	// configuration, panel expressions and alert behavior must remain identical.
	raw, _ := json.Marshal(struct {
		Target    Target
		Profile   string
		Dashboard Dashboard
		Rules     []AlertRule
	}{p.Target, p.Profile, p.Dashboard, p.Rules})
	sum := sha256.Sum256(raw)
	p.PlanHash = hex.EncodeToString(sum[:])
	return p, nil
}

func connectionProfile(kind string, names map[string]bool) string {
	switch {
	case names["node_cpu_seconds_total"] && names["node_memory_MemTotal_bytes"]:
		return "node"
	case names["windows_cpu_time_total"]:
		return "windows"
	case kind == "mydb" || names["mydb_up"]:
		return "mydb"
	case kind == "mysql" || names["mysql_up"]:
		return "mysql"
	case kind == "redis" || names["redis_up"]:
		return "redis"
	case kind == "postgresql" || names["pg_up"]:
		return "postgresql"
	case kind == "" || kind == "prometheus":
		return "prometheus"
	default:
		return kind
	}
}

func connectionDashboard(input ConnectionSetupInput, profile string, names map[string]bool) Dashboard {
	zh := input.Language == "zh"
	tr := func(en, cn string) string {
		if zh {
			return cn
		}
		return en
	}
	selector := ConnectionSelector(input)
	d := Dashboard{ID: "conn-" + strings.ReplaceAll(input.ID, "-", ""), Name: input.Target.Name, Panels: []Panel{}}
	add := func(id, en, cn, kind, unit, expr, legend string) {
		index := len(d.Panels)
		field := map[string]any{"unit": unit, "color": map[string]string{"mode": "palette-classic"}}
		if id == "availability" {
			field["unit"] = "none"
			field["mappings"] = []any{map[string]any{"type": "value", "options": map[string]any{"0": map[string]string{"text": tr("Unavailable", "采集异常"), "color": "red"}, "1": map[string]string{"text": tr("Available", "采集正常"), "color": "green"}}}}
		}
		if unit == "percent" {
			field["min"], field["max"] = 0, 100
			field["color"] = map[string]string{"mode": "thresholds"}
			field["thresholds"] = map[string]any{"mode": "absolute", "steps": []any{map[string]any{"value": nil, "color": "green"}, map[string]any{"value": 85, "color": "yellow"}, map[string]any{"value": 95, "color": "red"}}}
		}
		cfg, _ := json.Marshal(map[string]any{"type": kind, "title": tr(en, cn), "description": tr("Scoped to this collector. Missing samples are unknown. Rates need two observations.", "仅此采集实例；缺样不当作0，速率需要至少两次采样。"), "gridPos": map[string]int{"x": index % 2 * 12, "y": index / 2 * 9, "w": 12, "h": 9}, "fieldConfig": map[string]any{"defaults": field, "overrides": []any{}}, "targets": []any{map[string]any{"refId": "A", "expr": expr, "legendFormat": legend, "range": kind == "timeseries", "instant": kind != "timeseries", "datasource": map[string]string{"uid": "metricspanel", "type": "prometheus"}}}, "options": map[string]any{"reduceOptions": map[string]any{"calcs": []string{"lastNotNull"}}}})
		d.Panels = append(d.Panels, Panel{ID: id, Title: tr(en, cn), Visualization: kind, Unit: unit, Aggregation: "last", Expr: expr, Config: cfg})
	}
	add("availability", "Collection available", "采集可用", "stat", "count", "up"+selector, tr("Available", "采集可用"))
	// A known MySQL board already defines database-specific counter semantics.
	if profile == "mysql" {
		mysql := MySQLDashboard(ManagedHost{ID: input.ID, Name: input.Target.Name}, input.Language)
		old := fmt.Sprintf(`{managed_host=%q}`, input.ID)
		for _, p := range mysql.Panels {
			p.Expr = strings.ReplaceAll(p.Expr, old, selector)
			var cfg map[string]any
			_ = json.Unmarshal(p.Config, &cfg)
			observed := true
			if targets, ok := cfg["targets"].([]any); ok {
				for _, raw := range targets {
					item := raw.(map[string]any)
					item["expr"] = strings.ReplaceAll(item["expr"].(string), old, selector)
					observed = observed && connectionMetricsPresent(item["expr"].(string), names)
				}
				p.Config, _ = json.Marshal(cfg)
			}
			if observed && connectionMetricsPresent(p.Expr, names) {
				d.Panels = append(d.Panels, p)
			}
		}
		return d
	}
	used := map[string]bool{}
	metric := func(candidates ...string) string {
		for _, name := range candidates {
			if names[name] {
				used[name] = true
				return name + selector
			}
		}
		return ""
	}
	rate := func(expr string) string { return "rate(" + expr + "[2m])" }
	known := func(id, en, cn, unit string, counter bool, candidates ...string) {
		if expr := metric(candidates...); expr != "" {
			kind := "timeseries"
			if unit == "uptime" {
				kind = "stat"
			}
			if counter {
				expr = rate(expr)
			}
			legend := tr(en, cn)
			if profile == "node" && (id == "rx" || id == "tx" || id == "disk-read" || id == "disk-write") {
				legend = "{{device}}"
			} else if profile == "windows" && (id == "rx" || id == "tx") {
				legend = "{{nic}}"
			} else if profile == "postgresql" {
				legend = "{{datname}}"
			}
			add(id, en, cn, kind, unit, expr, legend)
		}
	}
	if profile == "mydb" {
		known("mydb-up", "MyDB available", "MyDB 可用", "count", false, "mydb_up")
		if names["mydb_connections_active"] && names["mydb_max_connections"] {
			add("mydb-capacity", "MyDB connection capacity", "MyDB 连接占用", "gauge", "percent", "(100 * mydb_connections_active"+selector+" / mydb_max_connections"+selector+") and (mydb_max_connections"+selector+" > 0)", tr("Connections", "连接占用"))
		}
		for _, spec := range myDBPanels {
			known(spec.id, spec.en, spec.zh, spec.unit, spec.counter, spec.metric)
		}
		if names["mydb_query_latency_microseconds_bucket"] {
			for _, quantile := range []struct{ id, q string }{{"95", "0.95"}, {"99", "0.99"}} {
				add("mydb-p"+quantile.id, "MyDB query latency P"+quantile.id, "MyDB 查询延迟 P"+quantile.id, "timeseries", "seconds", "histogram_quantile("+quantile.q+", sum by (le) (rate(mydb_query_latency_microseconds_bucket"+selector+"[2m]))) / 1000000", "P"+quantile.id)
			}
		}
		if names["mydb_wal_sync_microseconds_total"] && names["mydb_group_commits_total"] {
			add("mydb-wal-sync", "MyDB average WAL sync time", "MyDB 平均 WAL 刷盘耗时", "timeseries", "seconds", "(rate(mydb_wal_sync_microseconds_total"+selector+"[2m]) / rate(mydb_group_commits_total"+selector+"[2m]) / 1000000) and (rate(mydb_group_commits_total"+selector+"[2m]) > 0)", "WAL sync")
		}
		return d
	}
	if profile == "node" {
		cpu := metric("node_cpu_seconds_total")
		cpu = strings.TrimSuffix(cpu, "}") + `,mode="idle"}`
		add("cpu", "CPU usage", "CPU 使用率", "gauge", "percent", "100 * (1 - avg("+rate(cpu)+"))", "CPU")
		if total, free := metric("node_memory_MemTotal_bytes"), metric("node_memory_MemAvailable_bytes"); total != "" && free != "" {
			add("memory", "Memory usage", "内存使用率", "gauge", "percent", "100 * (1 - "+free+" / "+total+")", tr("Memory", "内存"))
		}
		if size, free := metric("node_filesystem_size_bytes"), metric("node_filesystem_avail_bytes"); size != "" && free != "" {
			filter := `,fstype!~"tmpfs|devtmpfs|squashfs|nsfs|proc|sysfs|cgroup.*"}`
			size, free = strings.TrimSuffix(size, "}")+filter, strings.TrimSuffix(free, "}")+filter
			expr := "(100 * (1 - " + free + " / " + size + ")) and (" + size + " > 0)"
			if readonly := metric("node_filesystem_readonly"); readonly != "" {
				expr += " and on(device,instance,mountpoint," + ConnectionLabel + ") (" + readonly + " == 0)"
			}
			add("disk", "Filesystem usage · each volume", "磁盘容量占用 · 逐分区", "bargauge", "percent", expr, "{{mountpoint}}")
		}
		known("load", "One-minute load", "1分钟系统负载", "count", false, "node_load1")
		known("rx", "Network received · each interface", "网卡接收 · 逐网卡", "Bps", true, "node_network_receive_bytes_total")
		known("tx", "Network sent · each interface", "网卡发送 · 逐网卡", "Bps", true, "node_network_transmit_bytes_total")
		known("disk-read", "Disk read · each device", "磁盘读取 · 逐设备", "Bps", true, "node_disk_read_bytes_total")
		known("disk-write", "Disk write · each device", "磁盘写入 · 逐设备", "Bps", true, "node_disk_written_bytes_total")
		if boot := metric("node_boot_time_seconds"); boot != "" {
			add("uptime", "Host uptime", "主机运行时长", "stat", "uptime", "time() - "+boot, tr("Host uptime", "主机运行时长"))
		}
	}
	if profile == "windows" {
		cpu := metric("windows_cpu_time_total")
		cpu = strings.TrimSuffix(cpu, "}") + `,mode="idle"}`
		add("cpu", "CPU usage", "CPU 使用率", "gauge", "percent", "100 * (1 - avg("+rate(cpu)+"))", "CPU")
		if total, free := metric("windows_cs_physical_memory_bytes"), metric("windows_os_physical_memory_free_bytes"); total != "" && free != "" {
			add("memory", "Memory usage", "内存使用率", "gauge", "percent", "100 * (1 - "+free+" / "+total+")", tr("Memory", "内存"))
		}
		if total, free := metric("windows_logical_disk_size_bytes"), metric("windows_logical_disk_free_bytes"); total != "" && free != "" {
			add("disk", "Disk usage · each volume", "磁盘占用 · 逐卷", "bargauge", "percent", "100 * (1 - "+free+" / "+total+")", "{{volume}}")
		}
		known("rx", "Network received · each interface", "网卡接收 · 逐网卡", "Bps", true, "windows_net_bytes_received_total")
		known("tx", "Network sent · each interface", "网卡发送 · 逐网卡", "Bps", true, "windows_net_bytes_sent_total")
	}
	if profile == "redis" {
		known("memory-bytes", "Redis allocated memory", "Redis 已分配内存", "bytes", false, "redis_used_memory", "redis_memory_used_bytes")
		if used, max := metric("redis_used_memory", "redis_memory_used_bytes"), metric("redis_maxmemory", "redis_memory_max_bytes"); used != "" && max != "" {
			add("memory", "Memory limit usage", "内存上限使用率", "gauge", "percent", "(100 * "+used+" / "+max+") and ("+max+" > 0)", tr("Memory limit", "内存上限"))
		}
		known("clients", "Connected clients", "当前连接客户端", "count", false, "redis_connected_clients")
		known("commands", "Commands per second", "每秒执行命令", "ops", true, "redis_total_commands_processed", "redis_commands_processed_total")
		known("evictions", "Evicted keys per second", "每秒淘汰键", "ops", true, "redis_evicted_keys", "redis_evicted_keys_total")
		known("rejected", "Rejected connections per second", "每秒拒绝连接", "ops", true, "redis_rejected_connections", "redis_rejected_connections_total")
		known("uptime", "Redis uptime", "Redis 运行时长", "uptime", false, "redis_uptime_in_seconds")
		known("rx", "Redis network received", "Redis 接收流量", "Bps", true, "redis_total_net_input_bytes", "redis_net_input_bytes_total")
		known("tx", "Redis network sent", "Redis 发送流量", "Bps", true, "redis_total_net_output_bytes", "redis_net_output_bytes_total")
	}
	if profile == "postgresql" {
		known("connections", "Connections by database", "各数据库当前连接数", "count", false, "pg_stat_database_numbackends")
		known("commits", "Commits per second by database", "各数据库每秒提交事务", "ops", true, "pg_stat_database_xact_commit")
		known("rollbacks", "Rollbacks per second by database", "各数据库每秒回滚事务", "ops", true, "pg_stat_database_xact_rollback")
		known("deadlocks", "Deadlocks per second", "每秒死锁次数", "ops", true, "pg_stat_database_deadlocks")
		known("temp", "Temporary bytes per second", "临时文件写入速率", "Bps", true, "pg_stat_database_temp_bytes")
	}
	// Recognized resources get a curated board. Exporter runtime internals and
	// other observations remain in Explore, rather than burying host health.
	if profile == "node" || profile == "windows" || profile == "redis" || profile == "postgresql" {
		if profile == "node" || profile == "windows" {
			layoutConnectionHostPanels(&d)
		}
		return d
	}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		if len(d.Panels) >= 24 {
			break
		}
		if used[name] || name == "up" || strings.HasSuffix(name, "_bucket") {
			continue
		}
		unit := "count"
		expr := name + selector
		if strings.HasSuffix(name, "_bytes") {
			unit = "bytes"
		} else if strings.HasSuffix(name, "_seconds") {
			unit = "seconds"
		}
		if strings.HasSuffix(name, "_total") {
			expr = rate(expr)
			unit = "ops"
			if strings.HasSuffix(name, "_bytes_total") {
				unit = "Bps"
			}
		}
		add(fmt.Sprintf("metric-%d", len(d.Panels)), strings.ReplaceAll(name, "_", " "), strings.ReplaceAll(name, "_", " "), "timeseries", unit, expr, "")
	}
	return d
}

// Give health instruments the first row and keep collection status compact.
// Existing saved dashboards are never reflowed by a later connection retry.
func layoutConnectionHostPanels(d *Dashboard) {
	y := 0
	for _, row := range []struct {
		ids    []string
		height int
	}{
		{[]string{"cpu", "memory"}, 12},
		{[]string{"disk"}, 8},
		{[]string{"rx", "tx"}, 12},
		{[]string{"disk-read", "disk-write"}, 12},
		{[]string{"load", "uptime"}, 10},
		{[]string{"availability"}, 6},
	} {
		indexes := []int{}
		for _, id := range row.ids {
			for i := range d.Panels {
				if d.Panels[i].ID == id {
					indexes = append(indexes, i)
				}
			}
		}
		if len(indexes) == 0 {
			continue
		}
		for column, index := range indexes {
			var config map[string]any
			_ = json.Unmarshal(d.Panels[index].Config, &config)
			config["gridPos"] = map[string]int{"x": column * (24 / len(indexes)), "y": y, "w": 24 / len(indexes), "h": row.height}
			d.Panels[index].Config, _ = json.Marshal(config)
		}
		y += row.height
	}
}

func connectionMetricsPresent(expr string, names map[string]bool) bool {
	parsed, err := parser.NewParser(parser.Options{}).ParseExpr(expr)
	if err != nil {
		return false
	}
	ok := true
	parser.Inspect(parsed, func(node parser.Node, _ []parser.Node) error {
		if vector, yes := node.(*parser.VectorSelector); yes && !names[vector.Name] {
			ok = false
		}
		return nil
	})
	return ok
}

func connectionFreshExpression(expr string, input ConnectionSetupInput) string {
	parsed, err := parser.NewParser(parser.Options{}).ParseExpr(expr)
	if err != nil {
		return expr
	}
	names := map[string]bool{}
	parser.Inspect(parsed, func(node parser.Node, _ []parser.Node) error {
		if v, ok := node.(*parser.VectorSelector); ok && v.Name != "" {
			names[v.Name] = true
		}
		return nil
	})
	for _, name := range slices.Sorted(maps.Keys(names)) {
		expr = fmt.Sprintf(`(%s) and on() (min(timestamp(%s%s)) > time() - %d)`, expr, name, ConnectionSelector(input), max(90, input.Target.IntervalSeconds*3))
	}
	return expr
}

func connectionAlertRules(input ConnectionSetupInput, d Dashboard, profile string, names map[string]bool) []AlertRule {
	selector := ConnectionSelector(input)
	zh := input.Language == "zh"
	tr := func(en, cn string) string {
		if zh {
			return cn
		}
		return en
	}
	makeRule := func(id, title, expr, compare string, threshold, recovery float64, pending int, severity, unit string) AlertRule {
		panelID := 1
		for i, p := range d.Panels {
			if p.ID == strings.TrimSuffix(id, "-critical") {
				panelID = i + 1
			}
		}
		fresh := fmt.Sprintf(`(%s) and on() (max(timestamp(up%s)) > time() - %d)`, expr, selector, max(90, input.Target.IntervalSeconds*3))
		if id != "unavailable" {
			fresh += fmt.Sprintf(` and on() (max(up%s) == 1)`, selector)
		}
		unload := "lt"
		if compare == "lt" {
			unload = "gt"
		}
		graph, _ := json.Marshal(map[string]any{"condition": "B", "data": []any{map[string]any{"refId": "A", "datasourceUid": "metricspanel", "relativeTimeRange": map[string]int{"from": max(120, input.Target.IntervalSeconds*3), "to": 0}, "model": map[string]any{"expr": fresh, "instant": true}}, map[string]any{"refId": "B", "datasourceUid": "__expr__", "model": map[string]any{"type": "threshold", "expression": "A", "conditions": []any{map[string]any{"evaluator": map[string]any{"type": compare, "params": []float64{threshold}}, "unloadEvaluator": map[string]any{"type": unload, "params": []float64{recovery}}}}}}}})
		r := AlertRule{UID: "conn-" + strings.ReplaceAll(input.ID, "-", "") + "-" + id, Title: input.Target.Name + " · " + title, Execution: "grafana", Condition: "nonzero", Grafana: graph, IntervalSeconds: 30, ForSeconds: pending, KeepFiringForSeconds: 60, NotificationCooldownSeconds: 3600, NotificationReceiver: input.Receiver, NoDataState: "KeepLast", ErrorState: "KeepLast", MissingSeriesEvaluations: 2, FolderUID: "general", Group: "connection-health", Labels: map[string]string{"managed_by": "metrics233-quick-connect", ConnectionLabel: input.ID, "severity": severity, "profile": profile}, Annotations: map[string]string{"summary": title, "value_ref": "A", "value_label": title, "unit": unit, "__dashboardUid__": d.ID, "__panelId__": fmt.Sprint(panelID), "action": tr("Inspect collection status and the resource dashboard before changing capacity.", "先检查采集状态和资源仪表盘，再决定是否调整容量。")}}
		return r
	}
	up := "up" + selector
	backend := up
	for _, name := range []string{"mysql_up", "mydb_up", "redis_up", "pg_up"} {
		if names[name] {
			backend += " * " + name + selector
		}
	}
	available := connectionFreshExpression(up, input)
	if backend != up {
		// A failed scrape still publishes fresh up=0. Its old backend _up
		// must not turn a continuous failure into no-data before pending ends.
		available = "((" + available + ") == 0) or (" + connectionFreshExpression(backend, input) + ")"
	}
	out := []AlertRule{makeRule("unavailable", tr("Collection unavailable for 2 minutes", "采集连续2分钟失败"), available, "lt", 0.5, 0.5, 120, "critical", "")}
	for _, p := range d.Panels {
		resource := ""
		warning, recovery, pending := 85.0, 80.0, 600
		switch p.ID {
		case "cpu":
			resource = tr("CPU remains high", "CPU 持续偏高")
		case "memory":
			resource = tr("Memory remains high", "内存使用率持续偏高")
			warning, recovery = 90, 85
		case "disk":
			resource = tr("Disk remains full", "磁盘占用持续偏高")
			pending = 900
		case "mysql-3", "mydb-capacity":
			resource = tr("MySQL connections remain high", "MySQL 连接占用持续偏高")
			if p.ID == "mydb-capacity" {
				resource = tr("MyDB connections remain high", "MyDB 连接占用持续偏高")
			}
			pending = 300
		}
		if resource != "" {
			out = append(out, makeRule(p.ID, resource, p.Expr, "gt", warning, recovery, pending, "warning", "%"))
			criticalPending := 300
			if p.ID == "mysql-3" || p.ID == "mydb-capacity" {
				criticalPending = 120
			}
			out = append(out, makeRule(p.ID+"-critical", resource+tr(" · critical", " · 严重"), p.Expr, "gt", 95, 90, criticalPending, "critical", "%"))
		}
	}
	if profile == "mydb" {
		for _, spec := range myDBAlerts {
			if names[spec.metric] {
				expr := spec.metric + selector
				if spec.counter {
					expr = "rate(" + expr + "[2m])"
				}
				recovery := spec.threshold
				if spec.counter {
					recovery = 0.000001 // Zero rates must satisfy the strict recovery comparator.
				}
				out = append(out, makeRule(spec.id, tr(spec.en, spec.zh), connectionFreshExpression(expr, input), spec.compare, spec.threshold, recovery, 120, spec.severity, ""))
			}
		}
	}
	return out
}
