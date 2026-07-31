#!/usr/bin/env bash
set -euo pipefail

SDK_DROP="/src/native/buildenv/sdk"
THEOS_SDKS="${THEOS:-/opt/theos}/sdks"

if [ ! -d "$SDK_DROP/iPhoneOS8.0.sdk" ]; then
  cat >&2 <<'EOF'
Missing native/buildenv/sdk/iPhoneOS8.0.sdk.

Run this from the repository root:
  native/buildenv/fetch-sdk.sh

EOF
  exit 2
fi

mkdir -p "$THEOS_SDKS"
ln -sfn "$SDK_DROP/iPhoneOS8.0.sdk" "$THEOS_SDKS/iPhoneOS8.0.sdk"
if [ -f "$SDK_DROP/libarclite_iphoneos.a" ]; then
  ln -sfn "$SDK_DROP/libarclite_iphoneos.a" "$THEOS_SDKS/libarclite_iphoneos.a"
fi

exec "$@"
