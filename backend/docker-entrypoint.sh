#!/bin/sh
# Container entrypoint: start a virtual X display, then run surf-backend.
# Headful Chrome under Xvfb is less bot-flagged than headless Chrome.
set -eu

PROFILE="${PROFILE:-/data/profile}"
rm -f "$PROFILE"/Singleton* 2>/dev/null || true
mkdir -p "$PROFILE"

export DISPLAY=:99
export PULSE_SERVER=unix:/tmp/pulse/native
rm -f /tmp/.X99-lock /tmp/.X11-unix/X99 2>/dev/null || true

# Start on a max-size framebuffer; ffmpeg grabs only the active viewport size.
# RANDR resizing is best-effort, so this must be large enough for portrait too.
VW0="${VW:-1024}"
VH0="${VH:-768}"
XFB_DEFAULT="$VW0"
if [ "$VH0" -gt "$XFB_DEFAULT" ]; then XFB_DEFAULT="$VH0"; fi
XFB_W="${XFB_W:-$XFB_DEFAULT}"
XFB_H="${XFB_H:-$XFB_DEFAULT}"
Xvfb :99 -screen 0 "${XFB_W}x${XFB_H}x24" -nolisten tcp >/tmp/xvfb.log 2>&1 &
xvfb_pid=$!

n=0
while [ ! -e /tmp/.X11-unix/X99 ] && [ "$n" -lt 60 ]; do
  if ! kill -0 "$xvfb_pid" 2>/dev/null; then
    cat /tmp/xvfb.log >&2 2>/dev/null || true
    exit 1
  fi
  n=$((n + 1))
  sleep 0.1
done
if [ ! -e /tmp/.X11-unix/X99 ]; then
  cat /tmp/xvfb.log >&2 2>/dev/null || true
  exit 1
fi

mkdir -p /tmp/pulse
pulseaudio --daemonize=yes --exit-idle-time=-1 --disallow-exit=true \
  --load="module-native-protocol-unix socket=/tmp/pulse/native auth-anonymous=1" \
  --load="module-null-sink sink_name=surf_output sink_properties=device.description=Surf" \
  >/tmp/pulseaudio.log 2>&1 || { cat /tmp/pulseaudio.log >&2 2>/dev/null || true; exit 1; }
pactl set-default-sink surf_output >/tmp/pulseaudio.log 2>&1 || true
pactl set-sink-volume surf_output 100% >/tmp/pulseaudio.log 2>&1 || true
pactl set-sink-mute surf_output 0 >/tmp/pulseaudio.log 2>&1 || true

exec /app/surf-backend
