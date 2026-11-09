# Native build

Released iOS packages are available from GitHub Releases. Source builds produce
a universal rootful `.deb`, not an IPA.

The app links the C99 library under `client/core`. Run its host tests before
using the iOS toolchain.

```sh
make client-core-test
make client-core-sanitize
```

## SDK

The build uses the iOS 8.0 SDK because it contains armv7 and arm64 framework
stubs. This does not set the minimum runtime version.

The SDK is stored outside Git at:

```text
native/buildenv/sdk/iPhoneOS8.0.sdk
```

Download it with:

```sh
native/buildenv/fetch-sdk.sh
```

The default archive is:

```text
https://github.com/GrowtopiaJaw/iPhoneOS-SDK/releases/download/v1.0/iPhoneOS8.0.sdk.zip
```

Its SHA 256 is:

```text
5e770b202937ca31b8547aa4dbef7543e3aa261f6b744012745604588e927b05
```

A mirror requires both `SDK_URL` and `SDK_SHA256`.

## Build environment

```sh
docker build -t surf-buildenv native/buildenv
docker run --rm -v "$PWD:/src" surf-buildenv bash -c \
  'make -C /src/client/ios clean package DEBUG=0 && bash /src/client/ios/verify-package.sh'
```

The image pins Theos commit
`16362d3aa83a0acd56df4493d575d34306d42478`, iOS toolchain release
`test-210562a`, and libplist 2.3.0. Downloads are checked by SHA 256.

The root target runs the same build.

```sh
make native-package
```

On Windows, Git Bash can fetch the SDK and Docker Desktop can run the build.

```powershell
& 'C:\Program Files\Git\bin\bash.exe' native/buildenv/fetch-sdk.sh
docker build -t surf-buildenv native/buildenv
$repoMount = "${PWD}:/src"
docker run --rm --network host -v $repoMount surf-buildenv bash -c `
  'make -C /src/client/ios clean package DEBUG=0 && bash /src/client/ios/verify-package.sh'
```

The build generates `client/ios/control` and
`client/ios/Resources/Info.plist` from `VERSION` and
`COMPATIBILITY_VERSION`. Generated files should not be edited by hand.

Every release or hardware test build gets a new `VERSION`. The build derives
`CFBundleVersion`. A Debian suffix such as `-2` marks another package build
but does not change Surf's app version.

`COMPATIBILITY_VERSION` changes only when old clients and backends cannot
interoperate safely. A backend must embed the exact package built for its
version and compatibility generation.

## Package targets

| Slice | Hardware | Minimum iOS | Supported range |
| --- | --- | ---: | --- |
| `armv7` | 32 bit iPhone, iPod touch, and iPad | 6.0 | iOS 6 and later where hardware permits |
| `arm64` | 64 bit iPhone, iPod touch, and iPad | 7.0 | iOS 7 through iOS 14 |

The Debian architecture is `iphoneos-arm`. It is a rootful package containing
both slices. The app declares iPhone, iPod, and iPad support.

Device bounds, browser controls, orientation, and fullscreen determine the
stream size. There is no model resolution table. Phone and tablet layouts are
selected with `UI_USER_INTERFACE_IDIOM()`.

Phone tab previews come from the last decoded frame. They are captured when the
Tabs view opens or the active tab changes, kept in a 12 entry cache, and removed
on memory warnings.

Packages from 0.6.0 onward install `/usr/libexec/surf-update-v2`. It verifies
and installs a compatible package offered by the backend. Older packages need
one manual update before this path is available.

The package is written to:

```text
client/ios/packages/
```

The latest package path is recorded in:

```text
client/ios/.theos/last_package
```

## Verify

The verifier checks package metadata, both architectures, minimum OS versions,
device families, icons, the Lucide font, third party notices, and the ownership
and mode of the update helper.

```sh
docker run --rm -v "$PWD:/src" surf-buildenv bash /src/client/ios/verify-package.sh
docker run --rm -v "$PWD:/src" surf-buildenv bash -c \
  'dpkg-deb -c /src/client/ios/packages/*.deb'
```

Hardware acceptance covers:

1. Both top and bottom iPad browser bars
2. Phone browser controls and tab previews
3. Rotation and both Surf and page fullscreen
4. Video, audio, keyboard, touch, pinch, fling, and text composition
5. Share, Library, Reader, Find, Media, Settings, and dialogs
6. Light and dark appearance
7. Compact phones, modern phones, iPads, and iPad Pro sizes
8. Diagnostics without changes to the stream viewport
9. Recovery from a forced IDR and foreground changes
10. Correct icons on iOS 6 and iOS 7 or later

Viewport logs must match the even sized page surface. Rotation and fullscreen
must settle on one encoder generation without reconnecting.

Build a release backend with the recorded package.

```sh
client_deb="$(cat client/ios/.theos/last_package)"
make surf-binary CLIENT_DEB="client/ios/${client_deb#./}"
```
