#!/usr/bin/env bash
# Only record.mjs --netflix selects this launcher. Normal Surf launches keep GPU.
set -euo pipefail
args=()
for arg in "$@"; do
    if [[ "$arg" != --enable-gpu ]]; then args+=("$arg"); fi
done
exec "${SURF_DEMO_CHROME:-/opt/google/chrome/chrome}" --disable-gpu "${args[@]}"
