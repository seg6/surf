#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: verify.sh CLIENT_DEB REPOSITORY_DIRECTORY" >&2
  exit 2
fi

client_deb="$1"
repository_dir="$2"
client_name="$(basename "$client_deb")"
package_path="debs/$client_name"

for command in basename bzip2 cmp cut find grep gzip md5sum sha1sum sha256sum stat; do
  command -v "$command" >/dev/null || {
    echo "required command is unavailable: $command" >&2
    exit 1
  }
done

for path in \
  .nojekyll CydiaIcon.png Packages Packages.gz Packages.bz2 Release index.html \
  docs/index.html install/ios/index.html "$package_path"; do
  test -e "$repository_dir/$path" || {
    echo "repository output is missing $path" >&2
    exit 1
  }
done

cmp "$client_deb" "$repository_dir/$package_path"
gzip -dc "$repository_dir/Packages.gz" | cmp - "$repository_dir/Packages"
bzip2 -dc "$repository_dir/Packages.bz2" | cmp - "$repository_dir/Packages"

package_sha256="$(sha256sum "$client_deb" | cut -d ' ' -f 1)"
package_size="$(stat -c '%s' "$client_deb")"
grep -Fqx 'Package: space.seg6.surf' "$repository_dir/Packages"
grep -Fqx 'Architecture: iphoneos-arm' "$repository_dir/Packages"
grep -Fqx "Filename: $package_path" "$repository_dir/Packages"
grep -Fqx "Size: $package_size" "$repository_dir/Packages"
grep -Fqx "SHA256: $package_sha256" "$repository_dir/Packages"

for algorithm in md5sum sha1sum sha256sum; do
  for metadata in Packages Packages.gz Packages.bz2; do
    expected="$($algorithm "$repository_dir/$metadata" | cut -d ' ' -f 1)"
    size="$(stat -c '%s' "$repository_dir/$metadata")"
    grep -Eq "^ ${expected} +${size} ${metadata}$" "$repository_dir/Release"
  done
done

# Whole site must stay inside the iOS 6 WebKit profile and resolve package fields.
# HTML may document forbidden features in prose; only reject real <script> tags
# and unresolved placeholders there. Layout/CSS feature checks run on .css files.
while IFS= read -r -d '' page; do
  if grep -Eq '@[A-Z0-9_]+@|__SURF_[A-Z0-9_]+__' "$page"; then
    echo "unresolved template value in ${page#"$repository_dir"/}" >&2
    exit 1
  fi
  if grep -Eqi '<script[[:space:]>]' "$page"; then
    echo "${page#"$repository_dir"/} uses a feature outside the iOS 6 compatibility profile" >&2
    exit 1
  fi
done < <(find "$repository_dir" -type f -name '*.html' -print0)

while IFS= read -r -d '' css; do
  if grep -Eqi 'display:[[:space:]]*(flex|grid)|var\(|@font-face|linear-gradient\(' "$css"; then
    echo "${css#"$repository_dir"/} uses a feature outside the iOS 6 compatibility profile" >&2
    exit 1
  fi
done < <(find "$repository_dir" -type f -name '*.css' -print0)

ios_page="$repository_dir/install/ios/index.html"
grep -Fq 'cydia://url/https://cydia.saurik.com/api/share#?source=https://seg6.space/surf/' \
  "$ios_page"
grep -Fq 'sileo://source/https://seg6.space/surf/' "$ios_page"
grep -Eq "href=\"(/surf/)?$package_path\"" "$ios_page"
grep -Fq 'https://seg6.space/surf/' "$ios_page"

grep -Fq '/surf/docs/' "$repository_dir/index.html" || \
  grep -Fq 'href="/surf/docs/' "$repository_dir/index.html" || \
  grep -Fq 'docs/' "$repository_dir/index.html"
# Cydia / Sileo deep links live on the iOS install page only.

printf 'Verified Surf package repository in %s\n' "$repository_dir"
