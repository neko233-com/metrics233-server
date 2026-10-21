# Authentication and security readiness review

Date: 2026-10-21. This is a focused source review with executable regression evidence for the single-node
authentication boundary. It is not a repository-wide security scan or a public deployment certificate.

## Scope and result

Reviewed `internal/localauth`, identity password/session/catalog code, HTTP authentication/session composition,
the CLI's authentication configuration, masked collector/datasource credentials and the documented HTTPS proxy
contract. The review found and fixed the boundary failures below. Existing identity/session protections remain
covered by their package tests. No identity schema, metric storage or ingest protocol was changed for this review.

| Boundary | Reproduced before the fix | Result after the fix |
| --- | --- | --- |
| Namespace `/apis/` authentication | Real inhibition-rule GET returned 200 anonymously in local mode without a workspace token. With a workspace token configured, a valid browser session instead returned 401. | The prefix passes through local authentication and role checks. The real server returns anonymous 401, authenticated Admin/Viewer GET 200 and Viewer mutation 403, with and without a workspace token. |
| Basic account-password failures | Eight wrong Basic passwords did not block a subsequent correct Basic login; form throttling could also be bypassed with Basic. | Basic account passwords and form login share the failure budget. Eight failures cause 429 with Retry-After; 24 simultaneous bad-password calls produce exactly eight 401 and sixteen 429 responses. |
| Successful automation | Counting every Basic request toward an eight-attempt allowance would break valid CLI/API traffic. | Twelve successive valid Basic calls succeed. Success clears prior failures; workspace Bearer and service-token authentication stay outside the password budget. |
| Canonical-path redirects | Seven failed logins, a Go ServeMux 307 canonical redirect, and another failure cleared the budget and allowed a correct password. | Only successful 2xx login responses clear failures. A redirect cannot clear the budget. |
| Repeated forwarded header fields | Trusted proxy plus multiple X-Forwarded-For fields selected an earlier attacker-controlled address instead of the rightmost untrusted hop. | Header fields and comma-separated hops are walked from right to left. Rotating an earlier field does not rotate the limiter key. Untrusted socket peers cannot supply forwarding metadata. |
| Limiter memory | More than 4096 simultaneously active client keys were accepted. | At most 4096 active entries; expired entries are reclaimed. Unknown clients fail closed while full, and existing clients retain their remaining quota. |
| HTTPS termination | A legitimate HTTPS public Origin reached an HTTP proxy upstream and failed login with 403 even with HTTPS root-url configured. | Explicit root-url establishes a single trusted origin shared by setup, login, logout and authenticated APIs. Forwarded protocol headers cannot establish trust. Foreign origins remain 403. |

The setup-token getter and invalidation now share a mutex; a concurrent-read/first-account test exercises this
lifecycle. This is a race-prevention fix, with no claim that the earlier string race established account takeover.

Password attempt checks and their failure updates are serialized before Argon2, preventing concurrent requests
from all passing an empty failure counter. Queue waits honor request cancellation. Login reads at most 64 KiB
before entering that gate, so a slow body does not hold the password-processing slot. Malformed and cross-origin
requests do not consume the failed-password budget. The limit is per resolved client IP and is intentionally
shared by users behind the same NAT; it is a bounded appliance control, not a distributed abuse-prevention system.

## Existing protections reviewed

- Passwords use Argon2id v19 with 64 MiB, three passes, four lanes, a random 16-byte salt and a 32-byte result.
  Identity's process-wide password worker bounds simultaneous KDF memory across catalogs. Queue cancellation,
  unknown-account work and fixed hash-policy validation are covered by `TestPasswordHashPolicyAndCancellation`.
- Session/service credentials contain 256 random bits and are stored as SHA-256 digests. Public views and
  authentication errors do not serialize credentials or their digests. Request-local password proofs are private,
  bound to credential/membership versions and cannot be reused as an independent credential.
- Session rotation retains absolute expiry, checks idle expiry, preserves device identity and rechecks current
  membership, disabled state and revocation. Logout revokes current/previous credentials. Existing session,
  metadata and password-proof regressions passed; rotation is not inferred merely from a Set-Cookie header.
- Local production sessions are HttpOnly/SameSite=Lax and Secure with an HTTPS root-url. Missing Origin remains
  allowed for CLI/non-browser clients; present malformed, foreign, duplicate or null Origin and cross-site fetch
  metadata are rejected before credential/session metadata is updated. This is not an additional CSRF token scheme.
- The identity catalog refuses to open a preexisting business database; the regression checks that its original
  table survives. Control data and identity remain distinct physical catalogs. Backups must include both, the
  matching secrets.key and plugins. The existing Docker recovery test restores a stopped-volume SHA-256-checked
  archive, then checks sessions/password login, metrics, decryption and missing-key refusal in a temporary copy.
- Collector/datasource secret APIs expose presence flags instead of plaintext. Existing encryption/masking tests
  and runtime recovery checks cover this behavior. This review did not expand the secret schema or dump data files.
- `/api/health` remains public and minimal. Detailed health, `/metrics`, `/api/`, `/apis/`, Prometheus, OTLP and
  protected plugin surfaces share their existing authenticated boundary. Unknown mutations require Admin.
- `--auth=token` remains the compatibility default. Identity creation and PublicOrigin configuration are inside
  the local-mode branch. The production test additionally configures an unusable identity path in token mode,
  checks that no identity catalog is created and checks a Bearer-authenticated namespace request.

## Executed checks

After the production-source patch:

```powershell
go test ./internal/localauth ./internal/instance ./internal/identity ./cmd/metricspanel -count=1
go vet ./internal/localauth ./internal/instance ./internal/identity ./cmd/metricspanel
go test -tags=integration ./tests/integration -run '^$' -count=1
```

All passed. The four-package run reported 4.034 s / 14.438 s / 9.493 s / 4.328 s respectively. After adding the
real namespace Viewer read/write cases, `go test ./internal/localauth -count=1` passed again in 2.519 s, and the
integration package compiled again. The empty integration selection proves compilation only; it is not a Docker
or HTTP-runtime pass. Windows has CGO disabled and no GCC, so these checks do not claim a Windows race-detector run.

The new production-runtime `https-proxy-origin` case uses the actual release server on an HTTP upstream with
canonical public Host/HTTPS Origin. It checks setup, Secure cookies, password login, API and namespace access,
foreign-origin refusal and logout revocation, including deliberately misleading X-Forwarded-Proto. The complete
post-patch token/local/browser/restore/HTTPS suite passed twice as part of the 535.733-second unified run; both
real SQL browser cycles also passed three tests with no skips or retries. Full Linux race initially failed its
ten-minute package timeout while the five-million-row storage test was pruning; the explicit twenty-minute
rerun then passed the full vet/race command in an 859.4-second validation step, with `internal/store` taking
743.373 seconds. The temporary image was removed. Detailed resource measurements, failed-attempt boundaries,
the 649-file source-equivalence audit and pre-migration source scope are recorded in
[Docker validation](docker-validation.md).

## Deployment boundary

Use loopback/private container publication behind HTTPS, an explicit public root-url and only actual proxy CIDRs.
The proxy must preserve the canonical public Host; arbitrary upstream Host rewriting is not a supported contract.
The configured public origin may normalize a default HTTP(S) port at the authentication layer, while the existing
inner server still checks matching Host authority. Use one consistent canonical authority throughout the proxy.
Never expose port 7333 directly to the internet or enable unsigned plugins in production.

No real public TLS proxy, DNS, certificate renewal, cloud firewall or externally reachable deployment was exercised
by this review. The Docker case models the HTTP upstream and does not establish those operational properties.
See [public deployment](public-deployment.md) for the required configuration.

After the module/repository migration, the actual `metrics233-server/` build context was separately rebuilt into
a new formal Linux runtime and passed `scripts/dockerfile_test.ps1 -Repeat 1 -Browser` in 103.085 seconds. The
new-module result again covers local/token authentication, HTTPS-origin HTTP upstream, Secure cookies, roles,
logout, persistence, recovery and Chromium Business monitoring permissions. Its Go package identity is
`github.com/neko233-com/metrics233-server/tests/integration`; no old image was reused. This post-migration check
does not reclassify the earlier full race/database evidence as a new-module run or claim a public TLS deployment.
