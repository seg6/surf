#!/usr/bin/env bash
set -euo pipefail

SDK_DROP="/src/native/buildenv/sdk"
THEOS_SDKS="${THEOS:-/opt/theos}/sdks"

if [ ! -d "$SDK_DROP/iPhoneOS6.1.sdk" ]; then
  cat >&2 <<'EOF'
Missing native/buildenv/sdk/iPhoneOS6.1.sdk.

Extract it from Xcode 4.6.3 and place it here:
  native/buildenv/sdk/iPhoneOS6.1.sdk

EOF
  exit 2
fi

mkdir -p "$THEOS_SDKS"
ln -sfn "$SDK_DROP/iPhoneOS6.1.sdk" "$THEOS_SDKS/iPhoneOS6.1.sdk"
if [ -f "$SDK_DROP/libarclite_iphoneos.a" ]; then
  ln -sfn "$SDK_DROP/libarclite_iphoneos.a" "$THEOS_SDKS/libarclite_iphoneos.a"
fi

exec "$@"
