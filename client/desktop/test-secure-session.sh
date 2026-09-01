#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "$0")/../.." && pwd)"
test_root="$(mktemp -d)"
backend_pid=""
pair_pid=""

cleanup() {
  if [[ -n "$pair_pid" ]]; then
    kill "$pair_pid" 2>/dev/null || true
    wait "$pair_pid" 2>/dev/null || true
  fi
  if [[ -n "$backend_pid" ]]; then
    kill "$backend_pid" 2>/dev/null || true
    wait "$backend_pid" 2>/dev/null || true
  fi
  rm -rf "$test_root"
}
trap cleanup EXIT

backend="$test_root/surf"
server_home="$test_root/server"
client_home="$test_root/client"
server_log="$test_root/server.log"
pair_log="$test_root/pair.log"
devices_log="$test_root/devices.log"
render_log="$test_root/render.log"
port=18443
animation_url="data:text/html;base64,PCFkb2N0eXBlIGh0bWw+PHN0eWxlPmh0bWwsYm9keXttYXJnaW46MDtoZWlnaHQ6MTAwJTtvdmVyZmxvdzpoaWRkZW47YmFja2dyb3VuZDojMTExfS5ib3h7d2lkdGg6MzUlO2hlaWdodDozNSU7YmFja2dyb3VuZDpsaW5lYXItZ3JhZGllbnQoMTM1ZGVnLCMzOWQsI2Y0NSk7YW5pbWF0aW9uOnN1cmYgMXMgbGluZWFyIGluZmluaXRlfUBrZXlmcmFtZXMgc3VyZnswJXt0cmFuc2Zvcm06dHJhbnNsYXRlKDAsMCk7ZmlsdGVyOmh1ZS1yb3RhdGUoMGRlZyl9NTAle3RyYW5zZm9ybTp0cmFuc2xhdGUoMTgwJSwxODAlKTtmaWx0ZXI6aHVlLXJvdGF0ZSgxODBkZWcpfTEwMCV7dHJhbnNmb3JtOnRyYW5zbGF0ZSgwLDApO2ZpbHRlcjpodWUtcm90YXRlKDM2MGRlZyl9fTwvc3R5bGU+PGRpdiBjbGFzcz0iYm94Ij48L2Rpdj4="

(
  cd "$repository_root/backend"
  go build -trimpath -o "$backend" ./cmd/surf
)

SURF_HOME="$server_home" PORT="$port" SURF_ADVERTISE=0 \
  "$backend" serve >"$server_log" 2>&1 &
backend_pid="$!"

ready=false
for _ in $(seq 1 120); do
  if SURF_HOME="$server_home" "$backend" status >/dev/null 2>&1; then
    ready=true
    break
  fi
  if ! kill -0 "$backend_pid" 2>/dev/null; then
    break
  fi
  sleep 0.25
done
if [[ "$ready" != true ]]; then
  echo "Surf integration backend did not become ready" >&2
  sed -n '1,240p' "$server_log" >&2
  exit 1
fi

SURF_HOME="$server_home" "$backend" pair >"$pair_log" 2>&1 &
pair_pid="$!"
pairing_code=""
for _ in $(seq 1 80); do
  pairing_code="$(sed -n 's/^Pairing code: \([0-9]\{6\}\)$/\1/p' "$pair_log" | head -n 1)"
  if [[ -n "$pairing_code" ]]; then
    break
  fi
  if ! kill -0 "$pair_pid" 2>/dev/null; then
    break
  fi
  sleep 0.25
done
if [[ -z "$pairing_code" ]]; then
  echo "Surf integration pairing code was not produced" >&2
  sed -n '1,240p' "$pair_log" >&2
  sed -n '1,240p' "$server_log" >&2
  exit 1
fi

(
  cd "$repository_root/client/desktop"
  SURF_CLIENT_HOME="$client_home" cargo run -q -p surf-media --example probe_decode -- \
    "127.0.0.1:$port" "$pairing_code" --confirm --expect-audio
)

if ! (
  cd "$repository_root/client/desktop"
  timeout 40s xvfb-run -a env \
    SURF_CLIENT_HOME="$client_home" \
    SURF_SMOKE_EXIT_AFTER_FRAMES=240 \
    SURF_SMOKE_INTERACTION=1 \
    cargo run -q -p surf-client -- "$animation_url" >"$render_log" 2>&1
); then
  echo "Surf OpenGL client process did not complete its sustained smoke test" >&2
  sed -n '1,240p' "$render_log" >&2
  exit 1
fi
if ! rg -q '^SURF_SMOKE_RESULT presented=240 ' "$render_log"; then
  echo "Surf OpenGL client did not sustain its live presentation smoke test" >&2
  sed -n '1,240p' "$render_log" >&2
  exit 1
fi
for step in resize-small edit-omnibox resize-large; do
  if ! rg -q "^SURF_SMOKE_STEP $step " "$render_log"; then
    echo "Surf sustained smoke test did not execute $step" >&2
    sed -n '1,240p' "$render_log" >&2
    exit 1
  fi
done
smoke_fps="$(sed -n 's/^SURF_SMOKE_RESULT .* fps=\([0-9.]*\) .*/\1/p' "$render_log")"
if ! awk -v fps="$smoke_fps" 'BEGIN { exit !(fps >= 55.0) }'; then
  echo "Surf sustained presentation rate was ${smoke_fps:-missing} FPS; expected at least 55" >&2
  sed -n '1,240p' "$render_log" >&2
  exit 1
fi
sed -n '/^SURF_SMOKE_RESULT /p' "$render_log"

kill "$pair_pid" 2>/dev/null || true
wait "$pair_pid" 2>/dev/null || true
pair_pid=""
SURF_HOME="$server_home" "$backend" devices list >"$devices_log"
SURF_HOME="$server_home" "$backend" quit >/dev/null
wait "$backend_pid"
backend_pid=""

if ! rg -q 'Surf integration probe' "$devices_log"; then
  echo "Surf integration backend did not persist the paired device" >&2
  sed -n '1,240p' "$devices_log" >&2
  exit 1
fi

echo "Secure session, FFmpeg decode, and OpenGL YUV presentation integration passed"
