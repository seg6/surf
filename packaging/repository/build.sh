#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: build.sh CLIENT_DEB OUTPUT_DIRECTORY" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
client_deb="$1"
output_dir="$2"
repository_url="https://seg6.space/surf/"

for command in \
    ar basename bzip2 cut date find grep gzip install md5sum sed sha1sum \
    sha256sum sha512sum stat tar; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done

test -f "$client_deb" || {
  echo "client package does not exist: $client_deb" >&2
  exit 1
}
test -s "$client_deb" || {
  echo "client package is empty: $client_deb" >&2
  exit 1
}

client_name="$(basename "$client_deb")"
[[ "$client_name" =~ ^space\.seg6\.surf_([0-9]+\.[0-9]+\.[0-9]+)-([0-9]+)_iphoneos-arm\.deb$ ]] || {
  echo "unexpected client package name: $client_name" >&2
  exit 1
}
app_version="${BASH_REMATCH[1]}"
package_version="${BASH_REMATCH[1]}-${BASH_REMATCH[2]}"

mapfile -t archive_members < <(ar t "$client_deb")
test "${archive_members[0]:-}" = "debian-binary" || {
  echo "client package does not start with debian-binary" >&2
  exit 1
}
control_member=""
for member in "${archive_members[@]}"; do
  if [[ "$member" =~ ^control\.tar\.(gz|xz|bz2)$ ]]; then
    control_member="$member"
    break
  fi
done
test -n "$control_member" || {
  echo "client package has no supported control archive" >&2
  exit 1
}

case "$control_member" in
  control.tar.gz) control_tar_flag=z ;;
  control.tar.xz) control_tar_flag=J ;;
  control.tar.bz2) control_tar_flag=j ;;
esac
control="$({ ar p "$client_deb" "$control_member" | tar -x"$control_tar_flag"Of - ./control; })"
test -n "$control"

field() {
  local name="$1"
  sed -n "s/^${name}: //p" <<< "$control"
}

test "$(field Package)" = "space.seg6.surf"
test "$(field Name)" = "Surf"
test "$(field Architecture)" = "iphoneos-arm"
test "$(field Version)" = "$package_version"

mkdir -p "$output_dir"
if find "$output_dir" -mindepth 1 -print -quit | grep -q .; then
  echo "output directory must be empty: $output_dir" >&2
  exit 1
fi
mkdir -p "$output_dir/debs"
install -m 0644 "$client_deb" "$output_dir/debs/$client_name"
install -m 0644 "$script_dir/../../native/client/Resources/icon-144.png" \
  "$output_dir/CydiaIcon.png"
: > "$output_dir/.nojekyll"

package_path="debs/$client_name"
package_size="$(stat -c '%s' "$output_dir/$package_path")"
package_md5="$(md5sum "$output_dir/$package_path" | cut -d ' ' -f 1)"
package_sha1="$(sha1sum "$output_dir/$package_path" | cut -d ' ' -f 1)"
package_sha256="$(sha256sum "$output_dir/$package_path" | cut -d ' ' -f 1)"
package_sha512="$(sha512sum "$output_dir/$package_path" | cut -d ' ' -f 1)"

{
  printf '%s\n' "$control"
  printf 'Homepage: %s\n' "$repository_url"
  printf 'Icon: %sCydiaIcon.png\n' "$repository_url"
  printf 'Filename: %s\n' "$package_path"
  printf 'Size: %s\n' "$package_size"
  printf 'MD5sum: %s\n' "$package_md5"
  printf 'SHA1: %s\n' "$package_sha1"
  printf 'SHA256: %s\n' "$package_sha256"
  printf 'SHA512: %s\n\n' "$package_sha512"
} > "$output_dir/Packages"

gzip -9 -n -c "$output_dir/Packages" > "$output_dir/Packages.gz"
bzip2 -9 -c "$output_dir/Packages" > "$output_dir/Packages.bz2"

if [ -n "${SURF_REPOSITORY_DATE:-}" ]; then
  release_date="$(date -u -d "$SURF_REPOSITORY_DATE" -R)"
else
  release_date="$(date -u -R)"
fi

{
  printf 'Origin: Surf\n'
  printf 'Label: Surf\n'
  printf 'Suite: stable\n'
  printf 'Codename: surf\n'
  printf 'Version: %s\n' "$app_version"
  printf 'Architectures: iphoneos-arm\n'
  printf 'Components: main\n'
  printf 'Description: Surf browser client for legacy iOS\n'
  printf 'Date: %s\n' "$release_date"
  for algorithm in MD5Sum SHA1 SHA256; do
    printf '%s:\n' "$algorithm"
    case "$algorithm" in
      MD5Sum) hash_command=md5sum ;;
      SHA1) hash_command=sha1sum ;;
      SHA256) hash_command=sha256sum ;;
    esac
    for metadata in Packages Packages.gz Packages.bz2; do
      metadata_hash="$($hash_command "$output_dir/$metadata" | cut -d ' ' -f 1)"
      metadata_size="$(stat -c '%s' "$output_dir/$metadata")"
      printf ' %s %16s %s\n' "$metadata_hash" "$metadata_size" "$metadata"
    done
  done
} > "$output_dir/Release"

sed \
  -e "s|@APP_VERSION@|$app_version|g" \
  -e "s|@PACKAGE_VERSION@|$package_version|g" \
  -e "s|@PACKAGE_PATH@|$package_path|g" \
  -e "s|@PACKAGE_SIZE@|$package_size|g" \
  -e "s|@PACKAGE_SHA256@|$package_sha256|g" \
  "$script_dir/index.html.in" > "$output_dir/index.html"

printf 'Built Surf %s package repository in %s\n' "$package_version" "$output_dir"
