# Native build

You can get released iOS packages from GitHub Releases. Building from source
gives you a universal rootful `.deb`, not an IPA.

The app links the C99 library under `client/core`, so run its host tests before
you reach for the iOS toolchain.

```sh
make client-core-test
make client-core-sanitize
```

## SDK

The build uses the iOS 8.0 SDK because it has framework stubs for both armv7
and arm64. That doesn't set the minimum runtime version.

The SDK lives outside Git, at:

```text
native/buildenv/sdk/iPhoneOS8.0.sdk
```

Download it with:

```sh
native/buildenv/fetch-sdk.sh
```

It downloads this archive by default:

```text
https://github.com/GrowtopiaJaw/iPhoneOS-SDK/releases/download/v1.0/iPhoneOS8.0.sdk.zip
```

The archive's SHA 256 is:

```text
5e770b202937ca31b8547aa4dbef7543e3aa261f6b744012745604588e927b05
```

If you use a mirror, you have to set both `SDK_URL` and `SDK_SHA256`.

## Build environment

```sh
docker build -t surf-buildenv native/buildenv
docker run --rm -v "$PWD:/src" surf-buildenv bash -c \
  'make -C /src/client/ios clean package DEBUG=0 && bash /src/client/ios/verify-package.sh'
```

The image pins Theos commit `16362d3aa83a0acd56df4493d575d34306d42478`, iOS
toolchain release `test-210562a` and libplist 2.3.0, and it checks every
download against its SHA 256.

The root target runs the same build:

```sh
make native-package
```

On Windows, you can fetch the SDK with Git Bash and run the build with Docker Desktop.

```powershell
& 'C:\Program Files\Git\bin\bash.exe' native/buildenv/fetch-sdk.sh
docker build -t surf-buildenv native/buildenv
$repoMount = "${PWD}:/src"
docker run --rm --network host -v $repoMount surf-buildenv bash -c `
  'make -C /src/client/ios clean package DEBUG=0 && bash /src/client/ios/verify-package.sh'
```

The build generates `client/ios/control` and `client/ios/Resources/Info.plist`
from `VERSION` and `COMPATIBILITY_VERSION`, so don't edit those two files by
hand.

Give every release or hardware test build a new `VERSION`. The build works out
`CFBundleVersion` from it. A Debian suffix such as `-2` marks another build of
the same package and doesn't change Surf's app version.

`COMPATIBILITY_VERSION` only changes when old clients and backends can't work
together safely. A backend has to embed the exact package that was built for its
version and compatibility generation.

## Package targets

| Slice | Hardware | Minimum iOS | Supported range |
| --- | --- | ---: | --- |
| `armv7` | 32 bit iPhone, iPod touch, and iPad | 6.0 | iOS 6 and later where hardware permits |
| `arm64` | 64 bit iPhone, iPod touch, and iPad | 7.0 | iOS 7 through iOS 14 |

The Debian architecture is `iphoneos-arm`. It's a rootful package that contains
both slices, and the app declares iPhone, iPod and iPad support.

The stream size comes from the device bounds, the browser controls, the
orientation and fullscreen. There's no table of resolutions per model. Phone
and tablet layouts are chosen with `UI_USER_INTERFACE_IDIOM()`.

Phone tab previews come from the last decoded frame. One is captured when the
Tabs view opens or the active tab changes. They're kept in a 12 entry cache and
dropped on memory warnings.

Packages from 0.6.0 onward install `/usr/libexec/surf-update-v2`, which
verifies and installs a compatible package offered by the backend. Older
packages need one manual update before this works.

The package is written to:

```text
client/ios/packages/
```

The path of the latest package is recorded in:

```text
client/ios/.theos/last_package
```

## Verify

The verifier checks the package metadata, both architectures, the minimum OS
versions, the device families, the icons, the Lucide font, the third party
notices, and the ownership and mode of the update helper.

```sh
docker run --rm -v "$PWD:/src" surf-buildenv bash /src/client/ios/verify-package.sh
docker run --rm -v "$PWD:/src" surf-buildenv bash -c \
  'dpkg-deb -c /src/client/ios/packages/*.deb'
```

Before a release, check these on real hardware:

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

The viewport logs should match the even sized page surface, and rotation and
fullscreen should settle on one encoder generation without reconnecting.

To build a release backend with the package you just made:

```sh
client_deb="$(cat client/ios/.theos/last_package)"
make surf-binary CLIENT_DEB="client/ios/${client_deb#./}"
```
