# Native UI system

Surf's native client uses one deliberately compact visual language across
iOS 6–14. The light appearance uses quiet Foam surfaces, Deep Tide text and
navigation, a restrained Surf Blue accent, and Sea Glass for focus and
progress. Its explicit dark appearance uses neutral Carbon and Graphite
surfaces, reserving Surf Blue for interactive controls instead of tinting the
entire shell navy; it does not depend on newer system trait APIs. Display and body copy
use the device's native system font family at deliberately different weights,
avoiding family mismatches on older releases while following each system's own
typography metrics.

## Browser chrome

- The omnibox is the primary control, with a 36-point white field, clear focus
  state, readable security status, and a slim horizontal loading line.
- Phone chrome has five evenly spaced actions: Back, Forward, Share, Tabs, and
  More. Library remains immediately available on every new-tab page and as the
  first action in More.
- iPad chrome keeps persistent tabs and an anchored action layout at the top or
  bottom edge, controlled by the persistent Bottom Browser Bar setting. The
  compact address field remains the primary rounded control, while scrollable tab cells
  sit directly on the transparent browser rail without an enclosing box. Only
  the active tab receives a fully outlined Surf-tinted selection—never a colored
  cap or raised desktop-style card. New Tab remains a direct, unboxed plus glyph
  beside the rail, keeping actions and tab selection visually distinct.
- More is a content-height Surf-owned six-tile surface for Library, Reader,
  Find, Media, Fullscreen, and Settings. It never uses the system share controller, so it
  cannot accidentally expose AirDrop. Share remains a separate action and is
  the only entry point to system destinations.
- Surf-owned popovers use one Carbon outer surface for their body and anchor
  arrow. System-owned Share, AirDrop, mail, and photo panels retain UIKit's
  native light presentation because their legacy icons and labels do not adapt
  reliably to a custom dark background.

## Icons and brand asset

Interface glyphs come from the bundled Lucide icon font instead of ad-hoc
drawing code. Their weight, optical proportions, highlighted state, disabled
state, and accessibility labels are centralized in `RBTheme`.

The application mark is the Deta Surf icon pinned in `native/client/Artwork`.
iOS 6's 57- and 72-point icon slots preserve the original pre-rendered RGBA
composition, where the paper plane visibly escapes the rounded blue tile.
iOS 7 and later select dedicated 60-, 76-, and 83.5-point opaque icons: the
plane remains oversized on a full-bleed field, but transparent outer pixels
cannot turn into a black perimeter under the newer system mask. The new-tab
and connection states use a separate transparent `brand-mark.png`.

The high-resolution production icon and separate plane silhouette are pinned
to the same upstream revision. Source checksums and complete license texts are
recorded in `THIRD_PARTY_NOTICES.md` and shipped in the app bundle.
Run `native/client/Artwork/generate-assets.sh` with ImageMagick 7 to reproduce
every derived PNG from the pinned master.

## Settings and diagnostics

Settings is a readable, single-column grouped list in both iPad form sheets and
narrow iPhone layouts. Server, Appearance, Browsing, Performance, Data &
Privacy, and About controls remain on the main page; descriptions may wrap and
the Surf client version and compatibility generation are always visible. Dark Mode immediately
rethemes the live native interface and tells every current and future Chromium
tab to advertise the standard `prefers-color-scheme: dark` preference. Sites
which do not author a dark appearance are left unchanged.

Diagnostics has three visibility states in practice: hidden, visible as a slim
health instrument, and temporarily expanded to its intrinsic content height.
Its translucent light paper, ink, and rule treatment follows the browser chrome
without completely hiding the remote page beneath it. A colored rail is the
only persistent health signal. The expanded inspector presents one hierarchy with no mode switch:
round trip/response/video readouts, one recent network trace, and a pipeline
breakdown. There is no explanatory status paragraph and no scrolling. Portrait
uses one hierarchy; short landscape viewports place the signal and pipeline
beside each other. The exact Surf client version remains fixed in the compact
readout and expanded header in both arrangements.
The expanded footer also names the active adaptive stream profile so a device
test can distinguish crisp, motion, balanced, and recovery behavior directly.

The inspector overlays the existing stream instead of resizing it. Health
classification uses fresh pipeline deltas and never treats an idle static page
as unhealthy solely because its presentation FPS is zero.

## Video presentation

Surf feeds compressed H.264 samples directly to
`AVSampleBufferDisplayLayer` whenever the required runtime methods exist. The
core queue is present on iOS 6.1 and later even though Apple did not publish
the class until iOS 8. The system owns decode scheduling, decoded IOSurfaces,
display timing, and composition; Surf does not bounce every frame through a
VideoToolbox callback, the UIKit main queue, a display link, and an OpenGL
drawable. The stream view is a stable Core Animation container, so a keyboard
transition can translate or cover it without reallocating the video surface or
rebuilding the encoder generation.

iOS 8 adds status, error, and failure-notification APIs around that queue. On
iOS 6 and 7, Surf instead detects a persistently full compressed cushion,
flushes and replaces the layer at complete IDR boundaries, and automatically
falls back after repeated failures. The fallback uses runtime-resolved
VideoToolbox, NV12 IOSurfaces, and a lazily created OpenGL ES layer. It keeps a
small FIFO, never drops an arbitrary P-frame and then attempts to decode its
dependants, and never uses decode-without-output as a hidden catch-up mode.

Both renderers use the same bounds-checked Annex-B/AVCC and parameter-set
builder. Diagnostics name the active renderer explicitly: the system path
reports accepted compressed frames, display-queue waits, recoveries, and
failures; the legacy path reports submit, callback, and UIKit handoff timing.

## Acceptance

Every release should exercise both iPad and phone idioms, including rotation,
new-tab favorites, tab closing/switching, More dismissal and actions, Share,
Reader night mode, QR pairing, Library, and Settings. Package verification must
also pass so missing fonts, malformed PNGs, device-family regressions, or lost
third-party notices fail the build before device testing.
Media acceptance additionally covers continuous motion at the native surface,
keyboard open/close during that motion, tab switching, foreground/background
resume, rotation, and a forced IDR recovery. A keyboard transition must not
create a new remote viewport or encoder generation.

The supported deployment range remains iOS 6–14. Device feedback from one OS
version is treated as a compatibility test case rather than a separate visual
fork: newer selectors stay availability-guarded, geometry remains adaptive,
and the same information hierarchy is retained throughout the range.
