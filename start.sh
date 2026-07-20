#!/bin/sh
# Start a virtual X display manually (no xvfb-run/xauth needed), then run the
# server. Headful Chrome under Xvfb is far less bot-flagged than headless.
PROFILE="${PROFILE:-/data/profile}"
rm -f "$PROFILE"/Singleton* 2>/dev/null
mkdir -p "$PROFILE"

export DISPLAY=:99
# Start on a max-size framebuffer; ffmpeg grabs only the active viewport size.
# RANDR resizing is best-effort, so this must be large enough for portrait too.
VW0="${VW:-1024}"
VH0="${VH:-768}"
XFB_DEFAULT="$VW0"
if [ "$VH0" -gt "$XFB_DEFAULT" ]; then XFB_DEFAULT="$VH0"; fi
XFB_W="${XFB_W:-$XFB_DEFAULT}"
XFB_H="${XFB_H:-$XFB_DEFAULT}"
Xvfb :99 -screen 0 "${XFB_W}x${XFB_H}x24" -nolisten tcp >/tmp/xvfb.log 2>&1 &

# wait for the display socket to appear
n=0
while [ ! -e /tmp/.X11-unix/X99 ] && [ "$n" -lt 60 ]; do n=$((n + 1)); sleep 0.1; done

exec /app/rbrowser
