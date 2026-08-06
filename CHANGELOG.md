# Changelog

This file records the user-visible changes in every Surf release.

## 0.12.1 - 2026-08-06

- Fixed physical scrolling progressively losing fling and inertia until the
  iOS client was restarted, especially around Mobile Websites reloads.
- Remapped lifetime UIKit contact identifiers to dense Chromium-local IDs for
  each active gesture, preserving stable multi-touch identity without exposing
  Chromium to ever-growing IDs that degrade its velocity tracking.
- Stamped injected touch events at backend dispatch time and ended gestures
  directly from the last real UIKit move, preventing transport or renderer
  delay from turning a fast physical flick into a drag that ends at rest.
- Added real-Chromium fling coverage and a contact-ID sweep to the motion probe
  for reproducing velocity regressions independently of the native client.

## 0.12.0 - 2026-08-06

- Replaced synthesized click, scroll, long-press, and zoom commands with raw
  UIKit multi-touch contacts dispatched through Chromium's native touch input.
- Kept physical input touch-based in both Desktop and Mobile Websites modes,
  restoring tap activation, vertical navigation, compositor fling and inertia,
  and multi-touch pinch zoom on mobile pages.
- Added ordered, bounded, and move-coalescing input queues with stable contact
  IDs, exact video-generation validation, CSS visual-viewport mapping, and
  cancellation across navigation, tab, viewport, website-mode, and disconnect
  boundaries.
- Kept Chromium gesture delivery nonblocking under expensive page handlers and
  committed UIKit's final lift position before release, preserving fling
  velocity when mobile pages process the move and release in adjacent frames.
- Replaced editable polling with event-driven focus reporting across documents,
  open shadow roots, and out-of-process frames, restoring the native keyboard
  for login fields such as Reddit's.
- Added iOS marked-text composition update, commit, and cancel handling plus
  appropriate native keyboard types for common HTML input modes.
- Removed the obsolete input commands and frame scroll metadata. This release
  requires the exact protocol version `20260806-1` on both ends.

## 0.11.0 - 2026-08-03

- Added roaming through Cloudflare Tunnel while retaining Surf's existing
  certificate-pinned TLS session end to end inside an opaque WebSocket bridge.
- Kept direct LAN endpoints unchanged and stored transport per endpoint, so a
  paired server can use low-latency LAN access nearby and roaming elsewhere.
- Added bounded tunnel capacity, binary-only framing, idle deadlines, and
  byte-integrity coverage without exposing arbitrary loopback services.
- Made roaming-address verification deadline-bound and routed public HTTPS
  discovery through Apple's system networking stack while retaining custom
  certificate pinning for direct Surf endpoints and the inner tunnel session.
- Rebuilt the on-device application log as a structured event system with
  explicit severity, component, message, and typed fields for every event.
- Added a readable, color-coded Logs screen grouped by date, with expandable
  event details, field inspection, copy, clear, and live auto-follow controls.
- Migrated existing text log history to structured NDJSON while preserving its
  timestamps and messages, and removed the legacy runtime logging API.
- Displayed address-verification progress and actionable DNS, TLS, transport,
  and identity failures directly on the visible Server Details screen.
- Excluded credentials, query strings, pairing tickets, and full URLs from
  structured diagnostics.

## 0.10.7 - 2026-08-01

- Made native browser-identity discovery reliable on slower ARM64 hosts by
  probing a private loopback origin and waiting for Chrome's real execution
  context, without sending a request off-host or fabricating Client Hints.

## 0.10.6 - 2026-08-01

- Made the desktop user-agent adjustment surgical: Surf now changes only the
  `HeadlessChrome/` product token and preserves the selected browser's native
  brands, versions, platform details, and client hints.
- Kept Mobile Websites coherent by changing only its device-facing fields
  while retaining Chrome, Edge, or Chromium's authentic brand/version list.
- Isolated browser-identity probing from Surf tabs and made failures fall back
  to the browser's completely native identity instead of partial metadata.

## 0.10.5 - 2026-08-01

- Loaded Surf's tab-capture and content-blocking extensions through Chrome's
  DevTools extension API, restoring support for branded Google Chrome and Edge
  builds that ignore the legacy command-line extension switch.
- Preferred compatible installed Google Chrome before Edge or Chromium, making
  host-provided Widevine and streaming-site browser compatibility available
  without copying the CDM into another browser.
- Made the live EME capability probe authoritative when externally supplied
  Widevine does not appear on `chrome://components`.

## 0.10.4 - 2026-08-01

- Made `surf pair` prefer `SURF_PUBLIC_ADDRESS` when generating pairing
  invitations, so Docker, NAT, and remotely hosted servers encode their actual
  reachable address instead of a private container address.

## 0.10.3 - 2026-08-01

- Made Windows upgrades force-close the installed Surf process tree before
  replacing its executable, avoiding Restart Manager error 5.
- Made silent desktop updates launch the newly installed Surf automatically.

## 0.10.2 - 2026-08-01

- Prevented a force-closed desktop tray from leaving its managed daemon,
  Chromium tree, listener, or backend lock behind.
- Made desktop startup remove dead runtime descriptors and safely take control
  of an unresponsive process only after verifying the Surf executable.
- Added targeted recovery for malformed desktop settings and device registries,
  preserving invalid files for diagnosis instead of requiring all Surf state to
  be deleted.
- Added bounded Chromium-profile recovery after repeated startup failures. Surf
  preserves the failed profile plus its TLS identity, pairing state, bookmarks,
  and history before retrying with clean browser state.
- Fixed Windows daemon shutdown so startup errors reach the supervisor and
  `desktop.log` with their real nonzero exit status.

## 0.10.1 - 2026-07-31

- Made the Windows tray authenticate and take ownership of an existing Surf
  daemon during startup and upgrades, without killing unverified processes.
- Added automatic daemon recovery with bounded backoff and child lifecycle
  logging, so a transient browser or port collision cannot leave Surf stopped.
- Made the Windows installer stop existing Surf process trees before launching
  the newly installed version.

## 0.10.0 - 2026-07-31

- Replaced shared passwords and plaintext transport with Surf's built-in TLS
  listener, persistent RSA server identity, exact certificate pinning, and a
  separate device-only RSA Keychain key for every paired server.
- Added server-initiated, single-use pairing invitations: self-contained QR
  codes with direct identity pinning on camera-equipped iOS 6+ devices, address
  plus six-digit code and an independently computed identity check for manual
  pairing, five-attempt manual-code lockout, and Bonjour used only as a locator.
  Pairing stays closed until the owner explicitly starts it.
- Made legacy-camera QR pairing dependable with a compact identity-bound QR,
  a larger low-glare desktop presentation, high-resolution capture, automatic
  exposure/focus, and an on-device software decoder for iOS 6.
- Made **Forget Server** revoke the backend approval when reachable, then remove
  the local server and key regardless of network availability.
- Added favicon discovery and caching to the iPad tab strip and phone Pages UI.
- Added unlimited saved servers, explicit changed-identity failures, verified
  alternative addresses, Rename, Forget, and Pair Again controls, automatic
  reconnect to only the last selected server, and removal of all legacy
  password/server defaults during the breaking upgrade.
- Added a persistent `surf daemon`, authenticated loopback CLI discovery,
  `surf status`, invitation creation through `surf pair`, and device management with
  immediate revocation through `surf devices list/revoke`.
- Moved every public interface to `/api/v1/...`, removed all legacy route
  aliases, bumped the native protocol, and encrypted control, media, files,
  diagnostics, and client updates without requiring a reverse proxy or public
  certificate.
- Added LAN-safe Bonjour advertisement on Windows and hardened the iOS 6
  Secure Transport lifecycle so cancellation, backend restarts, and concurrent
  media/control traffic reconnect without crashing the client.
- Isolated short-lived device cookies in memory by pinned server identity, so
  discovery probes and Surf instances sharing a hostname cannot receive one
  another's authenticated session.
- Made native-size browser text and page edges substantially sharper with
  constant-quality H.264 at QP 12, retaining a 24 Mbps VBR compatibility
  fallback without changing the low-latency 30 FPS delivery path.
- Made rotation and fullscreen viewport changes settle atomically, preserving
  exact small-phone landscape dimensions while keeping the control connection
  and media subscription alive through a single tab-capture reconfiguration.
- Synchronized the remote page Fullscreen API with Surf's native fullscreen
  chrome in both directions, including embedded players such as YouTube, and
  reduced fullscreen controls to a single unobtrusive Exit button.

## 0.9.0 - 2026-07-31

- Rebuilt native browser chrome around classic Safari's device-specific
  structure: phones now use a unified omnibox, six-button bottom toolbar, and
  swipeable Pages controller with readable page titles, while iPads keep a top
  toolbar and compact persistent tabs with close, new-tab, and overflow
  controls.
- Replaced the generic browser action menu with native Share and More activity
  surfaces, moved Settings to **Surf Settings** under More, and made the
  Bookmarks/History/Downloads Library full-screen on phones and an anchored
  popover on iPads.
- Added on-demand decoded-frame tab previews with a bounded 12-entry LRU cache,
  favicon/title placeholders, and memory-warning cleanup, without adding a
  backend or wire-protocol dependency.
- Made fullscreen expose translucent Back, Forward, and Exit controls while
  preserving one settled viewport update across fullscreen and rotation.
- Matched chrome appearance to the installed OS with procedural iOS 6
  gradients, shadows, and icons and a flatter iOS 7–14 palette.
- Made the rootful native package universal: armv7 remains compatible with iOS
  6, while arm64 supports 64-bit devices from iOS 7 through iOS 14.
- Enabled iPhone and iPod touch installation, with compact toolbar/find
  layouts, phone-safe menus and pickers, adaptive modals, and legacy phone
  icons and launch images.
- Added release and pull-request checks for both Mach-O slices, their minimum
  iOS versions, package identity, device families, resources, and privileged
  updater permissions.
- Preserved host-browser Widevine/CDM loading on every backend platform, added
  compatible Microsoft Edge discovery, and exposed an actual EME capability
  result in authenticated runtime statistics.
- Kept desktop User-Agent Client Hints coherent with Surf's normalized browser
  identity so sign-in and anti-bot checks do not see an Edge/Windows user agent
  paired with empty browser and platform metadata.
- Hardened the native CoreVideo/OpenGL presentation path against the recurring
  `presentRenderbuffer:` crash in the iOS 6 PowerVR driver.
- Kept video subscriptions alive across rotation and fullscreen viewport
  changes, automatically retrying Chromium's transient encoder reconfigure
  failures and configuring each recovered generation before its first frame.

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
