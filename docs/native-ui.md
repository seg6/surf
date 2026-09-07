# Native UI

The iOS client uses the same layout and color roles from iOS 6 through iOS 14.
Light mode uses pale surfaces, dark text, and blue controls. Dark mode uses
neutral dark surfaces and the same blue controls. Both use the system font
available on the device.

## Browser controls

Phone layouts have Back, Forward, Share, Tabs, and More in the bottom bar.
Library is available from the new tab page and More.

iPad layouts keep tabs and browser actions in a bar at the top or bottom. The
**Bottom Browser Bar** setting chooses its position. The address field expands
for editing. Tabs scroll horizontally, and the active tab has a blue outline.

More contains Library, Reader, Find, Media, Fullscreen, and Settings. Share is
separate because it opens the system share sheet.

Surf popovers use the app appearance. System share, mail, photo, and AirDrop
views keep the UIKit appearance supplied by the OS.

## Icons

Interface icons come from the bundled Lucide font and are configured in
`RBTheme`.

The app icon comes from the pinned Deta Surf artwork under
`client/ios/Artwork`. iOS 6 uses the original transparent 57 and 72 point
images. iOS 7 and later use opaque 60, 76, and 83.5 point images so the system
mask does not expose dark outer pixels. New tab and connection views use
`brand-mark.png`.

Checksums and licenses are in `THIRD_PARTY_NOTICES.md`. Derived images can be
rebuilt with ImageMagick 7.

```sh
client/ios/Artwork/generate-assets.sh
```

## Settings and diagnostics

Settings uses one grouped list on both phone and tablet. It shows server,
appearance, browsing, performance, privacy, and version information.

Dark Mode updates the native controls and sets
`prefers-color-scheme: dark` in current and future Chromium tabs. Websites
without a dark style remain unchanged.

Diagnostics can be hidden, compact, or expanded. The compact view shows health
and the Surf version. The expanded view adds latency, video, network, pipeline,
and adaptive profile details. It overlays the video and does not change the
stream size. An idle static page is not marked unhealthy because its displayed
frame rate is zero.

## Video

Surf sends compressed H.264 samples to `AVSampleBufferDisplayLayer` when the
required runtime methods are present. The queue works on iOS 6.1 and later,
although its public API arrived in iOS 8.

On iOS 6 and 7, Surf watches queue pressure without the later status APIs. A
stalled queue is replaced at an IDR boundary. Repeated failure falls back to
VideoToolbox, NV12 surfaces, and OpenGL ES.

Both paths use the same Annex B and AVCC parsing. Diagnostics report queue
acceptance and recovery for the system path, or decode and presentation timing
for the fallback path.

## Release checks

Every release covers phone and tablet layouts, rotation, tabs, new tab
favorites, More, Share, Reader, pairing, Library, Settings, and both appearance
modes.

Media checks include continuous motion, keyboard changes, tab changes,
background and foreground transitions, rotation, and IDR recovery. Keyboard and
diagnostic changes must not resize the remote page.
