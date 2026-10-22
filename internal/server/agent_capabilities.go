package server

import "net/http"

// This is a discovery contract for native operations, not an OpenAPI promise for
// every compatibility/plugin route. The route inventory comes from the actual
// registration pass and contains no environment, credentials or resource data.
type agentOperation struct {
	ID          string         `json:"id"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	MinimumRole string         `json:"minimum_role"`
	Effect      string         `json:"effect"`
	Concurrency string         `json:"concurrency"`
	ReadBack    string         `json:"read_back,omitempty"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"request_schema,omitempty"`
}

func nativeAgentOperations() []agentOperation {
	return []agentOperation{
		{ID: "hosts.revoke", Method: "POST", Path: "/api/v1/managed-hosts/{id}/revoke", MinimumRole: "Admin", Effect: "revoke credential", Concurrency: "body.version; stale 409", ReadBack: "/api/v1/managed-hosts", Description: "Immediately invalidate the old host credential and pause collection. Reinstall with the new protected bootstrap before enabling; measurements and customized dashboards remain."},
		{ID: "hosts.list", Method: "GET", Path: "/api/v1/managed-hosts", MinimumRole: "Admin", Effect: "read", Concurrency: "none", Description: "Masked SSH and collector configuration, recent outbound heartbeat and applied revision; source data is separate from installation status."},
		{ID: "hosts.save", Method: "PUT", Path: "/api/v1/managed-hosts/{id}", MinimumRole: "Admin", Effect: "configure", Concurrency: "body.version; stale 409", ReadBack: "/api/v1/managed-hosts", Description: "Update a managed host or pause collection. Credentials are write-only; omitted credentials are retained. Only the agent's own collection configuration is available through its independent outbound credential."},
		{ID: "hosts.deploy", Method: "POST", Path: "/api/v1/managed-hosts/{id}/deploy", MinimumRole: "Admin", Effect: "install", Concurrency: "body.version, one active installation per host, two SSH installations globally", ReadBack: "/api/v1/managed-hosts", Description: "Pinned SSH fingerprint, fixed Linux/macOS amd64/arm64 artifact with checksum, non-root system service and rollback. Unreachable private networks require hosts deploy-local from a reachable LAN executor. Installation success is not a first heartbeat or MySQL success."},
		{ID: "hosts.dashboard", Method: "POST", Path: "/api/v1/managed-hosts/{id}/dashboard", MinimumRole: "Admin", Effect: "configure", Concurrency: "create if absent; retain customized existing dashboard", ReadBack: "/api/v1/dashboards", Description: "Create a native host-resource or MySQL dashboard with fixed host scope. Later calls keep existing custom panels and revisions."},
		{ID: "metrics.query", Method: "GET", Path: "/prometheus/api/v1/query", MinimumRole: "Viewer", Effect: "read", Concurrency: "none", Description: "PromQL instant query; inspect status/error and data, never infer zero from missing data."},
		{ID: "retention.get", Method: "GET", Path: "/api/v1/settings/retention", MinimumRole: "Viewer", Effect: "read", Concurrency: "none", Description: "Read effective metric retention, backend writability and version."},
		{ID: "retention.save", Method: "PUT", Path: "/api/v1/settings/retention", MinimumRole: "Admin", Effect: "configure", Concurrency: "body.version; 409 requires re-read and a new decision", ReadBack: "/api/v1/settings/retention", Description: "Embedded/SQLite only; expiration permanently removes metric samples, preserving monitoring/account configuration.", Schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"days", "version"}, "properties": map[string]any{"days": map[string]any{"type": "integer", "minimum": 1, "maximum": 3650}, "version": map[string]any{"type": "integer", "minimum": 0}}}},
		{ID: "targets.save", Method: "POST", Path: "/api/v1/targets", MinimumRole: "Editor", Effect: "configure", Concurrency: "no CAS; read/backup/read, write once, then verify; do not retry an uncertain write", ReadBack: "/api/v1/targets", Description: "Create a collector with an explicit unique identity. Secrets are write-only. Inspect /api/v1/collectors/catalog first."},
		{ID: "alerts.preview", Method: "POST", Path: "/api/v1/alerts/preview", MinimumRole: "Viewer", Effect: "preview", Concurrency: "none", Description: "Evaluate a candidate query graph without saving a rule or sending a notification."},
		{ID: "alerts.save", Method: "PUT", Path: "/api/v1/alerts/rules/{uid}", MinimumRole: "Editor", Effect: "configure", Concurrency: "body.version; 409 requires re-read", ReadBack: "/api/v1/alerts/rules/{uid}", Description: "Set sustained pending, recovery and notification cooldown explicitly. Missing/error policy must be deliberate; do not use evaluate as a side-effect-free preview."},
		{ID: "notifications.preview", Method: "POST", Path: "/api/v1/alerting/contact-points/preview", MinimumRole: "Admin", Effect: "preview", Concurrency: "none", Description: "Render a Feishu card/template locally; does not queue or send a message.", Schema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"feishu_format": map[string]any{"enum": []string{"interactive", "text"}}, "status": map[string]any{"enum": []string{"firing", "resolved"}}, "title_template": map[string]any{"type": "string"}, "message_template": map[string]any{"type": "string"}}}},
		{ID: "notifications.save", Method: "PUT", Path: "/api/v1/alerting/contact-points/{uid}", MinimumRole: "Admin", Effect: "configure", Concurrency: "body.version; 409 requires re-read", ReadBack: "/api/v1/alerting/contact-points/{uid}", Description: "Retain masked credentials by omitting secret updates. Explicit test/delivery operations send external notifications."},
		{ID: "reporting.policy", Method: "PUT", Path: "/api/v1/reporting-sources/policy", MinimumRole: "Admin", Effect: "configure", Concurrency: "body.version; 409 requires re-read", ReadBack: "/api/v1/reporting-sources", Description: "Block/allow/reset canonical source IPs. Metric labels never authorize a source; allowlist-only can reject currently reporting servers."},
		{ID: "business.instances", Method: "GET", Path: "/api/v1/business/instances", MinimumRole: "Viewer", Effect: "read", Concurrency: "cursor pagination", Description: "Discover actual accepted identities, preserving serverId=0 and the full environment/channel/service/role/instance/implementation scope. Optional template_id restricts pages to authorized native-dashboard inherit bindings; q, limit and cursor remain available."},
		{ID: "business.resources", Method: "GET", Path: "/api/v1/business/instances/{id}/dashboard", MinimumRole: "Viewer", Effect: "read", Concurrency: "none", Description: "Resolve business panels, standard process CPU/RSS/uptime and the explicitly associated physical host. Safe resource projections contain no SSH credentials. Existing template permissions still apply."},
		{ID: "business.host-options", Method: "GET", Path: "/api/v1/business/host-options", MinimumRole: "Admin", Effect: "read", Concurrency: "none", Description: "List candidate managed-host and node-exporter collector names/IDs for an explicit physical-host association. Never infer associations from a NAT peer or reported IP label."},
		{ID: "business.host-binding", Method: "PUT", Path: "/api/v1/business/instances/{id}/host", MinimumRole: "Admin", Effect: "configure", Concurrency: "body.version; 409 requires re-read", ReadBack: "/api/v1/business/instances/{id}/dashboard", Description: "Associate a managed host or node-exporter collector without changing business templates; empty kind/id clears the association.", Schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"version", "kind", "id"}, "properties": map[string]any{"version": map[string]any{"type": "integer", "minimum": 0}, "kind": map[string]any{"enum": []string{"", "managed_host", "collector"}}, "id": map[string]any{"type": "string", "maxLength": 128}}}},
		{ID: "business.ranking", Method: "GET", Path: "/api/v1/business/instances/{id}/data", MinimumRole: "Viewer", Effect: "read", Concurrency: "cursor freezes instance/kind/window/sort/ranked/evaluation time; changing a scope requires a fresh first page", Description: "SQL and player-region results: up to five sort=column:asc,column:desc rules, numeric missing-last ordering before pagination. SQL ranked=1 excludes unavailable latency/zero calls before Top10; limit=10, default window=1h. SQL keys: table,operation,avg_ms_window,calls_window,failures_window. Region keys: country,province,city,value."},
		{ID: "business.binding", Method: "PUT", Path: "/api/v1/business/instances/{id}/dashboard", MinimumRole: "Editor", Effect: "configure", Concurrency: "body.version; 409 requires re-read", ReadBack: "/api/v1/business/instances/{id}/dashboard", Description: "Choose default, live template inheritance or independent native dashboard. Binding never grants dashboard/library ACLs.", Schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"version", "mode", "dashboard_id"}, "properties": map[string]any{"version": map[string]any{"type": "integer", "minimum": 0}, "mode": map[string]any{"enum": []string{"default", "inherit", "independent"}}, "dashboard_id": map[string]any{"type": "string"}}}},
		{ID: "dashboards.panels", Method: "PATCH", Path: "/api/v1/dashboards/{id}/panels", MinimumRole: "Editor", Effect: "configure", Concurrency: "If-Match: dashboard updated_at; missing 428, stale 409", ReadBack: "/api/v1/dashboards", Description: "Edit native panels without replacing opaque queries. Optional panel.thresholds={warning,critical} requires finite critical>warning; null clears thresholds. Optional panel.display_unit is empty or binBps (only with panel.unit=bytes): omission retains a custom display unit; explicit empty restores the base-unit mapping. Uptime uses visualization=stat and unit=uptime. Native types include barchart, piechart, state-timeline and status-history. State panels accept optional state_mapping=availability (0 unavailable/red, 1 available/green); empty clears mappings, omission preserves them."},
	}
}

func (s *Server) agentRoutes(api routeRegistrar, manifest *[]ProtectedRoute) {
	api.HandleFunc("GET /api/v1/agent/capabilities", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": "1", "product": "metrics233-server", "version": BuildVersion,
			"authentication": map[string]any{
				"status": "GET /api/auth/status", "account_login": "POST /login", "account_login_fields": []string{"user", "password"},
				"session": "HttpOnly session cookie; use a cookie jar, HTTPS and the configured same origin", "logout": "POST /api/auth/logout",
				"reporting":     "Separate ingest-only Bearer token; never usable for reads, administration, login or discovery",
				"account_basic": "Local accounts also support HTTP Basic over HTTPS; password failure limits apply",
			},
			"operations": nativeAgentOperations(), "protected_route_inventory": *manifest,
			"inventory_scope":    "Server route registrations; account middleware, Prometheus catch-all children and plugin-specific schemas are separate contracts",
			"workflow":           []string{"discover/authenticate", "read current state and identity", "preview without delivery", "save with supported concurrency precondition", "read back and verify real runtime", "verify external delivery separately when authorized"},
			"safety":             []string{"Never blindly retry uncertain mutations or overwrite a 409 conflict", "Ingest labels and business scope are not authorization", "Missing metrics are not zero; preserve no-data/error semantics", "External delivery ACK is separate from queue acceptance", "Template execution is in-process with step/time/output budgets"},
			"managed_host_agent": map[string]any{"routes": []string{"GET /agent/v1/hosts/{id}/configuration", "POST /agent/v1/hosts/{id}/heartbeat"}, "auth": "independent per-host Bearer credential; no browser cookies/origins, SSH, queries, management or cross-host access", "limits": map[string]int{"inflight_requests": 8, "samples_per_report": 5000, "request_mib": 4}, "configuration": "poll every at most 30 seconds; collector interval remains independent; paused hosts receive no collector secrets", "scope": "single collector per managed host record; no arbitrary shell/job orchestration"},
		})
	})
}
