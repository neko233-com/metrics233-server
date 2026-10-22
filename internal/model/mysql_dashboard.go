package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MySQLDashboard uses native collector names, not mysqld_exporter aliases.
// Each dashboard fixes the authenticated managed-host identity in its queries.
func MySQLDashboard(host ManagedHost, languages ...string) Dashboard {
	selector := fmt.Sprintf(`{managed_host=%q}`, host.ID)
	status := func(name string) string { return "mysql_global_status_" + name + selector }
	variable := func(name string) string { return "mysql_global_variables_" + name + selector }
	rate := func(name string) string { return "rate(" + status(name) + "[5m])" }
	d := Dashboard{ID: "mysql-" + strings.ReplaceAll(host.ID, "-", ""), Name: host.Name + " · MySQL", Panels: []Panel{}}
	add := func(title, kind, unit, description string, expressions, legends []string) {
		index := len(d.Panels)
		field := map[string]any{"unit": unit, "color": map[string]string{"mode": "palette-classic"}}
		if unit == "percent" {
			field["color"] = map[string]string{"mode": "thresholds"}
			field["min"] = 0
			field["max"] = 100
			field["thresholds"] = map[string]any{"mode": "absolute", "steps": []any{map[string]any{"value": nil, "color": "green"}, map[string]any{"value": 80, "color": "yellow"}, map[string]any{"value": 90, "color": "red"}}}
		}
		targets := []map[string]any{}
		for i, expr := range expressions {
			targets = append(targets, map[string]any{"refId": string(rune('A' + i)), "expr": expr, "legendFormat": legends[i], "range": kind == "timeseries", "instant": kind != "timeseries", "datasource": map[string]string{"uid": "metricspanel", "type": "prometheus"}})
		}
		config := map[string]any{"id": index + 1, "type": kind, "title": title, "description": description, "gridPos": map[string]int{"x": index % 2 * 12, "y": index / 2 * 9, "w": 12, "h": 9}, "fieldConfig": map[string]any{"defaults": field, "overrides": []any{}}, "targets": targets, "options": map[string]any{"reduceOptions": map[string]any{"calcs": []string{"lastNotNull"}}}}
		encoded, _ := json.Marshal(config)
		baseUnit := unit
		switch unit {
		case "none":
			baseUnit = "count"
		case "binBps":
			baseUnit = "bytes"
		case "s":
			baseUnit = "seconds"
		}
		d.Panels = append(d.Panels, Panel{ID: fmt.Sprintf("mysql-%d", index+1), Title: title, Aggregation: "last", Visualization: kind, Expr: expressions[0], Unit: baseUnit, Config: encoded})
	}
	add("MySQL availability", "stat", "none", "最近实际采集结果；0 为连接失败，缺样不等于恢复。", []string{"mysql_up" + selector}, []string{"MySQL"})
	var config map[string]any
	_ = json.Unmarshal(d.Panels[0].Config, &config)
	config["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["mappings"] = []any{map[string]any{"type": "value", "options": map[string]any{"0": map[string]string{"text": "Unavailable", "color": "red"}, "1": map[string]string{"text": "Available", "color": "green"}}}}
	d.Panels[0].Config, _ = json.Marshal(config)
	add("MySQL uptime", "stat", "uptime", "MySQL 实例运行时长，重启后重新计时。", []string{status("uptime")}, []string{"MySQL"})
	add("MySQL connection usage", "gauge", "percent", "当前连接数 / max_connections；容量来自固定只读参数查询。", []string{"100 * " + status("threads_connected") + " / " + variable("max_connections")}, []string{"Connections"})
	add("MySQL active threads", "stat", "none", "正在执行语句的线程数；区别于已建立但空闲的连接。", []string{status("threads_running")}, []string{"Running threads"})
	add("MySQL queries per second", "timeseries", "ops", "全局 Queries Counter 的 5 分钟平均每秒速率，包含监控等客户端查询。", []string{rate("queries")}, []string{"Queries /s"})
	add("MySQL connections", "timeseries", "none", "当前已建立连接与正在执行语句的线程数。", []string{status("threads_connected"), status("threads_running")}, []string{"Connected", "Running"})
	add("MySQL statement throughput", "timeseries", "ops", "各类 SQL 命令 Counter 的每秒速率，不是游戏异步合并前的保存请求次数。", []string{rate("com_select"), rate("com_insert"), rate("com_update"), rate("com_delete")}, []string{"SELECT", "INSERT", "UPDATE", "DELETE"})
	add("MySQL transaction throughput", "timeseries", "ops", "显式 COMMIT / ROLLBACK 的每秒速率；不代表所有 autocommit 事务。", []string{rate("com_commit"), rate("com_rollback")}, []string{"COMMIT", "ROLLBACK"})
	add("MySQL buffer pool usage", "gauge", "percent", "1 - 空闲页 / 总页数；缓冲池填满通常属于正常缓存行为，应结合命中率和读盘观察。", []string{"100 * (1 - " + status("innodb_buffer_pool_pages_free") + " / " + status("innodb_buffer_pool_pages_total") + ")"}, []string{"Buffer pool"})
	// A full cache is normal. It must not inherit utilization alarm colors.
	_ = json.Unmarshal(d.Panels[8].Config, &config)
	config["fieldConfig"].(map[string]any)["defaults"].(map[string]any)["thresholds"] = map[string]any{"mode": "absolute", "steps": []any{map[string]any{"value": nil, "color": "green"}}}
	d.Panels[8].Config, _ = json.Marshal(config)
	add("MySQL buffer pool reads", "timeseries", "ops", "逻辑读请求与实际物理读盘的每秒速率；两者为零时没有可计算的命中率。", []string{rate("innodb_buffer_pool_read_requests"), rate("innodb_buffer_pool_reads")}, []string{"Logical reads", "Physical reads"})
	add("MySQL InnoDB IO", "timeseries", "binBps", "InnoDB 累计数据读取 / 写入字节的每秒速率。", []string{rate("innodb_data_read"), rate("innodb_data_written")}, []string{"Read", "Written"})
	add("MySQL network traffic", "timeseries", "binBps", "MySQL 协议接收与发送的每秒字节。", []string{rate("bytes_received"), rate("bytes_sent")}, []string{"Received", "Sent"})
	add("MySQL current row lock waits", "stat", "none", "当前正在等待 InnoDB 行锁的操作数；这是当前值而非累计次数。", []string{status("innodb_row_lock_current_waits")}, []string{"Current waits"})
	add("MySQL row lock waits", "timeseries", "ops", "InnoDB 行锁等待累计次数的每秒速率。", []string{rate("innodb_row_lock_waits")}, []string{"Waits /s"})
	add("MySQL row lock wait time", "timeseries", "s", "累计 InnoDB 行锁等待毫秒的速率 / 1000；是累计等待秒/秒，不是单条 SQL 延迟。", []string{rate("innodb_row_lock_time") + " / 1000"}, []string{"Wait seconds /s"})
	add("MySQL aborted connections", "timeseries", "ops", "异常退出客户端和失败连接的每秒速率；瞬时非零不自动触发通知。", []string{rate("aborted_clients"), rate("aborted_connects")}, []string{"Aborted clients", "Aborted connects"})
	add("MySQL temporary tables", "timeseries", "ops", "每秒创建的临时表和落盘临时表。", []string{rate("created_tmp_tables"), rate("created_tmp_disk_tables")}, []string{"Temporary tables", "On disk"})
	add("MySQL slow query counter", "timeseries", "ops", "MySQL 按自身 long_query_time 判定的慢查询速率；不包含 SQL 文本，也不是游戏 Entity 慢 SQL 排名。", []string{rate("slow_queries")}, []string{"Slow queries /s"})
	if len(languages) > 0 && languages[0] == "zh" {
		legends := map[string]string{"Connections": "连接数", "Running threads": "执行中线程", "Queries /s": "查询 /秒", "Connected": "已连接", "Running": "执行中", "Buffer pool": "缓冲池", "Logical reads": "逻辑读取", "Physical reads": "实际读盘", "Read": "读取", "Written": "写入", "Received": "接收", "Sent": "发送", "Current waits": "当前等待", "Waits /s": "等待 /秒", "Wait seconds /s": "累计等待秒 /秒", "Aborted clients": "异常退出客户端", "Aborted connects": "失败连接", "Temporary tables": "临时表", "On disk": "落盘临时表", "Slow queries /s": "慢查询 /秒"}
		for i := range d.Panels {
			var panel map[string]any
			_ = json.Unmarshal(d.Panels[i].Config, &panel)
			for _, raw := range panel["targets"].([]any) {
				target := raw.(map[string]any)
				if translated := legends[target["legendFormat"].(string)]; translated != "" {
					target["legendFormat"] = translated
				}
			}
			if i == 0 {
				defaults := panel["fieldConfig"].(map[string]any)["defaults"].(map[string]any)
				defaults["mappings"] = []any{map[string]any{"type": "value", "options": map[string]any{"0": map[string]string{"text": "异常", "color": "red"}, "1": map[string]string{"text": "正常", "color": "green"}}}}
			}
			d.Panels[i].Config, _ = json.Marshal(panel)
		}
	}
	return d
}
