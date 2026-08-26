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

For every physical-device test iteration, increment `VERSION` before packaging
and increment `CFBundleVersion` in `Resources/Info.plist.in`. Theos package
revisions such as `-2` are useful artifact identifiers, but they do not trigger
Surf's in-app updater: the client and backend compare the app version from
`VERSION`. Rebuild the backend with that exact new `.deb` embedded before
reconnecting the device.

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
There is no per-model resolution table in the client or backend: device bounds,
chrome, orientation, and fullscreen determine the live even-sized surface.
Named sizes in tests are regression examples only.

The runtime layout is selected with `UI_USER_INTERFACE_IDIOM()`. Phone builds
use a five-action bottom toolbar and a full-screen Tabs controller; tablet
builds use a top toolbar and persistent tab row. Width affects spacing and rotation only,
so an iPhone-compatibility installation on an iPad exercises the real phone
path. iOS 6–14 share Surf's Oceanic Precision palette, typography, and
professionally sourced Lucide interface glyphs.

Phone tab previews are snapshots of the last decoded frame. They are captured
only when Tabs opens or the active phone tab is left, retained in a 12-entry
LRU cache, and purged on memory warnings. This keeps preview work off the normal
60 FPS presentation path.

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
and updater, both device families, every registered icon size, RGBA legacy
plane-break icons, opaque iOS 7+ icons, the registered Lucide font, bundled
third-party notices, and the updater's root/setuid mode:

```sh
docker run --rm -v "$PWD:/src" surf-buildenv bash /src/native/client/verify-package.sh
docker run --rm -v "$PWD:/src" surf-buildenv bash -c \
  'dpkg-deb -c /src/native/client/packages/*.deb'
```

For device acceptance, verify both idioms rather than resizing one layout:

- On iPad, test the top toolbar, persistent tabs and overflow, anchored Share,
  Library, and custom More popovers, rotation, Surf fullscreen, page-requested
  fullscreen, video, audio, and input. Fullscreen should expose only Exit.
- On iOS 6, confirm the original paper plane escapes the pre-rendered blue tile.
  On iOS 7 and later, confirm SpringBoard selects the opaque native-size icon
  without a black perimeter or an inset second tile.
- On an iPhone/iPod or phone-only compatibility package, test the five-action
  bottom toolbar, Tabs previews and cache behavior, full-screen Library, Share,
  the custom More drawer, portrait/landscape transitions, and Settings discovery.
- Confirm More never exposes AirDrop or another share target; only Share should
  invoke `UIActivityViewController` and system destinations.
- Watch the native diagnostics/log during every chrome transition. The
  reported viewport must equal the remaining even-sized stream surface, and
  rotation/fullscreen changes must produce one settled encoder generation,
  retain the WebSocket, and recover automatically without Retry Video.
- Expand and collapse the performance inspector while watching viewport logs;
  the panel must overlay the stream without producing a viewport update or a
  new encoder generation.
- Exercise rapid portrait/landscape changes as well as entering and leaving a
  player such as YouTube. Returning from page fullscreen must restore the
  device-specific chrome at the correct viewport without black borders.
- In both Desktop and Mobile Websites modes, verify a normal tap, a long page
  fling, TikTok-style vertical swipe navigation, multi-touch pinch zoom, the
  Speedometer 3.1 **Start Test** button, and text/password keyboard focus inside
  an open shadow root (Reddit login is a representative live check).
- Cover representative compact phone/iPod, modern phone, iPad, and iPad Pro
  surfaces. Every requested even size must remain exact rather than being
  coerced to the dimensions of another device family.

To build the unified release binary with a matching client package embedded:

```sh
client_deb="$(cat native/client/.theos/last_package)"
make surf-binary CLIENT_DEB="native/client/${client_deb#./}"
```
