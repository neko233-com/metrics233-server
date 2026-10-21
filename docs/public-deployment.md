# Public deployment

This page is the operational checklist for exposing MetricsPanel233 to the internet without leaking metrics,
credentials or session data. It assumes one Linux host and one service instance. Horizontal scaling and multi-tenant
isolation are outside the current product scope.

## Threat model and defaults

- The service is single-workspace; roles are global to that workspace.
- Non-loopback listeners always require `METRICSPANEL_TOKEN` with at least 16 characters.
- Configure a distinct environment-only `METRICSPANEL_INGEST_TOKEN` for external reporting agents in token or local mode.
  It requires 32–8192 printable ASCII characters without whitespace; token mode also requires a separate 16+ character management token;
  it cannot read metrics, manage contact points or unblock its IP. Never give agents the management token.
  See [reporting access](reporting-access.md).
- Production backoffice uses local accounts, HttpOnly sessions and Admin/Editor/Viewer roles. Clear
  `METRICSPANEL_TOKEN`, which otherwise remains a legacy Admin credential, and initialize the account offline
  before public exposure. See [local accounts](local-auth.md).
- The built-in HTTP server does not terminate TLS. Put a reverse proxy in front of it.
- `/api/health` is public and returns only `commit`, `database` and `version`. `/api/v1/health` and all metric,
  dashboard, plugin and user APIs require authentication.
- Setup is available only while no account exists. The one-time token is printed to stdout; it is not stored.

## 1. Bind to loopback and terminate TLS at the proxy

```sh
METRICSPANEL_AUTH=local \
METRICSPANEL_TRUSTED_PROXIES=127.0.0.1/32 \
./metricspanel serve \
  --listen 127.0.0.1:7333 \
  --db /var/lib/metricspanel/control.db \
  --identity-db /var/lib/metricspanel/identity.db \
  --root-url https://metrics.example.com
```

`--root-url` must be the real public HTTPS origin. It controls `Secure` session cookies, HSTS and absolute links.
When explicitly configured, its origin is also authoritative for local-account login, setup, logout and API
origin checks. Preserve the public `Host` header on the HTTP upstream. `X-Forwarded-Proto` does not establish
origin trust; listing trusted proxies enables client-IP interpretation only.
Keep `METRICSPANEL_INGEST_TOKEN` in a protected service EnvironmentFile; never pass secrets as CLI arguments.
The service account needs read/write access only to the data directory.

Before starting the public service, create the first administrator offline from a user-operated root terminal:

```sh
sudo ./metricspanel bootstrap-admin --identity-db /var/lib/metricspanel/identity.db --login admin
```

Password and confirmation are read without echo. Prepare the existing data directory as mode `0700`, then ensure
the service owner can access the resulting `0600` catalog. The command refuses an existing account. Keep public
access disabled until initialization succeeds; the printed setup token is a development/recovery boundary, not a
public account-registration feature. Remove old `METRICSPANEL_ADMIN_*` bootstrap variables.

## 2. Reverse proxy examples

### Caddy

```caddyfile
metrics.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:7333
}
```

Caddy obtains and renews TLS certificates automatically and forwards `X-Forwarded-For` and `X-Forwarded-Proto`.

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name metrics.example.com;

    ssl_certificate     /etc/letsencrypt/live/metrics.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/metrics.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:7333;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

`X-Forwarded-For` is only trusted for rate limiting when the direct peer matches
`METRICSPANEL_TRUSTED_PROXIES` (comma-separated CIDRs). Do not trust a public CIDR. If you run several proxies, list
each proxy address.

## 3. Firewall and network

- Expose only `443/tcp` publicly. Keep `7333` bound to `127.0.0.1` or a private interface.
- Do not expose ClickHouse, SQLite files, the identity database or `secrets.key`.
- Restrict SSH and administrative access separately.
- Use a host firewall and, when available, a cloud security group.

## 4. Sessions, CSRF and headers

In local-account mode:

- Session cookies are `HttpOnly`, `SameSite=Lax`, `Secure` under an HTTPS `--root-url`, and rotate every 24 hours.
- Login and account-password Basic authentication share a client-IP budget of 8 failed passwords per 5 minutes.
  Successful password authentication clears prior failures and does not consume this budget. Setup has its own
  client-IP attempt budget. Bearer and service-token authentication do not consume the password budget.
- Browser mutations require a same-origin `Origin`/`Sec-Fetch-Site`; cross-origin requests are rejected.
- API responses set `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`,
  `Cross-Origin-Opener-Policy: same-origin`, `Cross-Origin-Resource-Policy: same-origin`,
  `Permissions-Policy` and a restrictive CSP. HSTS is added when TLS is detected or `--root-url` is HTTPS.
- `METRICSPANEL_TOKEN`, if deliberately set, retains compatibility Admin access; account-only production clears it.
  The separate ingest token permits only five exact POST paths. Use the account session for the backoffice.

## 5. Secrets and data leakage checklist

- Datasource and application secrets are encrypted with AES-256-GCM. Back up the control database, `secrets.key`,
  `identity.db` and the plugin directory together.
- API responses return `secure_fields` presence flags only; omitted credentials are retained, empty strings clear them.
- Passwords, session tokens, bearer tokens and setup tokens must never appear in logs. Login failures log only the
  client IP and HTTP status.
- `/api/health` contains no storage paths, counters or credentials.
- Do not enable `--allow-unsigned-plugin` in production.
- Keep the Go binary, reverse proxy and OS patched. Re-run `go test`, the headless suite and the container smoke after
  upgrades.

## 6. Backups and recovery

```sh
systemctl stop metricspanel
tar -C /var/lib/metricspanel -czf /secure/backups/metricspanel-$(date +%F).tgz .
systemctl start metricspanel
```

Back up `control.db`, `identity.db`, `secrets.key` and `plugins/`. Test a restore on a separate host before relying on
it. Metric retention and dashboard history are independent: restoring configuration does not roll back samples.

## 7. Grafana ecosystem access

Grafana-compatible clients can use:

- `GET /api/health` without credentials for load balancers.
- `GET /api/login/ping`, `GET /api/user`, `GET /api/user/orgs` and `POST /api/logout` for Grafana-compatible clients.
- Basic auth with a local Admin/Editor account, or `Authorization: Bearer <workspace token>` for automation.
- Prometheus datasource endpoints under `/prometheus/`.
- Dashboard, folder, library, datasource, annotation, alert and plugin routes described in the main README.

Roles are enforced before Grafana-compatible handlers run. Unknown mutations fail closed to Admin.

## 8. Container deployment

Run the container with the loopback port published only to the host proxy:

```sh
docker run -d --name metricspanel \
  --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  --pids-limit=128 --memory=256m --cpus=1 \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  -p 127.0.0.1:7333:7333 \
  -e METRICSPANEL_AUTH=local \
  -e METRICSPANEL_ROOT_URL=https://metrics.example.com \
  -v metricspanel-data:/data \
  metricspanel233:local
```

Initialize the mounted identity catalog offline with protected stdin before starting this local-account container.
Terminate TLS in front of it. Do not publish `0.0.0.0:7333` directly to the internet.

Replace the example public origin, and configure `METRICSPANEL_TRUSTED_PROXIES` with only the proxy peer CIDR as
observed from inside the container. A host proxy may appear as a Docker bridge gateway, rather than `127.0.0.1`.
The resource limits above are a verified starting profile for the bounded test workload; measure and adjust them
for actual series count, ingestion and query concurrency.

For local verification use `scripts/dockerfile_test.ps1 -Repeat 2 -IncludeDatabaseIntegration`; it builds the formal
runtime and removes only its own resources. See [Docker validation](docker-validation.md) for checks and boundaries.
