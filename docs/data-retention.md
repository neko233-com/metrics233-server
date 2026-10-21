# Metric data retention

New workspaces retain 15 days of metric samples by default. `serve --retention-days N` chooses the initial fallback (1–3650); an existing explicit startup value is preserved until the administrator saves a workspace setting.

SQLite: open Collectors → Data retention, enter the number of days and save. GET `/api/v1/settings/retention` returns `days`, `version`, `editable`, `backend` and `cleanup_interval_seconds`. PUT requires the current `version` and integer `days`; Admin only. Stale revisions return 409 and are never retried automatically. The saved setting survives restarts and takes precedence over the startup fallback.

Cleanup runs at startup and once per hour. Each SQLite cleanup reads the effective lifetime and deletes older samples in the same transaction, serialized with setting updates. A failed setting read never guesses a shorter lifetime. The cutoff is exclusive: a sample exactly at `now - N*24h` remains. Empty metric series are removed; accounts, dashboards, collectors, alert rules, contacts and source policies remain. Alert/notification history has its separate bounded retention contract.

Shortening the lifetime authorizes permanent deletion at the next cleanup. Extending it cannot restore already deleted samples. SQLite reuses freed pages; pruning does not promise an immediate smaller database file and does not run a blocking VACUUM. Normal backup/restore is still required for recovery.

ClickHouse retains explicit startup/TTL management: the settings endpoint is read-only (`editable:false`), and PUT is rejected. Existing tables need an operator-approved `ALTER TABLE ... MODIFY TTL`; changing a UI or CLI value alone does not silently rewrite a table TTL. This feature does not migrate data between backends.
