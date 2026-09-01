#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
version="${VERSION:?VERSION is required}"
compatibility_version="${COMPATIBILITY_VERSION:?COMPATIBILITY_VERSION is required}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid VERSION: $version" >&2
  exit 1
fi
if [[ ! "$compatibility_version" =~ ^[1-9][0-9]*$ ]]; then
  echo "invalid COMPATIBILITY_VERSION: $compatibility_version" >&2
  exit 1
fi
core_version="${version%%[-+]*}"
IFS=. read -r major minor patch <<< "$core_version"
if (( 10#$minor >= 1000 || 10#$patch >= 1000 )); then
  echo "VERSION minor and patch components must be below 1000: $version" >&2
  exit 1
fi
# Keep the bundle build monotonic without a second hand-edited counter. This
# also remains greater than the fixed build number 64 used through Surf 0.15.4.
bundle_version=$((10#$major * 1000000 + 10#$minor * 1000 + 10#$patch))

sed -e "s/@VERSION@/$version/g" \
    -e "s/@COMPATIBILITY_VERSION@/$compatibility_version/g" \
    "$script_dir/control.in" > "$script_dir/control"
sed -e "s/@VERSION@/$version/g" \
    -e "s/@BUNDLE_VERSION@/$bundle_version/g" \
    -e "s/@COMPATIBILITY_VERSION@/$compatibility_version/g" \
    "$script_dir/Resources/Info.plist.in" > "$script_dir/Resources/Info.plist"
