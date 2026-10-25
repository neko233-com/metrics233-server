# Release and upgrade checklist

Release preparation is local. A repository push, tag, external upload or remote deployment is a separate action.
The full validation workflow still runs only on `v*` tags or explicit dispatch.

## Build and identity

1. Run relevant Go/HTTP/renderer tests and the real Docker checks. Record OS, toolchain, SQLite/ClickHouse mode,
   workload and test result; do not infer a production environment's health from a compiled archive.
2. Choose a semantic version. For local candidate validation, use a prerelease such as `0.1.1-dev.20261021`.
3. From any directory, run `pwsh -File /path/to/scripts/build-release.ps1 -Version VERSION`, or set `VERSION` and
   run `scripts/build-release.sh`. Both use Node 24 and the same streaming deterministic ZIP writer.
4. The default five targets are Windows amd64, Linux amd64/arm64 and macOS amd64/arm64. PowerShell `-Targets` or
   shell `TARGETS` can select a subset. Unsupported or duplicate targets and unsafe versions fail before building.
5. Output is a new `dist/<version>/` directory. Existing output is refused, with all existing files preserved.
   The source and frontend inputs (including both packaged README languages) are hashed before/after the build; concurrent changes fail the build. A staging
   directory is owned by one run and is the only directory eligible for failure cleanup.
6. Check `SHA256SUMS` against every ZIP and `PROVENANCE.json`. Each ZIP contains the executable, English/Chinese README, license
   and `BUILDINFO.json`; the latter identifies version, target, compiler, Node, source revision, dirty state, input
   hashes and binary SHA-256. The manifest includes uncommitted source inputs; a Git SHA alone does not describe a
   dirty candidate. No credential, local database, plugin directory or `.env` is copied into an archive.
7. Rebuild the same frozen inputs/toolchain into another new output directory and compare archive and binary
   hashes. ZIP entry dates, order and Unix modes are fixed; packaging does not depend on source mtimes. The ZIP writer
   has independent Python `zipfile`/CRC/extraction tests (`node --test scripts/test/*.test.mjs`). This is a same-input
   reproducibility contract, not an untested claim across arbitrary toolchain versions or operating systems.

## Runtime acceptance

- Extract the archive and run `version`, then start it with a fresh isolated SQLite data directory. Confirm public
  minimal health, auth policy, business push/query and actual browser rendering.
- Run the Linux amd64 release under the confined production profile. Other target archives are cross-compiled;
  claim native macOS/arm64/Windows acceptance only after executing the matching archive on that platform.
- Run `scripts/dockerfile_test.ps1 -Repeat 2 -Browser -IncludeDatabaseIntegration` for real image, role, collection,
  SQL browser and durable restore evidence. A `validation` Docker target supplies GCC for Linux `vet`/race tests.
- Validate the actual exporter cardinality, sustained query/write load, retention and disk growth on the intended
  production hardware. Local bounded baselines are not universal limits or latency SLOs.

## Upgrade and rollback

1. Keep the existing binary/image and its checksum manifest. Stop writes and take a consistent full backup of
   control database (including any WAL), `identity.db`, `secrets.key`, plugin directory and metric backend data.
2. Restore that backup into a separate empty directory/volume and verify sign-in, metric queries and decryption
   before changing the live installation. Never restore identity or encryption keys from an unrelated instance.
3. Existing ClickHouse Compose users must include `compose.clickhouse.yml`; the default `compose.yml` now selects
   embedded compressed TSDB and local vectors. SQLite still stores control/identity metadata; explicit legacy
   `--storage sqlite` remains available. Switching backends does not migrate historical metric samples. Existing ClickHouse TTL changes remain an
   explicit `ALTER TABLE` operation; a changed CLI retention value does not rewrite old table TTLs.
4. Use the real HTTPS root URL, trust only the actual proxy peer CIDRs, and keep port 7333 bound behind the proxy.
   First-account/password entry remains on the official user-operated setup page or the existing operator workflow.
5. After the approved switch, verify the actual process's version, backend, roles, recent writes, queries and alerts.
   Roll back executable and matching full data snapshot together if a data format changed; do not assume an older
   binary can read a newer migration. New data written after a snapshot requires an explicit recovery decision.
6. Only an authorized release tag or manual CI run establishes release-CI acceptance. Local build/test results,
   uploaded archives and repository pushes must be reported independently from deployment verification.
