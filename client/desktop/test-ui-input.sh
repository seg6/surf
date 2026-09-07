#!/usr/bin/env bash
set -euo pipefail
repository_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repository_root/client/desktop"
cargo build --locked -p surf-client --example ui-input
cargo build --locked -p surf-client
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
export SURF_UI_TEST_ROOT="$test_root"
export SURF_UI_TEST_BIN="${CARGO_TARGET_DIR:-$repository_root/client/desktop/target}/debug"
xvfb-run -a -s "-screen 0 1920x1400x24" bash <<'X11'
set -euo pipefail
export LIBGL_ALWAYS_SOFTWARE=1 WINIT_UNIX_BACKEND=x11 WAYLAND_DISPLAY=
export SURF_CLIENT_HOME="$SURF_UI_TEST_ROOT/client" SURF_UI_GALLERY=browser SURF_UI_TRACE=1
"$SURF_UI_TEST_BIN/surf-client" > "$SURF_UI_TEST_ROOT/client.log" 2>&1 &
client_pid=$!
trap 'status=$?; if [[ $status != 0 ]]; then cat "$SURF_UI_TEST_ROOT/client.log"; fi; kill "$client_pid" 2>/dev/null || true' EXIT
for _ in $(seq 1 100); do
    if rg -q SURF_UI_STATE "$SURF_UI_TEST_ROOT/client.log"; then break; fi
    sleep .1
done
drive() { "$SURF_UI_TEST_BIN/examples/ui-input" "$@"; }
expect() {
    for _ in $(seq 1 60); do
        if rg SURF_UI_STATE "$SURF_UI_TEST_ROOT/client.log" | tail -n 1 | rg -q "$1"; then return; fi
        sleep .05
    done
    return 1
}
drive focus-type "hello world"
expect '"draft":"hello world","editing":true'
drive key Escape
drive key ctrl+l
drive type "second search"
drive key Return
sleep .2
expect '"editing":false'
# Re-enter immediately: the previous submit must not steal the next local edit.
drive key ctrl+l
drive type "third search"
expect '"draft":"third search","editing":true'
drive key ctrl+l
drive paste "hello café 世界"
expect '"draft":"hello café 世界","editing":true'
# The bottom tools button opens a native popup; Escape should dismiss it.
drive key Escape
drive click 1160 739
expect '"panel":"Some\(More\)"'
drive key Escape
expect '"panel":"None"'
# Clicking the page relinquishes local navigation focus and routes spaces remotely.
drive click 300 300
expect '"page_focused":true'
drive type "a b"
for _ in $(seq 1 60); do
    if rg -q 'SURF_UI_COMMAND Key.*text: " "' "$SURF_UI_TEST_ROOT/client.log"; then break; fi
    sleep .05
done
rg -q 'SURF_UI_COMMAND Key.*text: " "' "$SURF_UI_TEST_ROOT/client.log"
drive resize 375 667
expect '"viewport":\[375.0,625.0\]'
drive click 320 646
expect '"panel":"Some\(Tabs\)"'
drive click 340 440
rg -q 'SURF_UI_COMMAND Tab.*action: "close", id: 1' "$SURF_UI_TEST_ROOT/client.log"
expect '"panel":"Some\(Tabs\)"'
drive key Escape
expect '"panel":"None"'
drive click 120 646
drive type "clicked"
expect '"draft":"clicked","editing":true'
printf 'Surf real X11 input: focus, spaces, page input, popup Escape, resize and tab close passed.\n'
X11
