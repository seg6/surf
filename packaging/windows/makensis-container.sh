#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
image="${SURF_BUILDENV_IMAGE:-surf-buildenv}"

exec docker run --rm --entrypoint makensis \
  --volume "$repo_root:$repo_root" \
  --workdir "$repo_root" \
  "$image" "$@"
