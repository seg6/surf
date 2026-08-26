# Native UI system

Surf's native client uses one deliberately compact visual language across
iOS 6–14. The design is called **Oceanic Precision**: quiet Foam surfaces,
Deep Tide text and navigation, a restrained Surf Blue accent, and Sea Glass
for focus and progress. Display and body copy use the device's native system
font family at deliberately different weights, avoiding family mismatches on
older releases while following each system's own typography metrics.

## Browser chrome

- The omnibox is the primary control, with a 36-point white field, clear focus
  state, readable security status, and a slim horizontal loading line.
- Phone chrome has five evenly spaced actions: Back, Forward, Share, Tabs, and
  More. Library remains immediately available on every new-tab page and as the
  first action in More.
- iPad chrome keeps persistent tabs and an anchored action layout. Active tabs
  read as white cards above a light ocean strip rather than inherited system
  chrome. New Tab is a direct plus glyph in that strip, not a nested circular
  badge.
- More is a Surf-owned six-tile surface for Library, Reader, Find, Media,
  Fullscreen, and Settings. It never uses the system share controller, so it
  cannot accidentally expose AirDrop. Share remains a separate action and is
  the only entry point to system destinations.

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

Settings is a task dashboard rather than a themed system preferences table.
It leads with the selected server and the frequently changed Mobile Sites and
Performance Monitor controls, then routes Browsing, Diagnostics, server data, and
About into focused destinations. Compact devices use one column; wide form
sheets use two without changing the information hierarchy.

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

The inspector overlays the existing stream instead of resizing it. Health
classification uses fresh pipeline deltas and never treats an idle static page
as unhealthy solely because its presentation FPS is zero.

## Acceptance

Every release should exercise both iPad and phone idioms, including rotation,
new-tab favorites, tab closing/switching, More dismissal and actions, Share,
Reader night mode, QR pairing, Library, and Settings. Package verification must
also pass so missing fonts, malformed PNGs, device-family regressions, or lost
third-party notices fail the build before device testing.

The supported deployment range remains iOS 6–14. Device feedback from one OS
version is treated as a compatibility test case rather than a separate visual
fork: newer selectors stay availability-guarded, geometry remains adaptive,
and the same information hierarchy is retained throughout the range.
