#!/usr/bin/env bash
set -euo pipefail

artwork_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
client_dir="$(cd "$artwork_dir/.." && pwd)"
source_icon="$artwork_dir/deta-surf-icon.png"
source_plane="$artwork_dir/deta-surf-plane.png"
resources="$client_dir/Resources"
expected_sha256="a7fe7713e95bb66a476e9f65cbff4fae4441c52dfd12f284548f321382e9f594"
expected_plane_sha256="c2eecccb48ad6a929549c9411ec6506653831fec7405122474e5c1f7e6d5eb30"

if ! command -v magick >/dev/null 2>&1; then
  echo "ImageMagick 7 is required (missing magick)" >&2
  exit 1
fi
if [ ! -f "$source_icon" ]; then
  echo "Missing pinned source icon: $source_icon" >&2
  exit 1
fi
if [ ! -f "$source_plane" ]; then
  echo "Missing pinned source plane: $source_plane" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual_sha256="$(sha256sum "$source_icon" | awk '{print $1}')"
else
  actual_sha256="$(shasum -a 256 "$source_icon" | awk '{print $1}')"
fi
if [ "$actual_sha256" != "$expected_sha256" ]; then
  echo "Pinned source icon checksum mismatch: $actual_sha256" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual_plane_sha256="$(sha256sum "$source_plane" | awk '{print $1}')"
else
  actual_plane_sha256="$(shasum -a 256 "$source_plane" | awk '{print $1}')"
fi
if [ "$actual_plane_sha256" != "$expected_plane_sha256" ]; then
  echo "Pinned source plane checksum mismatch: $actual_plane_sha256" >&2
  exit 1
fi

temporary_dir="$(mktemp -d)"
cleanup() { rm -rf "$temporary_dir"; }
trap cleanup EXIT

# The separate upstream plane supplies a clean alpha silhouette. Apply that
# silhouette to the high-resolution production artwork so modern icons keep
# the source illustration's crisp blue ink and stitched details.
magick "$source_plane" -filter Lanczos -resize 940x940 -gravity center \
  -background none -extent 1024x1024 -alpha extract +profile icc \
  "$temporary_dir/plane-mask.png"
magick "$source_icon" "$temporary_dir/plane-mask.png" -alpha off \
  -compose CopyOpacity -composite "$temporary_dir/plane-hi.png"

make_legacy_icon() {
  output="$1"
  pixels="$2"
  # iOS 6 preserves the original pre-rendered composition: the plane and its
  # shadow visibly escape the rounded blue tile.
  magick "$source_icon" -filter Lanczos -resize "${pixels}x${pixels}" \
    -strip -depth 8 "PNG32:$resources/$output"
}

make_modern_icon() {
  output="$1"
  pixels="$2"
  # iOS 7+ masks the complete square itself. Use a full-bleed opaque field and
  # let the oversized plane approach that mask without an inset tile/border.
  magick -size "${pixels}x${pixels}" gradient:'#27C4F8-#087FD9' \
    \( "$temporary_dir/plane-hi.png" -filter Lanczos -resize "${pixels}x${pixels}" \) \
    -gravity center -composite -strip -depth 8 "PNG24:$resources/$output"
}

make_mark() {
  output="$1"
  pixels="$2"
  magick "$source_icon" -resize "${pixels}x${pixels}" -strip -depth 8 \
    "$resources/$output"
}

make_launch() {
  output="$1"
  width="$2"
  height="$3"
  mark_size="$4"
  magick -size "${width}x${height}" xc:'#F7FAFC' \
    \( "$source_icon" -resize "${mark_size}x${mark_size}" \) \
    -gravity center -composite -strip -depth 8 "PNG24:$resources/$output"
}

make_legacy_icon icon-57.png 57
make_legacy_icon icon-57@2x.png 114
make_legacy_icon icon-72.png 72
make_legacy_icon icon-72@2x.png 144
make_legacy_icon icon-144.png 144
make_legacy_icon icon-classic-60.png 60
make_legacy_icon icon-classic-60@2x.png 120
make_legacy_icon icon-classic-60@3x.png 180
make_legacy_icon icon-classic-76.png 76
make_legacy_icon icon-classic-76@2x.png 152
make_legacy_icon icon-classic-167.png 167
make_modern_icon icon-60.png 60
make_modern_icon icon-60@2x.png 120
make_modern_icon icon-60@3x.png 180
make_modern_icon icon-76.png 76
make_modern_icon icon-76@2x.png 152
make_modern_icon icon-167.png 167
make_mark brand-mark.png 144

make_launch Default.png 320 480 100
make_launch Default@2x.png 640 960 200
make_launch Default-568h@2x.png 640 1136 200
make_launch Default-667h@2x.png 750 1334 230
make_launch Default-736h@3x.png 1242 2208 370
make_launch Default-Landscape-736h@3x.png 2208 1242 310

echo "Generated OS-specific legacy and modern Surf icons plus launch images from pinned artwork"
