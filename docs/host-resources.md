# Native host resources

The outbound collector can read local CPU, memory, uptime and writable local disks alongside its database target.
It uses the same dedicated process, per-host credential and collection interval. No exporter port is opened.
`collect_host` defaults to enabled for managed hosts and can be disabled independently in the Admin form.
Existing older agents continue reporting their database; host resources require an updated binary.

CPU is the busy fraction between consecutive OS counter observations; Linux guest time is not counted twice and
I/O wait is idle. The initial observation establishes a baseline. Counter resets or failed reads produce missing
CPU usage, never a healthy-looking zero. Memory is `(total-available)/total`; reclaimable pages count as available.
Disk capacity is `(total-available)/total` for writable local volumes, including space consumed by other APFS
container users. Read-only snapshots, helper volumes and remote/pseudo filesystems are excluded. At most 32 volumes
are collected; shared container capacities must not be summed. Genuine zero and missing values stay distinct.

`host_cpu_usage_percent`, `host_memory_used_percent`, `host_disk_used_percent`, capacity byte metrics,
`host_cpu_logical_cores`, `host_uptime_seconds` and per-resource `host_resource_collection_success` are reserved
observations. The server validates finite ranges, pins host/job/configured identity labels and derives a bounded
latest summary from accepted samples. It does not trust client-supplied summary JSON. Failed resources disappear
from the new summary. Paused heartbeats preserve the old observation timestamp; the UI displays a stale state.

Admin `POST /api/v1/managed-hosts/{id}/dashboard` accepts `{"kind":"host","language":"zh"}` for an eight-panel
host board. The default kind remains `mysql`, preserving existing consumers. Every resource query pins the actual
managed-host ID; existing customized dashboards are returned unchanged. The host page shows the three native SVG
instruments, last 60-minute trends, all collected writable disks and links to host/MySQL boards. Low-frequency
configuration and destructive operations are collapsed; revoke still requires explicit confirmation.

`model.HostAlertDefaults` previews six versioned graph rules. Raw measurement A is independent of threshold B:
CPU/disk warning >85%, memory >90%, critical >95%; warning needs 10 minutes (disk 15), critical 5. Recovery requires
5 continuous minutes below the lower threshold (CPU/disk warning 80%, memory warning 85%, critical 90%). All use
3600-second notification cooldown and KeepLast for unavailable observations. Missing readings do not resolve an
incident. Caller-selected routes/inhibition and outbox delivery remain separate configuration/acceptance gates.
For CD-enabled groups, recovery coalesces the configured group wait, capped at 30 seconds. This combines resource
evaluations that finish a few seconds apart; explicit zero wait retains immediate recovery. The previous firing's
cooldown deadline never postpones a valid recovery.
Persist through the normal alert API and preserve custom rules; the preview factory does not mutate settings.

Native gauge rails honor the panel's configured threshold steps, including percentage steps and reversed risk
colors. Needles are bounded without modifying the displayed measurement. Missing/paused readings have no needle;
reduced-motion preferences disable needle transitions. Host-page reference rails are explicitly identified as
references when a supported rule is unavailable. For standard per-host rules, the host page reads the actual
threshold/unload graph and persisted pending/recovery/cooldown durations; it does not trust a stale descriptive
annotation. Paused, acceptance, foreign-host and unsupported custom conditions do not become a fabricated policy.
Cards state idle processing capacity, available memory and remaining disk space. Missing/stale samples never
become zero capacity. Friendly disk names keep raw mount/device/filesystem details in an optional disclosure.

Feishu resource cards use compact plain-text field pairs for host/current observation, condition, duration,
start/recovery time and cooldown. A `value_ref` annotation selects a raw graph ref without exposing boolean helper
values. Custom templates remain single-pass data. Untrusted mentions/URLs remain inert, the card stays below
20 KiB, and actions target only the configured HTTPS origin with a validated dashboard UID or the alert page.
Notification tests remain clearly identified and do not claim a resource failure.

Source/unit, cross-build, rendered-browser, private release and actual OS/Feishu acceptance are distinct layers.
