# Changelog

This file records the user-visible changes in every Surf release.

## 0.8.4 - 2026-07-30

- Restored Chromium tab audio on Linux while keeping captured tabs inaudible on
  the host before, during, and after an iPad connection.
- Corrected portrait and landscape tab-capture geometry, removing the black
  bars and transposed frames seen after rotation.
- Made audio-only capture transition reliably into video capture and survive
  reconnects, orientation changes, and encoder restarts.
- Prevented multiple backends from sharing one browser profile, rejected stale
  DevTools endpoints, and strengthened Chromium process-tree cleanup on normal
  shutdown and unexpected Linux or Windows exits.

## 0.8.3 - 2026-07-30

- Stabilized tab-capture delivery with a latest-frame, source-aware 30 FPS
  pacing loop and improved motion diagnostics.
- Completed native Intel and Apple Silicon macOS packaging.

## 0.8.2 - 2026-07-30

- Preserved browser text and edge detail through tab capture and WebCodecs for
  a sharper native-size image.

## 0.8.1 - 2026-07-30

- Replaced the screenshot/FFmpeg video route with Chromium
  `tabCapture`/WebCodecs H.264 delivery.
- Moved audio to the same active-tab capture source on Linux, Windows, and
  macOS.
- Simplified runtime and package ownership around the unified capture path.
- Drove capture from the client refresh cadence for lower interaction latency.

## 0.8.0 - 2026-07-30

- Added Windows process-loopback audio capture.

## 0.7.0 - 2026-07-29

- Shipped the native browser UI with bookmarks, history, downloads, settings,
  media controls, mobile-site behavior, and a compact tab/omnibox layout.
- Added low-latency touch browsing, native diagnostics, content blocking, and
  stable 30 FPS H.264 defaults.

## 0.6.7 - 2026-07-29

- Reduced H.264 latency through faster frame ingestion, automatic FFmpeg
  threading, and pipelined VideoToolbox decoding.
- Added optional low-latency NVENC support and documented the original-iPad
  performance profile.
- Recovered native updates left partially installed.

## 0.6.6 - 2026-07-29

- Offered newer compatible iPad clients when the wire protocol still matches.

## 0.6.5 - 2026-07-29

- Generated native-client updates correctly from Linux ARM64 releases.

## 0.6.4 - 2026-07-28

- Added Linux ARM64 release packages and Docker deployment support.

## 0.6.3 - 2026-07-28

- Isolated the native updater from replacing itself during installation.
- Removed duplicate CI builds so the release workflow owns packaging.

## 0.6.2 - 2026-07-28

- Verified the native package installation result before reporting an update
  as successful.

## 0.6.1 - 2026-07-28

- Added the native performance overlay and client diagnostics used to measure
  decode, presentation, frame age, and interaction latency.

## 0.6.0 - 2026-07-28

- Shipped one self-updating `surf` executable for the desktop tray and
  standalone backend.
- Added verified desktop and iPad update manifests and native packages.
- Added managed Chromium resolution and complete Linux, Windows, and macOS
  release packaging.
- Hardened release validation, Windows resources, and macOS disk-image
  creation.

## 0.5.0 - 2026-07-27

- Unified the desktop supervisor and backend into one `surf` executable.
- Made tray mode the default while retaining `surf serve` for headless use.
- Added cross-platform desktop packages and reliable health/version metadata.

## 0.4.0 - 2026-07-27

- Consolidated Surf around one observable, H.264-only client pipeline.
- Added display-link presentation pacing and lower-latency CDP-to-H.264
  streaming.
- Introduced cross-platform Chromium process management and Windows capture
  support.

## 0.3.0 - 2026-07-27

- Added a standalone Linux runtime with browser discovery, diagnostics, and
  host-audio support.
- Made sharp H.264 streaming the default and removed manual stream presets.
- Decomposed tab, input, CDP, and media ownership for ordered interaction and
  stale-work protection.
- Added editable saved-server settings to the native client.

## 0.2.0 - 2026-07-26

- Required password authentication and short-lived WebSocket tickets.
- Restricted diagnostics, bounded uploads, and hardened HTTP server limits.
- Pinned native build tooling for reproducible packages.

## 0.1.0 - 2026-07-26

- Initial Surf backend, legacy-iOS native client, container image, and release
  workflow.
