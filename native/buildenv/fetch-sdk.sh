#!/usr/bin/env bash
set -euo pipefail

SDK_URL="${SDK_URL:-https://github.com/GrowtopiaJaw/iPhoneOS-SDK/releases/download/v1.0/iPhoneOS6.1.sdk.zip}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SDK_DIR="$ROOT/native/buildenv/sdk"
SDK_PATH="$SDK_DIR/iPhoneOS6.1.sdk"

if [ -d "$SDK_PATH" ]; then
  echo "iPhoneOS6.1.sdk already present at $SDK_PATH"
  exit 0
fi

tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

mkdir -p "$SDK_DIR" "$tmp/extract"
echo "Downloading iPhoneOS6.1.sdk..."
curl -fL "$SDK_URL" -o "$tmp/iPhoneOS6.1.sdk.zip"
unzip -q "$tmp/iPhoneOS6.1.sdk.zip" -d "$tmp/extract"

found="$tmp/extract/iPhoneOS6.1.sdk"
if [ ! -d "$found" ]; then
  found="$(find "$tmp/extract" -type d -name iPhoneOS6.1.sdk -print -quit)"
fi
if [ ! -d "$found" ]; then
  echo "Downloaded archive did not contain iPhoneOS6.1.sdk" >&2
  exit 1
fi

mv "$found" "$SDK_PATH"
echo "Installed SDK at $SDK_PATH"
