# Local accounts and offline first administrator

Use `METRICSPANEL_AUTH=local` for account/password backoffice access. Clear `METRICSPANEL_TOKEN`: when present,
it remains a compatibility Admin credential even in local mode. Reporting applications use only
`METRICSPANEL_INGEST_TOKEN`; it cannot read data, sign in, create accounts or change policy. See
[reporting access](reporting-access.md) for the exact five POST paths.

## Initialize before public exposure

Do not expose an uninitialized local instance to the public internet. On Linux, an operator with root access can
initialize the separate identity catalog without starting the HTTP server or opening the metric/control database:

```sh
sudo ./metricspanel bootstrap-admin --identity-db /var/lib/metricspanel/identity.db --login admin
```

The command prompts `Password:` and `Confirm password:` with terminal echo disabled. Passwords must contain
8–1024 bytes, matching the existing account policy. Optional `--name` and `--email` are nonsecret metadata. There is no password flag or password
environment-variable input for this command. The user enters the password directly in their SSH terminal;
operators must not request it in chat or record terminal input.

The identity path must be explicit, absolute and normalized. Its existing parent must have no group/other access
(normally `0700`); the catalog and existing SQLite WAL/SHM sidecars must be regular files with no group/other
access (normally `0600`). Parent paths and catalog/sidecar symlinks are rejected. Protect the containing path
from untrusted writers. The command does not create parent directories. If a privileged operator adjusts directory
permissions, preserve the runtime service owner. Newly created files belong to the invoking user; give the service
owner access before starting it, and retain restrictive permissions.

Success prints only `initialized`, `user_id`, `organization_id` and `role` JSON fields. The first account is Admin
in organization 1. Any existing account causes `bootstrap_already_initialized`; the command never overwrites
accounts or resets a password. Keep normal backup/recovery procedures for later credential recovery.

For protected automation, `--file -` accepts one JSON object from stdin containing `login`, `password` and optional
`name`/`email`, at most 16 KiB. Unknown fields, extra JSON objects and file paths are rejected. Never place a real
password in command arguments, shell history, source, logs or a checked-in fixture. Interactive `--login` and JSON
input are mutually exclusive. The command has a 30-second catalog-operation deadline and returns fixed diagnostic
codes without echoing input or database errors.

## Switch an existing deployment

1. Complete pending administration and back up the data directory, including configuration, `identity.db`, SQLite
   sidecars and `secrets.key`, while the service is stopped for a consistent snapshot.
2. Prepare the identity catalog with the offline command before enabling public local-account service. Retain the
   existing protected ingest token; set `METRICSPANEL_AUTH=local` and remove `METRICSPANEL_TOKEN` and old
   `METRICSPANEL_ADMIN_*` bootstrap variables from the private service environment.
3. Start behind the existing HTTPS proxy with a loopback listener, the same identity path and correct HTTPS
   `--root-url`. Sign in through the account form and verify session access. Passwords and session tokens stay private.

Readiness can check public `/api/health` and `/api/auth/status` (`mode=local`, `setupRequired=false`), then verify
anonymous queries return 401 and ingest-token queries return 403. It must not retain an Admin password or a legacy
management bearer solely for health checks. Session role checks, CSRF/origin policy and source-IP restrictions remain
in force. If the catalog is empty, the existing development setup-token flow remains available; public production
must keep that state inaccessible until offline initialization finishes.

## Verification scope

Linux fixtures exercise protected paths, real catalog creation/authentication, repeat rejection and secret-free
metadata. Real local TLS tests cover login/cookie/logout, all five ingest protocols, source-policy rejection,
duplicate credentials and custom-authorizer fail-closed behavior. These checks do not constitute a production
account switch or a user-operated password initialization.
