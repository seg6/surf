#!/bin/sh
# Local LAN launcher. Runs Surf fully inside Docker and advertises it on Bonjour
# so the iPad can find it with "Find Local Surf".
set -eu

IMAGE="${IMAGE:-surf-backend:lan}"
NAME="${NAME:-surf-backend-lan}"
PORT="${PORT:-18080}"

if [ -f ./.env ]; then
  set -a
  . ./.env
  set +a
fi

if [ -z "${SURF_PASSWORD:-}" ]; then
  echo "SURF_PASSWORD is required. Set it in backend/.env or export it before running ./start.sh." >&2
  exit 1
fi
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
VERSION="$(tr -d '[:space:]' < "$SCRIPT_DIR/../VERSION")"
PROTOCOL_VERSION="$(tr -d '[:space:]' < "$SCRIPT_DIR/../PROTOCOL_VERSION")"

docker build --network host \
  --build-arg VERSION="$VERSION" \
  --build-arg PROTOCOL_VERSION="$PROTOCOL_VERSION" \
  -t "$IMAGE" .
AUTH_HASH="$(docker run --rm --network host --entrypoint /app/surf-backend "$IMAGE" -hash-password "$SURF_PASSWORD")"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --name "$NAME" \
  --network host \
  --shm-size 1g \
  -v surf_backend_lan_profile:/data/profile \
  -v surf_backend_lan_downloads:/data/downloads \
  -e PORT="$PORT" \
  -e PROFILE=/data/profile \
  -e START_URL="${START_URL:-https://www.google.com}" \
  -e VW="${VW:-1024}" \
  -e VH="${VH:-768}" \
  -e XFB_W="${XFB_W:-1024}" \
  -e XFB_H="${XFB_H:-1024}" \
  -e QUALITY="${QUALITY:-55}" \
  -e NATIVE_QUALITY="${NATIVE_QUALITY:-100}" \
  -e NATIVE_MOTION_QUALITY="${NATIVE_MOTION_QUALITY:-92}" \
  -e STREAM_FPS="${STREAM_FPS:-30}" \
  -e STREAM_SCALE="${STREAM_SCALE:-800x800}" \
  -e STREAM_BITRATE="${STREAM_BITRATE:-2800}" \
  -e STREAM_MAXRATE="${STREAM_MAXRATE:-3600}" \
  -e STREAM_BUFSIZE="${STREAM_BUFSIZE:-900}" \
  -e STREAM_PRESET="${STREAM_PRESET:-ultrafast}" \
  -e SURF_ADVERTISE=1 \
  -e AUTH_HASH="$AUTH_HASH" \
  "$IMAGE"

echo "Surf LAN is running at http://localhost:$PORT"
echo "Use the password configured in SURF_PASSWORD."
echo "iPad: Settings -> Find Local Surf -> Connect"
