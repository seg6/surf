# iOS client

This is a native UIKit client for jailbroken iPhone, iPod touch and iPad. The
package ID is `space.seg6.surf` and the architecture is `iphoneos-arm`, a fat
binary with armv7 and arm64 slices. It only supports rootful jailbreaks.

For installation steps, see [Install on iOS](../guide/install-ios.md).

## Support

| Slice | Minimum OS | Hardware |
| --- | --- | --- |
| `armv7` | iOS 6.0 | 32-bit iPhone, iPod touch, and iPad |
| `arm64` | iOS 7.0 | A7 and newer through iOS 14 |

The current target ends at iOS and iPadOS 14.8.1. iOS 15 and later, rootless
packages, armv6, iOS 5 and the original iPad aren't supported.

An A5 or newer is recommended. The 256 MB iPhone 3GS and iPod touch 4 can
install the package, but they remain experimental.

The full flow has been tested on an original iPad mini running iOS 6.1.3. That
covers pairing, saved servers, touch and keyboard input, video, audio,
rotation and fullscreen. The other rows in the tables below are build targets,
and not every combination has been run on hardware.

### 32-bit devices

| Models | Surf-compatible OS range | Status |
| --- | --- | --- |
| iPhone 3GS | 6.0-6.1.6 | Experimental |
| iPhone 4 | 6.0-7.1.2 | Legacy candidate |
| iPhone 4s | 6.0-9.3.6 | Legacy candidate |
| iPhone 5 | 6.0-10.3.4 | Good candidate |
| iPhone 5c | 7.0-10.3.3 | Good candidate |
| iPod touch 4 | 6.0-6.1.6 | Experimental |
| iPod touch 5 | 6.0-9.3.5 | Legacy candidate |
| iPad 2 and iPad 3 | 6.0-9.3.5 (9.3.6 cellular) | Legacy candidates |
| iPad 4 | 6.0-10.3.3 (10.3.4 cellular) | Good candidate |
| iPad mini 1 | 6.0-9.3.5 (9.3.6 cellular) | Verified on iOS 6.1.3 |

### 64-bit devices

| Models | Surf-compatible OS range |
| --- | --- |
| iPhone 5s | 7.0-12.5.7 |
| iPhone 6 and 6 Plus | 8.0-12.5.7 |
| iPad Air 1 and iPad mini 2 | 7.0-12.5.7 |
| iPad mini 3 | 8.0-12.5.7 |
| iPod touch 6 | 8.4-12.5.7 |
| iPhone 6s through 12, including SE 1 and SE 2 | Launch OS-14.8.1 |
| iPad Air 2-4, mini 4-5, iPad 5-8 | Launch OS-14.8.1 |
| iPad Pro through 2020 | Launch OS-14.8.1 |
| iPod touch 7 | 12.3-14.8.1 |

## UI

The layout and color roles are the same from iOS 6 through 14. The light theme
has pale surfaces, dark text and blue controls. The dark theme uses neutral
dark surfaces with the same blue. The font is the system font.

On a phone, the bottom bar holds Back, Forward, Share, Tabs and More. The Library
can be reached from the new tab page and from More.

On an iPad, tabs and browser actions sit in a bar at the top or bottom, and the
Bottom Browser Bar setting chooses which. The address field expands when you
edit it. Tabs scroll horizontally, and the active tab has a blue outline.

More contains Library, Reader, Find, Media, Fullscreen and Settings. Share is
a separate button because it opens the system share sheet. Surf's own popovers
follow the app's appearance, while the system share, mail, photo and AirDrop
views keep whatever UIKit appearance the OS gives them.

### Icons

The interface icons come from the bundled Lucide font and are configured in
`RBTheme`. The app icon comes from the pinned Deta Surf artwork under
`client/ios/Artwork`. iOS 6 uses the original transparent 57 and 72 point
images. iOS 7 and later use opaque 60, 76 and 83.5 point images, so the system
mask doesn't expose dark outer pixels. The new tab and connection views use
`brand-mark.png`. Checksums and licenses are in `THIRD_PARTY_NOTICES.md`. The
derived images are rebuilt with ImageMagick 7:

```sh
client/ios/Artwork/generate-assets.sh
```

### Settings and diagnostics

Settings is a single grouped list on both phone and tablet. It shows server,
appearance, browsing, performance, privacy and version information.

Dark Mode updates the native controls and sets `prefers-color-scheme: dark` in
current and future Chromium tabs. Websites without a dark style are left
unchanged.

Diagnostics can be hidden, compact or expanded. Compact shows health and the
Surf version. Expanded adds latency, video, network, pipeline and adaptive
profile details. The overlay doesn't change the stream size, and an idle
static page isn't marked unhealthy just because its displayed frame rate is
zero.

## Video on device

When the required runtime methods are present, Surf sends compressed H.264
samples to `AVSampleBufferDisplayLayer`. The queue works on iOS 6.1 and later,
even though its public API only arrived in iOS 8.

On iOS 6 and 7 the later status APIs aren't available, so Surf watches queue
pressure itself. A stalled queue is replaced at an IDR boundary, and if that
keeps failing Surf falls back to VideoToolbox, NV12 surfaces and OpenGL ES.

Both paths share the same Annex B and AVCC parsing. For the system path,
diagnostics report queue acceptance and recovery. For the fallback path they
report decode and presentation timing.

## Platform services

| Concern | Implementation |
| --- | --- |
| Core bridge | `RBCoreBridge` |
| Transport | `RBSecureHTTPClient`, `RBSocket`, `RBTunnelPipe` |
| Identity | `RBDeviceIdentity` (Security and Keychain) |
| Video | `RBVideoDecoder`, `RBSampleBufferRenderer`, `RBStreamView` |
| Audio | `RBAudioPlayer` (AudioQueue) |

## Release checks

Every release is checked on phone and tablet layouts, with rotation, tabs, new
tab favorites, More, Share, Reader, pairing, Library, Settings and both
appearance modes. Media checks include continuous motion, keyboard changes, tab
changes, background and foreground transitions, rotation and IDR recovery.
Keyboard and diagnostic changes must not resize the remote page.

## Build

See [Native iOS build](../develop/native-ios.md).
