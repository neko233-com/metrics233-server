# Reporting sources and access

The **Reporting sources / 上报来源** page lists native JSON, Prometheus remote-write and OTLP/HTTP senders that actually wrote samples. Scrape targets remain under Collectors; internal self-metrics are not external reporters. Source timestamps use server receipt time, not client sample timestamps. A manual policy entry with no observation is shown as not yet reported, never as an online service.

The security identity is a canonical client IP, shared across every ingest protocol and alias. IPv4-mapped IPv6 normalizes to IPv4. `service_name`, `service` and `job` are bounded display hints only: changing a label cannot evade a blocked IP. Machines behind the same NAT share a policy. A rotating address requires updating its policy; IP policy is not proof of a person's identity.

## Authentication and proxy trust

Keep workspace management credentials private. Use the separately configured ingest-only token when granting write access; the Go SDK already accepts a fixed credential with `WithToken`. A source allow entry never bypasses authentication or local roles.

Set `METRICSPANEL_TRUSTED_PROXIES` only to the actual reverse-proxy CIDRs. The server ignores forwarding headers from every other socket peer. For a trusted peer, it selects the rightmost untrusted IP across all `X-Forwarded-For` fields, using the same resolver as login rate limits. Never trust a public subnet just to make the source list look correct. Invalid source addresses fail before metric writes.

## One-click policy

- **Block** rejects new writes from the entire IP before decoding or storing metric bodies. Already admitted in-flight requests may finish.
- **Allow / unblock and allow** permits the IP, while keeping the token requirement.
- **Remove from allowlist** removes the explicit decision. In allowlist-only mode the IP becomes denied; in token-plus-blocklist mode it follows the default allow decision.
- **Allowlist only** rejects every IP without an explicit allow entry. Initially disabled to preserve existing authorized senders; add the intended IPs before enabling it.

Only Admin can change policy. Viewer and Editor can read the page. Policy changes and observations are stored in the owning organization's SQLite control catalog and survive restart. Access-policy lookup failure rejects writes; an auxiliary observation failure after a successful metric write does not turn that write into a retryable error.

## API and bounds

`GET /api/v1/reporting-sources` returns `version`, `allowlist_only`, capacity limits and `sources`. Each source includes `address`, `disposition`, `allowed`, `first_seen`, `last_seen`, `accepted_batches`, `accepted_samples`, `protocols` and `last_service`. Counts describe accepted batches, including the accepted portion of a partially written remote-write batch. Invalid credentials and rejected bodies do not register a reporter.

`PUT /api/v1/reporting-sources/policy` requires the current `version` and explicit `allowlist_only`. Add `address` and `action` together (`allow`, `block`, or `reset`) to change one IP. A stale version returns HTTP 409 without mutation. The UI requires refresh rather than silently replaying a stale security change.

The observation ledger retains the 1,024 most recently observed IPs. Its eviction never removes policy. The separate policy registry holds at most 4,096 explicit entries; exceeding capacity rejects a new entry atomically and leaves existing decisions intact. No scan of metric series is needed to render the page. The UI paginates 50 rows, refreshes once per minute while mounted, and also offers manual refresh.
