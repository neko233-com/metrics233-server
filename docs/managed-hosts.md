# Managed hosts and outbound collectors

Host management is an Admin-only SSH installer and configuration controller for Linux/macOS amd64/arm64.
It installs a small dedicated collector, not another monitoring server. Protocol readers depend on a narrow
`collector.TargetStore` scheduling interface and the standard-library wire types in `internal/telemetry`;
the collector binary does not link the control-plane store, query engine or web UI.

Updated agents also collect local host resources in the same process. See [native host resources](host-resources.md)
for CPU/memory/disk definitions, the separate host dashboard, missing/stale values and sustained alert defaults.

## Workflow

1. Add a host and a collector in **Metadata → Host management**. The collector URL is resolved on the target.
2. Verify the SSH SHA256 fingerprint against a trusted record. Fingerprint inspection stops before authentication;
   it never automatically trusts a newly observed key. Save the verified pin before installing.
3. Install using the server when it can reach SSH. A private-network target needs the same installer executed
   inside its network: `metricspanel hosts deploy-local --id ID --host-version VERSION --collector-artifacts /absolute/collectors`.
   Use a protected account session/credential configuration; never pass passwords or tokens as command arguments.
4. Verify the outbound heartbeat, applied configuration version and actual samples separately from SSH installation.
   **Open dashboard** creates a MySQL dashboard once; later calls preserve all custom panel edits.

The server cannot reach a private LAN merely because its IP was entered. The local installer retrieves an explicit
Admin deployment plan in memory and opens the SSH connection from that LAN. This does not open a public database
or SSH port. After installation, settings and pause/resume flow through outbound HTTPS without another SSH session.
This first implementation manages one collector per host record. It is not arbitrary remote command execution,
a general service orchestrator, an automatic LAN relay, or an unverified claim of whole-cluster deployment.

## Credentials and permissions

- Account sessions, generic ingest tokens and independent per-host agent tokens are distinct. A host token reads
  only its own collector configuration and submits its own heartbeat/samples; it cannot query, configure SSH,
  access another host, or authenticate a browser session. Cookies and browser origins are rejected by agent routes.
- SSH/database credentials reuse AES-GCM encryption with organization/resource-bound AAD. List responses return
  presence flags only; omitted edits retain secrets. The explicit Admin `POST .../deployment-plan` is sensitive:
  consume it in memory with the LAN installer, never print it or save it in a report.
- Host writes and deployment requests require the current `version`; a stale request returns 409. At most one
  server installation runs per host and two globally. Agent requests have eight slots, a 4 MiB body and 5,000-sample
  budget. Unknown agent routes return an error rather than a successful SPA document.
- Revoke rotates the per-host credential and pauses the collector atomically. The old credential immediately
  returns 401; re-enable requires deployment with the replacement bootstrap. Pause alone keeps the credential
  and can be resumed through the normal configuration flow.
- Agent report labels cannot impersonate another managed host or job/instance. Reserved business identity labels
  come only from the saved Admin configuration. Native database reporters are also restricted to their metric namespace.
  Reporting-source allow/block policy still applies to accepted metric batches.
- Only fixed installers and server-owned artifacts with matching SHA256 manifests are accepted. SSH values and
  secrets never become shell command text. Sudo credentials are explicitly consumed before the shell reads its
  installer, including cached/NOPASSWD cases. Both sudo calls keep the same parent shell so macOS no-TTY tickets
  remain valid with zsh. Linux systemd/macOS LaunchDaemon services run as locked dedicated collector users; the protected
  bootstrap contains only server origin, host identity and that host's credential. Database secrets arrive in memory.
- The installer preserves the previous service/configuration and restores them when activation fails. Old release
  binaries remain for recovery. A service-manager activation receipt is not an external heartbeat or database proof.
  First-connection loss and operator actions after successful activation still need explicit runtime inspection.

## MySQL

The native reader executes only `SHOW GLOBAL STATUS` and a fixed allowlist from `SHOW GLOBAL VARIABLES`.
It collects capacity, connections, statement/query counters, InnoDB, lock waits, traffic and slow-query counters.
It does not read application rows, capture SQL text, change MySQL settings or grant users. Prefer a dedicated
least-privilege account. `tls_mode=require` encrypts without certificate verification; `verify-full` validates
the certificate and hostname. These choices stay explicit rather than silently downgrading a failed connection.

The 18-panel native MySQL dashboard fixes its host identity in every query. Rates use five-minute counter windows
within the chosen dashboard range (60 minutes by default). MySQL restarts reset counters. Connection utilization
uses real `max_connections`; a full buffer pool remains green because cache occupancy alone is not an incident.
Explicit COMMIT/ROLLBACK counters are not a count of all autocommit transactions. Lock-wait time is cumulative
waiting seconds per second, not per-query latency. Game Entity slow-SQL Top10 remains a separate application view.

Polling configuration never overlaps another cycle and occurs at most 30 seconds apart. Longer collection intervals
are honored independently. Pause clears collector secrets from the next configuration response. Transport errors
use bounded backoff and fixed diagnostics; uncertain writes are not replayed. Samples and catalog heartbeat writes
are separate acceptance layers: accepted measurements can survive a later failed heartbeat update.

## Agent API

Account/Admin routes are listed by `GET /api/v1/agent/capabilities`:

- `GET/POST /api/v1/managed-hosts`; `PUT /api/v1/managed-hosts/{id}`
- `POST /api/v1/managed-hosts/{id}/fingerprint`, `/deploy`, `/deployment-plan`, `/dashboard`, `/revoke`
- `GET /api/v1/managed-hosts/{id}/events`
- `GET /api/v1/managed-host-artifacts/{linux|darwin}-{amd64|arm64}`

Dashboard creation optionally accepts `{"language":"zh"}` or `{"language":"en"}` for its initial saved
legends and value mappings. The UI uses the selected language. Repeating this request preserves existing
customizations rather than rewriting the dashboard on a locale change.

Independent agent routes are `GET /agent/v1/hosts/{id}/configuration` and
`POST /agent/v1/hosts/{id}/heartbeat`. Only HTTPS (or literal loopback HTTP for isolated tests) is accepted by the
collector. Redirects and inherited HTTP proxies are disabled. The normal lean Docker image does not bundle optional
SSH installation artifacts; provide their read-only directory beside the server executable as `collectors/`.
