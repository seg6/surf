#!/usr/bin/env bash
set -euo pipefail

SDK_URL="${SDK_URL:-https://github.com/GrowtopiaJaw/iPhoneOS-SDK/releases/download/v1.0/iPhoneOS8.0.sdk.zip}"
SDK_SHA256="${SDK_SHA256:-5e770b202937ca31b8547aa4dbef7543e3aa261f6b744012745604588e927b05}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SDK_DIR="$ROOT/native/buildenv/sdk"
SDK_PATH="$SDK_DIR/iPhoneOS8.0.sdk"

verify_sha256() {
  expected="$1"
  file="$2"
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s  %s\n' "$expected" "$file" | sha256sum -c -
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$file")"
    actual="${actual%% *}"
    if [ "$actual" = "$expected" ]; then
      return
    fi
    echo "sha256 mismatch for $file: expected $expected, got $actual" >&2
    exit 1
  fi
  echo "sha256sum or shasum is required to verify $file" >&2
  exit 1
}

if [ -d "$SDK_PATH" ]; then
  echo "iPhoneOS8.0.sdk already present at $SDK_PATH"
  exit 0
fi

tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

mkdir -p "$SDK_DIR" "$tmp/extract"
echo "Downloading iPhoneOS8.0.sdk..."
curl -fL "$SDK_URL" -o "$tmp/iPhoneOS8.0.sdk.zip"
verify_sha256 "$SDK_SHA256" "$tmp/iPhoneOS8.0.sdk.zip"
unzip -q "$tmp/iPhoneOS8.0.sdk.zip" -d "$tmp/extract"

found="$tmp/extract/iPhoneOS8.0.sdk"
if [ ! -d "$found" ]; then
  found="$(find "$tmp/extract" -type d -name iPhoneOS8.0.sdk -print -quit)"
fi
if [ ! -d "$found" ]; then
  echo "Downloaded archive did not contain iPhoneOS8.0.sdk" >&2
  exit 1
fi

mv "$found" "$SDK_PATH"
echo "Installed SDK at $SDK_PATH"
