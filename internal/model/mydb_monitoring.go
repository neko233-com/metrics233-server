package model

// Fixed metric names only: SQL text, usernames and unbounded digests never
// become dashboard identities or default alert labels.
var myDBPanels = []struct {
	id, en, zh, unit, metric string
	counter                  bool
}{
	{"mydb-connections", "MyDB active connections", "MyDB 当前连接数", "count", "mydb_connections_active", false},
	{"mydb-queries", "MyDB queries per second", "MyDB 每秒查询", "ops", "mydb_queries_total", true},
	{"mydb-errors", "MyDB query errors per second", "MyDB 每秒查询错误", "ops", "mydb_query_errors_total", true},
	{"mydb-slow", "MyDB slow queries per second", "MyDB 每秒慢查询", "ops", "mydb_slow_queries_total", true},
	{"mydb-auth", "MyDB failed logins per second", "MyDB 每秒登录失败", "ops", "mydb_authentication_failures_total", true},
	{"mydb-audit", "MyDB audit writer healthy", "MyDB 审计写入健康", "count", "mydb_audit_healthy", false},
	{"mydb-audit-dropped", "MyDB rejected audit events per second", "MyDB 每秒审计拒绝事件", "ops", "mydb_audit_rejections_total", true},
	{"mydb-storage-errors", "MyDB storage errors per second", "MyDB 每秒存储错误", "ops", "mydb_storage_errors_total", true},
	{"mydb-checkpoint-errors", "MyDB checkpoint errors per second", "MyDB 每秒检查点错误", "ops", "mydb_checkpoint_errors_total", true},
	{"mydb-secure-transport", "MyDB requires TLS", "MyDB 强制 TLS", "count", "mydb_security_require_secure_transport", false},
	{"mydb-strong-passwords", "MyDB enforces strong passwords", "MyDB 强密码检查", "count", "mydb_security_strong_passwords_enforced", false},
	{"mydb-local-infile", "MyDB LOCAL INFILE enabled", "MyDB LOCAL INFILE 开关", "count", "mydb_security_local_infile_enabled", false},
	{"mydb-uptime", "MyDB uptime", "MyDB 运行时长", "uptime", "mydb_uptime_seconds", false},
	{"mydb-lock-waits", "MyDB lock waits per second", "MyDB 每秒锁等待", "ops", "mydb_lock_waits_total", true},
	{"mydb-deadlocks", "MyDB deadlocks per second", "MyDB 每秒死锁", "ops", "mydb_deadlocks_total", true},
	{"mydb-commit-queue", "MyDB queued commits", "MyDB 等待提交数", "count", "mydb_write_commit_queue_depth", false},
	{"mydb-commits", "MyDB commit groups per second", "MyDB 每秒提交组", "ops", "mydb_group_commits_total", true},
	{"mydb-writes", "MyDB storage write batches per second", "MyDB 每秒存储写批次", "ops", "mydb_storage_write_batches_total", true},
}

var myDBAlerts = []struct {
	id, en, zh, metric, compare, severity string
	threshold                             float64
	counter                               bool
}{
	{"mydb-auth", "MyDB failed logins persist", "MyDB 登录失败持续发生", "mydb_authentication_failures_total", "gt", "warning", 0, true},
	{"mydb-audit", "MyDB audit writer unhealthy", "MyDB 审计写入异常", "mydb_audit_healthy", "lt", "critical", 0.5, false},
	{"mydb-audit-dropped", "MyDB audit events rejected", "MyDB 审计事件持续被拒绝", "mydb_audit_rejections_total", "gt", "critical", 0, true},
	{"mydb-storage-errors", "MyDB storage errors persist", "MyDB 存储错误持续发生", "mydb_storage_errors_total", "gt", "critical", 0, true},
	{"mydb-checkpoint-errors", "MyDB checkpoint errors persist", "MyDB 检查点错误持续发生", "mydb_checkpoint_errors_total", "gt", "critical", 0, true},
	{"mydb-secure-transport", "MyDB does not require TLS", "MyDB 未强制 TLS", "mydb_security_require_secure_transport", "lt", "warning", 0.5, false},
	{"mydb-strong-passwords", "MyDB strong password checks disabled", "MyDB 强密码检查未启用", "mydb_security_strong_passwords_enforced", "lt", "warning", 0.5, false},
	{"mydb-local-infile", "MyDB LOCAL INFILE enabled", "MyDB 启用了 LOCAL INFILE", "mydb_security_local_infile_enabled", "gt", "warning", 0.5, false},
}
