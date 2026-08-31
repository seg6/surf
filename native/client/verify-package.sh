#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION="$(tr -d '[:space:]' < "$ROOT/../../VERSION")"
LAST_PACKAGE="$ROOT/.theos/last_package"

if [ ! -f "$LAST_PACKAGE" ]; then
  echo "Missing $LAST_PACKAGE; build the package first" >&2
  exit 1
fi

package_path="$(tr -d '\r\n' < "$LAST_PACKAGE")"
case "$package_path" in
  /*) package="$package_path" ;;
  *) package="$ROOT/${package_path#./}" ;;
esac
if [ ! -f "$package" ]; then
  echo "Last package does not exist: $package" >&2
  exit 1
fi

if [ "$(dpkg-deb -f "$package" Package)" != "space.seg6.surf" ]; then
  echo "Unexpected package identifier in $package" >&2
  exit 1
fi
if [ "$(dpkg-deb -f "$package" Architecture)" != "iphoneos-arm" ]; then
  echo "Unexpected package architecture in $package" >&2
  exit 1
fi
package_version="$(dpkg-deb -f "$package" Version)"
case "$package_version" in
  "$VERSION"|"$VERSION"-*) ;;
  *)
    echo "Package version is $package_version, expected $VERSION" >&2
    exit 1
    ;;
esac

tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT
dpkg-deb -x "$package" "$tmp"

app="$tmp/Applications/Surf.app/Surf"
updater="$tmp/usr/libexec/surf-update-v2"
icon_selector="$tmp/usr/libexec/surf-select-icons"

minimum_version() {
  arch="$1"
  binary="$2"
  otool -arch "$arch" -l "$binary" | awk '
    $1 == "cmd" && ($2 == "LC_VERSION_MIN_IPHONEOS" || $2 == "LC_BUILD_VERSION") {
      in_version_command = 1
      next
    }
    !found && in_version_command && ($1 == "version" || $1 == "minos") {
      version = $2
      found = 1
    }
    END {
      if (found) print version
    }
  '
}

verify_binary() {
  binary="$1"
  if [ ! -f "$binary" ]; then
    echo "Missing binary in package: $binary" >&2
    exit 1
  fi

  lipo "$binary" -verify_arch armv7 arm64

  armv7_min="$(minimum_version armv7 "$binary")"
  arm64_min="$(minimum_version arm64 "$binary")"
  case "$armv7_min" in
    6.0|6.0.0) ;;
    *)
      echo "$binary armv7 slice targets iOS $armv7_min, expected 6.0" >&2
      exit 1
      ;;
  esac
  case "$arm64_min" in
    7.0|7.0.0) ;;
    *)
      echo "$binary arm64 slice targets iOS $arm64_min, expected 7.0" >&2
      exit 1
      ;;
  esac

  echo "Verified $(basename "$binary"): armv7 (iOS $armv7_min), arm64 (iOS $arm64_min)"
}

verify_binary "$app"
verify_binary "$updater"

# iOS 6/7 provide the core AVSampleBufferDisplayLayer methods but not the
# failure-notification data symbol published with iOS 8. A strong import would
# make dyld reject the app before Surf's runtime capability check can run.
failure_notification_import="$(
  nm -arch armv7 -m "$app" |
    grep '_AVSampleBufferDisplayLayerFailedToDecodeNotification' || true
)"
case "$failure_notification_import" in
  *"(undefined) weak external"*) ;;
  *)
    echo "iOS 8 display-layer failure notification must remain weak-imported" >&2
    exit 1
    ;;
esac

plist="$tmp/Applications/Surf.app/Info.plist"
plist_xml="$tmp/Info.xml"
plistutil -i "$plist" -f xml -o "$plist_xml"
device_families="$(sed -n '/<key>UIDeviceFamily<\/key>/,/<\/array>/p' "$plist_xml")"
phone_family_count="$(printf '%s\n' "$device_families" | grep -c '<integer>1</integer>' || true)"
tablet_family_count="$(printf '%s\n' "$device_families" | grep -c '<integer>2</integer>' || true)"
if [ "$phone_family_count" != 1 ] || [ "$tablet_family_count" != 1 ]; then
  echo "UIDeviceFamily must contain phone/iPod (1) and iPad (2) exactly once" >&2
  exit 1
fi
required_capabilities="$(
  sed -n '/<key>UIRequiredDeviceCapabilities<\/key>/,/<\/array>/p' "$plist_xml"
)"
if grep -q '<string>armv7</string>' <<< "$required_capabilities"; then
  echo "Info.plist still excludes arm64-only devices" >&2
  exit 1
fi

app_fonts="$(sed -n '/<key>UIAppFonts<\/key>/,/<\/array>/p' "$plist_xml")"
if ! grep -q '<string>Lucide.ttf</string>' <<< "$app_fonts"; then
  echo "Info.plist does not register the bundled Lucide icon font" >&2
  exit 1
fi

for icon_name in Icon.png Icon@2x.png Icon-72.png Icon-72@2x.png Icon-60 Icon-76 Icon-83.5; do
  if ! grep -q "<string>$icon_name</string>" "$plist_xml"; then
    echo "Info.plist does not register $icon_name" >&2
    exit 1
  fi
done

if grep -q '<key>CFBundleIcons' "$plist_xml"; then
  echo "Rootful package must use legacy CFBundleIconFiles size selection" >&2
  exit 1
fi

for resource in \
  Icon.png Icon@2x.png Icon-72.png Icon-72@2x.png brand-mark.png \
  Icon-60.png Icon-60@2x.png Icon-60@3x.png \
  Icon-76~ipad.png Icon-76@2x~ipad.png Icon-83.5@2x.png \
  IconSets/Classic/Icon-60.png IconSets/Classic/Icon-60@2x.png \
  IconSets/Classic/Icon-60@3x.png IconSets/Classic/Icon-76~ipad.png \
  IconSets/Classic/Icon-76@2x~ipad.png IconSets/Classic/Icon-83.5@2x.png \
  IconSets/Modern/Icon-60.png IconSets/Modern/Icon-60@2x.png \
  IconSets/Modern/Icon-60@3x.png IconSets/Modern/Icon-76~ipad.png \
  IconSets/Modern/Icon-76@2x~ipad.png IconSets/Modern/Icon-83.5@2x.png \
  Default.png Default@2x.png Default-568h@2x.png \
  Default-667h@2x.png Default-736h@3x.png Default-Landscape-736h@3x.png \
  Lucide.ttf ThirdPartyNotices/README.md \
  ThirdPartyNotices/DETA-SURF-LICENSE.txt ThirdPartyNotices/LUCIDE-LICENSE.txt
do
  if [ ! -s "$tmp/Applications/Surf.app/$resource" ]; then
    echo "Missing phone/tablet resource in package: $resource" >&2
    exit 1
  fi
done

verify_png() {
  resource="$1"
  dimensions="$2"
  description="$(file -b "$tmp/Applications/Surf.app/$resource")"
  case "$description" in
    "PNG image data, $dimensions, 8-bit"*) ;;
    *)
      echo "$resource has unexpected image metadata: $description" >&2
      exit 1
      ;;
  esac
}

verify_png Icon.png "57 x 57"
verify_png Icon@2x.png "114 x 114"
verify_png Icon-72.png "72 x 72"
verify_png Icon-72@2x.png "144 x 144"
verify_png Icon-60.png "60 x 60"
verify_png Icon-60@2x.png "120 x 120"
verify_png Icon-60@3x.png "180 x 180"
verify_png Icon-76~ipad.png "76 x 76"
verify_png Icon-76@2x~ipad.png "152 x 152"
verify_png Icon-83.5@2x.png "167 x 167"
verify_png IconSets/Classic/Icon-60.png "60 x 60"
verify_png IconSets/Classic/Icon-60@2x.png "120 x 120"
verify_png IconSets/Classic/Icon-60@3x.png "180 x 180"
verify_png IconSets/Classic/Icon-76~ipad.png "76 x 76"
verify_png IconSets/Classic/Icon-76@2x~ipad.png "152 x 152"
verify_png IconSets/Classic/Icon-83.5@2x.png "167 x 167"
verify_png IconSets/Modern/Icon-60.png "60 x 60"
verify_png IconSets/Modern/Icon-60@2x.png "120 x 120"
verify_png IconSets/Modern/Icon-60@3x.png "180 x 180"
verify_png IconSets/Modern/Icon-76~ipad.png "76 x 76"
verify_png IconSets/Modern/Icon-76@2x~ipad.png "152 x 152"
verify_png IconSets/Modern/Icon-83.5@2x.png "167 x 167"
verify_png brand-mark.png "144 x 144"
verify_png Default.png "320 x 480"
verify_png Default@2x.png "640 x 960"
verify_png Default-568h@2x.png "640 x 1136"
verify_png Default-667h@2x.png "750 x 1334"
verify_png Default-736h@3x.png "1242 x 2208"
verify_png Default-Landscape-736h@3x.png "2208 x 1242"

for resource in Icon.png Icon@2x.png Icon-72.png Icon-72@2x.png; do
  description="$(file -b "$tmp/Applications/Surf.app/$resource")"
  case "$description" in
    *"8-bit/color RGBA,"*) ;;
    *)
      echo "$resource must preserve the transparent iOS 6 plane-break artwork" >&2
      exit 1
      ;;
  esac
done

for resource in IconSets/Classic/Icon-60.png IconSets/Classic/Icon-60@2x.png IconSets/Classic/Icon-60@3x.png IconSets/Classic/Icon-76~ipad.png IconSets/Classic/Icon-76@2x~ipad.png IconSets/Classic/Icon-83.5@2x.png; do
  description="$(file -b "$tmp/Applications/Surf.app/$resource")"
  case "$description" in
    *"8-bit/color RGBA,"*) ;;
    *)
      echo "$resource must preserve the transparent classic artwork" >&2
      exit 1
      ;;
  esac
done

for resource in Icon-60.png Icon-60@2x.png Icon-60@3x.png Icon-76~ipad.png Icon-76@2x~ipad.png Icon-83.5@2x.png; do
  description="$(file -b "$tmp/Applications/Surf.app/$resource")"
  case "$description" in
    *"8-bit/color RGB,"*) ;;
    *)
      echo "$resource must be opaque so iOS 7+ cannot show a black edge" >&2
      exit 1
      ;;
  esac
done

for resource in icon-57.png icon-57@2x.png icon-72.png icon-72@2x.png icon-144.png icon-60.png icon-60@2x.png icon-60@3x.png icon-76.png icon-76@2x.png icon-167.png icon-classic-60.png icon-classic-60@2x.png icon-classic-60@3x.png icon-classic-76.png icon-classic-76@2x.png icon-classic-167.png; do
  if [ -e "$tmp/Applications/Surf.app/$resource" ]; then
    echo "Unqualified source artwork leaked into the application root: $resource" >&2
    exit 1
  fi
done

if [ ! -x "$icon_selector" ]; then
  echo "Missing executable OS-specific icon selector" >&2
  exit 1
fi

SURF_APP_DIR="$tmp/Applications/Surf.app" SURF_SYSTEM_VERSION=6.1.3 "$icon_selector"
for resource in Icon-60.png Icon-60@2x.png Icon-60@3x.png Icon-76~ipad.png Icon-76@2x~ipad.png Icon-83.5@2x.png; do
  if ! cmp -s "$tmp/Applications/Surf.app/$resource" "$tmp/Applications/Surf.app/IconSets/Classic/$resource"; then
    echo "iOS 6 selector did not activate classic $resource" >&2
    exit 1
  fi
done

SURF_APP_DIR="$tmp/Applications/Surf.app" SURF_SYSTEM_VERSION=8.4 "$icon_selector"
for resource in Icon-60.png Icon-60@2x.png Icon-60@3x.png Icon-76~ipad.png Icon-76@2x~ipad.png Icon-83.5@2x.png; do
  if ! cmp -s "$tmp/Applications/Surf.app/$resource" "$tmp/Applications/Surf.app/IconSets/Modern/$resource"; then
    echo "iOS 7+ selector did not activate modern $resource" >&2
    exit 1
  fi
done

control_dir="$tmp/DEBIAN"
dpkg-deb -e "$package" "$control_dir"
if [ ! -x "$control_dir/postinst" ] || \
   ! grep -q '/usr/libexec/surf-select-icons' "$control_dir/postinst" || \
   ! grep -q 'com.apple.IconsCache/space.seg6.surf' "$control_dir/postinst"; then
  echo "Package postinst does not select and invalidate Surf's OS-specific icon" >&2
  exit 1
fi

# Keep the registered 76-pixel iPad candidate unambiguous. Classic SpringBoard
# resolves the ~ipad form too; the selector supplies its OS-appropriate pixels.
for resource in Icon-76.png Icon-76@2x.png icon-76.png icon-76@2x.png; do
  if [ -e "$tmp/Applications/Surf.app/$resource" ]; then
    echo "Unsuffixed modern iPad icon would override the iOS 6 artwork: $resource" >&2
    exit 1
  fi
done

font_description="$(file -b "$tmp/Applications/Surf.app/Lucide.ttf")"
case "$font_description" in
  "TrueType Font data"*) ;;
  *)
    echo "Lucide.ttf is not a TrueType font: $font_description" >&2
    exit 1
    ;;
esac

updater_mode="$(
  dpkg-deb -c "$package" |
    awk '!found && ($NF == "usr/libexec/surf-update-v2" || $NF == "./usr/libexec/surf-update-v2") {
      mode = $1
      found = 1
    }
    END {
      if (found) print mode
    }'
)"
case "$updater_mode" in
  -rwsr-xr-x) ;;
  *)
    echo "Updater mode is $updater_mode, expected -rwsr-xr-x" >&2
    exit 1
    ;;
esac

echo "Verified package: $(basename "$package")"
