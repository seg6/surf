#!/usr/bin/env bash
set -euo pipefail

icon_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$icon_dir/../../.." && pwd)"
source_icon="$repo_dir/native/client/Artwork/deta-surf-icon.png"
source_plane="$repo_dir/native/client/Artwork/deta-surf-plane.png"

if ! command -v magick >/dev/null 2>&1; then
  echo "ImageMagick 7 is required (missing magick)" >&2
  exit 1
fi

# Desktop launchers should receive the full-resolution production mark. The
# status tray gets the separately sourced plane silhouette so it stays legible
# in a 16–32 point slot without shrinking the blue app tile into noise.
cp "$source_icon" "$icon_dir/surf-icon.png"
magick "$source_plane" -filter Lanczos -resize 32x32 -strip -depth 8 \
  "PNG32:$icon_dir/tray-icon.png"

echo "Generated desktop and tray icons from pinned Deta Surf artwork"
