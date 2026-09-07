#!/usr/bin/env bash
set -euo pipefail
for tool in cargo rg xvfb-run xauth; do
    command -v "$tool" >/dev/null 2>&1 || {
        echo "Missing test dependency: $tool (see client/desktop/README.md)" >&2
        exit 1
    }
done
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
wait_ready() {
    for _ in $(seq 1 100); do
        if ! kill -0 "$client_pid" 2>/dev/null; then
            echo "Surf exited before its test window was ready; client log follows:" >&2
            return 1
        fi
        if rg -q SURF_UI_STATE "$SURF_UI_TEST_ROOT/client.log"; then return; fi
        sleep .1
    done
    echo "Timed out waiting for the Surf test window; client log follows:" >&2
    return 1
}
wait_ready
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

# Exercise the redesigned task views through the same real event path.
restart_scene() {
    kill "$client_pid"
    wait "$client_pid" 2>/dev/null || true
    env SURF_UI_SIZE=1024x768 SURF_UI_GALLERY="$1" "$SURF_UI_TEST_BIN/surf-client" > "$SURF_UI_TEST_ROOT/client.log" 2>&1 &
    client_pid=$!
    wait_ready
    sleep .2
}
restart_scene new-tab
drive click 400 227
drive type "two word search"
expect '"new_tab_query":"two word search"'
drive key Return
expect '"new_tab_query":""'
rg -q 'SURF_UI_COMMAND Navigate.*two' "$SURF_UI_TEST_ROOT/client.log"
drive key ctrl+l
drive type "address again"
expect '"draft":"address again","editing":true'

restart_scene settings
drive click 245 235
expect '"settings_category":"Browsing"'
drive click 245 302
expect '"settings_category":"Testing"'
drive resize 375 667
expect '"viewport":\[375.0,625.0\]'
expect '"settings_category":"Testing"'
drive resize 320 480
expect '"viewport":\[320.0,438.0\]'
# Apply stays reachable outside the scroll area even at the smallest preset.
drive click 155 378
expect '"viewport":\[768.0,982.0\]'
drive key Escape
expect '"panel":"None"'
printf 'Surf real X11 input: focus, spaces, page input, popup Escape, resize, tab close, new-tab search and settings categories passed.\n'
X11
