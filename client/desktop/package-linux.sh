#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "$0")/../.." && pwd)"
version="$(tr -d '[:space:]' < "$repository_root/VERSION")"
cargo_version="$(sed -n 's/^version = "\([^"]*\)"/\1/p' "$repository_root/client/desktop/Cargo.toml" | head -n 1)"
if [[ "$cargo_version" != "$version" ]]; then
  echo "desktop Cargo version $cargo_version does not match Surf $version" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64) architecture="amd64" ;;
  aarch64|arm64) architecture="arm64" ;;
  *)
    echo "unsupported Linux desktop architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

target_dir="${SURF_DESKTOP_TARGET_DIR:-$repository_root/.local/build/desktop-release}"
output_dir="${SURF_DESKTOP_OUTPUT_DIR:-$repository_root/dist}"
package_name="surf-desktop-$version-linux-$architecture"
stage="$output_dir/$package_name"

CARGO_TARGET_DIR="$target_dir" \
  cargo build --locked --release \
    --manifest-path "$repository_root/client/desktop/Cargo.toml" \
    -p surf-client

binary="$target_dir/release/surf-client"
if ldd "$binary" | grep -q 'not found'; then
  echo "desktop client has unresolved runtime libraries" >&2
  ldd "$binary" >&2
  exit 1
fi

rm -rf "$stage"
mkdir -p "$stage/bin" "$stage/share/applications" "$stage/share/icons/hicolor/1024x1024/apps"
install -m 0755 "$binary" "$stage/bin/surf-client"
install -m 0644 "$repository_root/LICENSE" "$stage/LICENSE"
install -m 0644 "$repository_root/THIRD_PARTY_NOTICES.md" "$stage/THIRD_PARTY_NOTICES.md"
mkdir -p "$stage/licenses"
install -m 0644 "$repository_root/client/desktop/assets/fonts/LICENSE.txt" "$stage/licenses/Inter-OFL.txt"
install -m 0644 "$repository_root/client/ios/Artwork/LUCIDE-LICENSE.txt" "$stage/licenses/Lucide.txt"
install -m 0644 "$repository_root/client/desktop/assets/icons/RESVG-LICENSE-MIT.txt" "$stage/licenses/resvg-MIT.txt"
install -m 0644 "$repository_root/client/ios/Artwork/DETA-SURF-LICENSE.txt" "$stage/licenses/Deta-Surf.txt"
install -m 0644 "$repository_root/client/desktop/README.md" "$stage/README.md"
install -m 0644 "$repository_root/backend/cmd/surf/surf-icon.png" \
  "$stage/share/icons/hicolor/1024x1024/apps/space.seg6.surf.client.png"
install -m 0644 "$repository_root/client/desktop/surf-client.desktop.in" \
  "$stage/share/applications/space.seg6.surf.client.desktop"
chmod 0644 "$stage/share/applications/space.seg6.surf.client.desktop"

mkdir -p "$output_dir"
archive="$output_dir/$package_name.tar.gz"
tar -C "$output_dir" -czf "$archive" "$package_name"
printf '%s\n' "$archive"
