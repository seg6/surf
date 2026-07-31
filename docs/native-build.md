# Native Build

Most users should download the universal rootful `.deb` from GitHub Releases.
Build the native client only if you are changing the app or packaging your own
release. Surf does not produce an IPA.

## SDK

The iOS 8.0 SDK is downloaded into this ignored local path:

```text
native/buildenv/sdk/iPhoneOS8.0.sdk
```

Fetch it with:

```sh
native/buildenv/fetch-sdk.sh
```

The default source is:

```text
https://github.com/GrowtopiaJaw/iPhoneOS-SDK/releases/download/v1.0/iPhoneOS8.0.sdk.zip
```

The default archive must match this SHA-256:

```text
5e770b202937ca31b8547aa4dbef7543e3aa261f6b744012745604588e927b05
```

Set `SDK_URL` and `SDK_SHA256` together to use a mirror of the same archive.
The iOS 8.0 SDK is used because its framework and library stubs contain both
armv7 and arm64. The SDK version is a build input, not the minimum runtime
version.

## Build Environment

```sh
docker build -t surf-buildenv native/buildenv
docker run --rm -v "$PWD:/src" surf-buildenv bash -c \
  'make -C /src/native/client clean package DEBUG=0 && bash /src/native/client/verify-package.sh'
```

The build environment pins Theos to commit
`16362d3aa83a0acd56df4493d575d34306d42478` and the iOS toolchain to release
`test-210562a`, with SHA-256 verification for each host-architecture archive.
It also builds libplist 2.3.0 at a pinned commit so Theos's `plistutil` has the
ABI it expects on current Debian hosts; this keeps clean package builds and
binary-plist verification reproducible.

Or use the root make target:

```sh
make native-package
```

From Windows PowerShell, Git Bash can fetch the SDK and Docker Desktop can run
the build directly without installing GNU Make on Windows:

```powershell
& 'C:\Program Files\Git\bin\bash.exe' native/buildenv/fetch-sdk.sh
docker build -t surf-buildenv native/buildenv
$repoMount = "${PWD}:/src"
docker run --rm --network host -v $repoMount surf-buildenv bash -c `
  'make -C /src/native/client clean package DEBUG=0 && bash /src/native/client/verify-package.sh'
```

`make native-package` renders generated native metadata from `VERSION`, compiles
the native protocol gate from `PROTOCOL_VERSION`, and verifies the resulting
package. Do not edit generated `native/client/control` or
`native/client/Resources/Info.plist` by hand.

## Compatibility

The package contains universal `armv7` and `arm64` binaries:

| Slice   | Intended devices                    | Minimum iOS | Supported range                            |
| ------- | ----------------------------------- | ----------- | ------------------------------------------ |
| `armv7` | 32-bit iPhone, iPod touch, and iPad | 6.0         | iOS 6 onward, as permitted by the hardware |
| `arm64` | 64-bit iPhone, iPod touch, and iPad | 7.0         | iOS 7 through iOS 14                       |

The Debian package architecture remains `iphoneos-arm`, the standard identifier
for a rootful jailbreak package containing both slices. It is not a rootless
`iphoneos-arm64` package. The bundle declares device families 1 and 2, so the
same package installs on iPhone/iPod and iPad. Classic phone launch images opt
into native 3.5-, 4-, 4.7-, and 5.5-inch viewports; the client sends the
resulting live viewport and every rotation to the backend.

The runtime layout is selected with `UI_USER_INTERFACE_IDIOM()`. Phone builds
use a bottom toolbar and a full-screen Pages controller; tablet builds use a
top toolbar and persistent tab row. Width affects spacing and rotation only,
so an iPhone-compatibility installation on an iPad exercises the real phone
path. iOS 6 receives Surf's procedural skeuomorphic gradients, shadows, and
icons; iOS 7–14 use the flatter system-era palette without copied Apple art.

Phone tab previews are snapshots of the last decoded frame. They are captured
only when Pages opens or the active phone tab is left, retained in a 12-entry
LRU cache, and purged on memory warnings. This keeps preview work off the normal
30 FPS presentation path.

Packages from version 0.6.0 onward install the small root-owned
`/usr/libexec/surf-update-v2` helper. When a release backend reports an incompatible
older client, the app can download the backend's embedded matching `.deb`,
verify its size and SHA-256, validate its package identity, and install it.
Devices running a package older than 0.6.0 need one final manual installation
to bootstrap that helper.

The package is written to:

```text
native/client/packages/
```

Theos records the exact package produced by the latest build in:

```text
native/client/.theos/last_package
```

## Verify

The verifier extracts that exact `.deb` and checks the package identifier,
`iphoneos-arm` metadata, both architectures and minimum iOS versions in the app
and updater, both device families, required phone/tablet resources, and the
updater's root/setuid mode:

```sh
docker run --rm -v "$PWD:/src" surf-buildenv bash /src/native/client/verify-package.sh
docker run --rm -v "$PWD:/src" surf-buildenv bash -c \
  'dpkg-deb -c /src/native/client/packages/*.deb'
```

For device acceptance, verify both idioms rather than resizing one layout:

- On iPad, test the top toolbar, persistent tabs and overflow, anchored Share,
  Bookmarks, and More popovers, rotation, fullscreen, video, audio, and input.
- On an iPhone/iPod or phone-only compatibility package, test the six-button
  bottom toolbar, Pages previews and cache behavior, full-screen Library and
  activities, portrait/landscape transitions, and Surf Settings discovery.
- Watch the native diagnostics/log during every chrome transition. The
  reported viewport must equal the remaining even-sized stream surface, and
  rotation/fullscreen changes must recover automatically without Retry Video.

To build the unified release binary with a matching client package embedded:

```sh
client_deb="$(cat native/client/.theos/last_package)"
make surf-binary CLIENT_DEB="native/client/${client_deb#./}"
```
