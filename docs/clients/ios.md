# iOS client

Native UIKit client for jailbroken iPhone, iPod touch, and iPad. Package ID
`space.seg6.surf`, architecture `iphoneos-arm` (fat armv7 + arm64), rootful
only.

Install steps: [Install on iOS](../guide/install-ios.md).

## Support

| Slice | Minimum OS | Hardware |
| --- | --- | --- |
| `armv7` | iOS 6.0 | 32-bit iPhone, iPod touch, and iPad |
| `arm64` | iOS 7.0 | A7 and newer through iOS 14 |

Current target ends at iOS / iPadOS 14.8.1. Unsupported: iOS 15+, rootless
packages, armv6, iOS 5, original iPad.

A5 or newer is recommended. The 256 MB iPhone 3GS and iPod touch 4 can install
the package but remain experimental.

Tested end-to-end on an original iPad mini (iOS 6.1.3): pairing, saved servers,
touch/keyboard, video, audio, rotation, fullscreen. Other rows in the tables
below are build targets; not every combo has been run on hardware.

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
| iPhone 6 / 6 Plus | 8.0-12.5.7 |
| iPad Air 1 / iPad mini 2 | 7.0-12.5.7 |
| iPad mini 3 | 8.0-12.5.7 |
| iPod touch 6 | 8.4-12.5.7 |
| iPhone 6s-12 (incl. SE 1 / SE 2) | Launch OS-14.8.1 |
| iPad Air 2-4, mini 4-5, iPad 5-8 | Launch OS-14.8.1 |
| iPad Pro through 2020 | Launch OS-14.8.1 |
| iPod touch 7 | 12.3-14.8.1 |

## UI

Same layout and color roles from iOS 6 through 14. Light: pale surfaces, dark
text, blue controls. Dark: neutral dark surfaces, same blue. System font.

**Phone:** Back, Forward, Share, Tabs, and More in the bottom bar. Library is
available from the new tab page and More.

**iPad:** Tabs and browser actions in a bar at the top or bottom. The
**Bottom Browser Bar** setting chooses its position. The address field expands
for editing. Tabs scroll horizontally; the active tab has a blue outline.

More contains Library, Reader, Find, Media, Fullscreen, and Settings. Share is
separate because it opens the system share sheet. Surf popovers use the app
appearance. System share, mail, photo, and AirDrop views keep the UIKit
appearance supplied by the OS.

### Icons

Interface icons come from the bundled Lucide font and are configured in
`RBTheme`. The app icon comes from the pinned Deta Surf artwork under
`client/ios/Artwork`. iOS 6 uses the original transparent 57 and 72 point
images. iOS 7 and later use opaque 60, 76, and 83.5 point images so the system
mask does not expose dark outer pixels. New tab and connection views use
`brand-mark.png`. Checksums and licenses are in `THIRD_PARTY_NOTICES.md`.
Rebuild derivatives with ImageMagick 7:

```sh
client/ios/Artwork/generate-assets.sh
```

### Settings and diagnostics

Settings uses one grouped list on both phone and tablet. It shows server,
appearance, browsing, performance, privacy, and version information.

Dark Mode updates the native controls and sets `prefers-color-scheme: dark` in
current and future Chromium tabs. Websites without a dark style remain
unchanged.

Diagnostics can be hidden, compact, or expanded. Compact shows health and the
Surf version. Expanded adds latency, video, network, pipeline, and adaptive
profile details. The overlay does not change the stream size. An idle static
page is not marked unhealthy because its displayed frame rate is zero.

## Video on device

Surf sends compressed H.264 samples to `AVSampleBufferDisplayLayer` when the
required runtime methods are present. The queue works on iOS 6.1 and later,
although its public API arrived in iOS 8.

On iOS 6 and 7, Surf watches queue pressure without the later status APIs. A
stalled queue is replaced at an IDR boundary. Repeated failure falls back to
VideoToolbox, NV12 surfaces, and OpenGL ES.

Both paths use the same Annex B and AVCC parsing. Diagnostics report queue
acceptance and recovery for the system path, or decode and presentation timing
for the fallback path.

## Platform services

| Concern | Implementation |
| --- | --- |
| Core bridge | `RBCoreBridge` |
| Transport | `RBSecureHTTPClient`, `RBSocket`, `RBTunnelPipe` |
| Identity | `RBDeviceIdentity` (Security / Keychain) |
| Video | `RBVideoDecoder`, `RBSampleBufferRenderer`, `RBStreamView` |
| Audio | `RBAudioPlayer` (AudioQueue) |

## Release checks

Every release covers phone and tablet layouts, rotation, tabs, new tab
favorites, More, Share, Reader, pairing, Library, Settings, and both appearance
modes. Media checks include continuous motion, keyboard changes, tab changes,
background and foreground transitions, rotation, and IDR recovery. Keyboard and
diagnostic changes must not resize the remote page.

## Build

[Native iOS build](../develop/native-ios.md).
