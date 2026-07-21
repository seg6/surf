#!/usr/bin/env bash
set -euo pipefail

version="${VERSION:?VERSION is required}"
protocol_version="${PROTOCOL_VERSION:?PROTOCOL_VERSION is required}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid VERSION: $version" >&2
  exit 1
fi
if [[ ! "$protocol_version" =~ ^[0-9]{8}-[0-9A-Za-z.-]+$ ]]; then
  echo "invalid PROTOCOL_VERSION: $protocol_version" >&2
  exit 1
fi

sed "s/@VERSION@/$version/g" control.in > control
sed "s/@VERSION@/$version/g" Resources/Info.plist.in > Resources/Info.plist
