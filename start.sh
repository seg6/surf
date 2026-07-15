#!/bin/sh
# Start a virtual X display manually (no xvfb-run/xauth needed), then run the
# server. Headful Chrome under Xvfb is far less bot-flagged than headless.
PROFILE="${PROFILE:-/data/profile}"
rm -f "$PROFILE"/Singleton* 2>/dev/null
mkdir -p "$PROFILE"

export DISPLAY=:99
Xvfb :99 -screen 0 "${VW:-1024}x${VH:-768}x24" -nolisten tcp >/tmp/xvfb.log 2>&1 &

# wait for the display socket to appear
n=0
while [ ! -e /tmp/.X11-unix/X99 ] && [ "$n" -lt 60 ]; do n=$((n + 1)); sleep 0.1; done

exec /app/rbrowser
