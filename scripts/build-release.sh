#!/usr/bin/env sh
# Build self-contained release binaries for the common desktop/server targets.
# Requires Go, Node.js and npm. Docker is not involved.
set -eu
root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
targets="$(printf '%s' "${TARGETS:-windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64}" | tr ' ' ',')"
if [ -n "${OUTPUT_DIRECTORY:-}" ]; then
  exec node "$root/scripts/build-release.mjs" --version "${VERSION:-0.1.1}" --targets "$targets" --output "$OUTPUT_DIRECTORY"
fi
exec node "$root/scripts/build-release.mjs" --version "${VERSION:-0.1.1}" --targets "$targets"
