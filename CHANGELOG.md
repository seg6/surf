# Changelog

A compatibility change requires matching client and server generations. App
version differences alone do not.

## Unreleased

### Set up Surf's browser on your computer

- Open your Surf tabs on the computer to sign in or change browser settings.
  Choose **Browser setup…** in the tray or dashboard, or run `surf browser`.
- Connected devices show a paused screen. Close the setup windows to resume
  automatically, or choose **Resume here** on an iOS or desktop client and
  confirm that the computer's browser will close.
- Tabs, sign-ins and browser settings carry over using the same Surf profile.
  Pages reload when switching, so unsaved forms and active transfers may be
  lost. Completed downloads, bookmarks and history stay in place.
- With Surf stopped, `surf browser` lets you configure its browser without
  starting a streaming server. Closing the setup windows exits the command.
  Use `surf browser --status` to check its state or `surf quit` to close it.
- Surf prevents two browsers from using its profile at once. If the setup
  browser will not close normally, a separate force-close confirmation lets
  you recover without closing unrelated browsers.

### Fixes

- Fixed **Save & restart** in the dashboard failing with a form-data error
  when changing the server name, port or other connection settings.
- Fixed a browser worker startup stall that prevented Facebook Reels and
  other features that depend on dedicated web workers from loading.
- Fixed live browser view startup failures caused by overlapping capture
  requests or requesting a new stream before releasing the existing one.
- Browser capture failures now report their cause promptly instead of waiting
  for video startup to time out. Capture can recover after transient errors.

## 0.16.0 - 2026-09-07

### Browse from Linux, too

- Added a Linux desktop client preview. Connect to a Surf server, pair with a
  code, and browse with tabs, bookmarks, history, downloads, clipboard, audio,
  and keyboard and mouse input. The server still runs the browser; this is a
  separate client, not a replacement for the desktop backend.
- The desktop client has compact controls, searchable Library views, an
  address bar with suggestions, and resolution presets for testing iPhone and
  iPad layouts. Window resizing updates the remote browser viewport.
- The iOS and Linux clients now share a small C99 core for session state,
  browser state and protocol handling. Their interfaces and device-specific
  rendering remain separate. Automated tests exercise the shared behavior,
  real desktop input, pairing and streaming to catch regressions earlier.

### More reliable downloads

- Fixed downloads not starting after the browser-management changes.
- Download progress now clears when a transfer finishes or fails. Incomplete
  files are kept separate from completed downloads, and duplicate filenames
  no longer overwrite existing files.
- Fixed opening and saving downloads in both native clients, including names
  containing spaces or special characters. iOS transfers use bounded chunks
  to avoid loading a large file into memory at once.

### A more consistent interface

- Restored native iOS Settings rows, controls and grouped sections, with
  clearer Library editing actions and fixes for backgrounds changing on scroll.
- Improved dark-mode contrast across popup frames, menus, search fields and
  address-bar controls. Classic iOS uses subdued toolbar icons.
- The backend dashboard now follows the Cydia repository page's visual style.
  Choose Light, Dark or System appearance; the repository page also follows
  the browser's light/dark preference where supported.

Upgrade normally; no uninstall or data reset is needed. Compatibility remains
generation 1, so existing compatible clients and servers can still connect.
The Linux client is a preview; the iOS client remains the established device app.

## 0.15.5 - 2026-09-01

- Updates now install over the existing desktop and iOS packages. Browser data,
  settings, device keys, and the server identity stay in place.
- Client and server app versions may differ when their compatibility generation
  matches.
- Older iOS clients receive the package embedded in the backend. Newer clients
  require a backend update and are not downgraded.
- Builds verify the embedded package ID, version, compatibility generation,
  length, and SHA 256.
- Added `surf quit` for a clean shutdown of the desktop app or foreground
  server.

## 0.15.4 - 2026-09-01

- Chrome and Edge launcher process handoff no longer causes a false
  **Video Unavailable** error.
- The server and Chromium now restart as one supervised unit after a browser
  failure.
- Chromium stays open between client sessions while capture stops when no
  client is connected.
- Tab, URL, website mode, and appearance state is saved after changes.
- Failed Windows profile recovery is retried on the next launch.
- Browser startup errors include a bounded tail of browser output.

## 0.15.3 - 2026-08-31

- `AVSampleBufferDisplayLayer` video is available on iOS 6 and 7.
- Video continues through keyboard transitions and falls back to the older
  renderer after repeated display failures.
- Fixed an iOS 6 crash when the address bar opened the keyboard.
- Restored separate icon artwork for iOS 6 and iOS 7 or later.

## 0.15.2 - 2026-08-31

- iOS 8 and later sends compressed H.264 directly to the system display queue.
- Keyboard changes no longer rebuild the video surface.
- Queue recovery drops only independently decodable frames and replaces failed
  display layers.
- Diagnostics now report metrics for the active renderer.
- Native size 60 FPS video uses a 16 Mbit/s variable bitrate by default.
- Compatibility generation 1, originally `20260831-1`.

## 0.14.0 - 2026-08-31

- Reworked the iOS browser controls, tabs, new tab page, Library, pairing,
  Reader, media controls, and settings.
- Replaced drawn symbols with Lucide icons and adopted the Deta Surf artwork.
- Added a compact iPad browser bar that can sit at the top or bottom.
- Tabs now scroll continuously and show current page titles.
- More is now a browser menu. The system share sheet opens only from Share.
- Reorganized Settings and replaced the Performance Monitor.
- Added a dark appearance and Chromium color scheme preference.
- Fixed stale tab titles, loading state, navigation state, and tab state.
- Compatibility generation `20260826-1`. Client and server must match.

## 0.13.7 - 2026-08-21

- Added the Cydia and Sileo repository at
  [seg6.space/surf](https://seg6.space/surf/).
- Added installer links to desktop pairing, terminal pairing, and the README.
- Release publishing verifies package hashes and the signed release source.

## 0.13.6 - 2026-08-21

- Backgrounding the iOS app now pauses media, clipboard checks, and diagnostics.
- The connection stays open for one minute before disconnecting.

## 0.13.5 - 2026-08-13

- Chromium started with the first client and stopped two minutes after the last
  client disconnected.
- Tabs, website mode, cookies, and storage survived idle shutdown.
- Added browser lifecycle diagnostics and `SURF_BROWSER_IDLE_TIMEOUT`.
- The supervised lifecycle in 0.15.4 later replaced this model.

## 0.13.4 - 2026-08-11

- The iOS app now opens a playback audio session before creating its queue.
- Diagnostics distinguish silent capture from transport and playback failures.

## 0.13.3 - 2026-08-09

- Switching website mode no longer disables DOM touch or Pointer Events.
- Browser tests cover inertial scrolling and native HTML selects.

## 0.13.2 - 2026-08-09

- Keyboard intent now follows the full interaction started by a touch.
- HTML selects use a native picker with multiple selection and frame support.
- Compatibility generation `20260809-4`.

## 0.13.1 - 2026-08-09

- Autofocus and script focus no longer open the iOS keyboard.
- Compatibility generation `20260809-3`.

## 0.13.0 - 2026-08-09

- Added clipboard sync and one time text delivery.
- Added Paste, Escape, and Tab controls above the iOS keyboard.
- Combined desktop, server, and iOS logs into one bounded structured format.
- Added the log viewer and `surf logs`.
- Renamed `surf daemon` to `surf serve`.
- Added `surf serve --pair`.
- Unified restart and shutdown handling across supported hosts.
- Compatibility generation `20260809-2`.

## 0.12.5 - 2026-08-08

- Chromium software H.264 became the default for consistent old iOS decoding.
- GPU page rendering and composition remain enabled.

## 0.12.4 - 2026-08-07

- Removed the virtual GPU, X11, forced software rendering, and shared memory
  launch modes.
- Chromium now uses one headless configuration with host GPU rendering.

## 0.12.3 - 2026-08-07

- Increased capture and iOS presentation from 30 to 60 FPS.
- Reduced presentation buffering to two frames.
- Raised fallback H.264 bitrate to 48 Mbit/s.
- Release builds now reuse unchanged toolchains and Go outputs.
- Windows installers are built in a pinned container.

## 0.12.2 - 2026-08-06

- Desktop tray bindings are now pinned and platform independent.
- Linux can build Linux, Windows, Intel Mac, and Apple Silicon Mac packages.
- iOS and desktop releases now come from one signed tag workflow.
- Added Linux based macOS archives and NSIS Windows installers.

## 0.12.1 - 2026-08-06

- Fixed touch IDs that caused scrolling to lose momentum over time.
- Input timestamps now reflect backend dispatch.
- The final touch position is delivered before release.
- Added Chromium tests for fling behavior and touch ID reuse.

## 0.12.0 - 2026-08-06

- Replaced synthetic gestures with the device touch contacts.
- Added bounded touch snapshots, stable contact IDs, and stale gesture
  cancellation.
- Page handlers no longer block later touch input.
- Focus events now work through open shadow roots and embedded frames.
- Added iOS marked text composition and field specific keyboard layouts.
- Compatibility generation `20260806-1`.

## 0.11.0 - 2026-08-03

- Added Cloudflare Tunnel transport with an inner pinned Surf TLS connection.
- Saved servers can keep separate LAN and roaming addresses.
- Added transport limits and endpoint verification deadlines.
- Replaced iOS text logs with bounded structured events and a log viewer.
- Added clearer DNS, TLS, transport, and identity errors.
- Logs omit credentials, query strings, tickets, and full URLs.

## 0.10.7 - 2026-08-01

- Browser identity detection now waits for the page environment on slow ARM64
  hosts.
- The check uses a private local address.

## 0.10.6 - 2026-08-01

- Surf now removes only the `HeadlessChrome` product marker.
- Browser name, version, platform, and Client Hints remain unchanged.
- Mobile Websites changes device facing fields only.
- Failed identity detection leaves the browser identity untouched.

## 0.10.5 - 2026-08-01

- Capture and content blocking extensions now load through the DevTools API.
- Surf prefers compatible Chrome before Edge or Chromium.
- Added a live Widevine EME capability test.

## 0.10.4 - 2026-08-01

- Pairing codes now prefer `SURF_PUBLIC_ADDRESS`.

## 0.10.3 - 2026-08-01

- Windows installers stop Surf and Chromium before replacing files.
- Silent updates start the new version after installation.

## 0.10.2 - 2026-08-01

- Forced desktop shutdown no longer leaves the server, Chromium, listener, or
  live lock behind.
- Startup removes dead runtime records and verifies processes before takeover.
- Invalid settings and device files are preserved while affected state is
  repaired.
- Repeated Chromium failures move the profile aside and retry with clean browser
  state.
- Windows startup failures now return a nonzero status to the desktop app.

## 0.10.1 - 2026-07-31

- The Windows tray can adopt an authenticated server that is already running.
- Server restart uses bounded backoff and records process events.
- The installer closes old Surf and Chromium processes before launch.

## 0.10.0 - 2026-07-31

- Replaced shared passwords and plaintext connections with pinned TLS and
  separate device keys.
- Added QR and manual pairing invitations.
- Added an iOS 6 QR decoder and camera improvements.
- **Forget Server** now revokes the device when the backend is reachable.
- Added identity change errors, multiple verified addresses, rename, forget,
  and pair again controls.
- Sessions are kept in memory and bound to the server identity.
- Added `surf status`, `surf pair`, and device management commands.
- Moved network routes under `/api/v1`.
- Added secure reconnect handling and tab favicons.
- Raised H.264 detail to QP 12 with a 24 Mbit/s fallback at 30 FPS.
- Rotation and fullscreen now resize without reconnecting.
- Page fullscreen and native fullscreen now stay in sync.
- This release changed the client and server protocol.

## 0.9.0 - 2026-07-31

- Added native phone and iPod touch layouts.
- Reworked browser controls around classic Safari.
- Split Share and More, moved Settings, and added a fullscreen phone Library.
- Added bounded tab previews.
- Added separate iOS 6 and iOS 7 or later visual styles.
- Added armv7 and arm64 package checks.
- Added Microsoft Edge discovery and Widevine reporting.
- Hardened the iOS 6 OpenGL renderer.
- Video now survives rotation and fullscreen changes.

## 0.8.4 - 2026-07-30

- Restored Chromium tab audio on Linux without playing it on the host.
- Fixed rotated and letterboxed video.
- Audio only sessions can add video and survive encoder restarts.
- Added browser profile locking and improved Chromium cleanup.

## 0.8.3 - 2026-07-30

- Capture now keeps the newest frame and reports motion diagnostics.
- Added Intel and Apple Silicon Mac packages.

## 0.8.2 - 2026-07-30

- Improved text and detail in streamed pages.

## 0.8.1 - 2026-07-30

- Replaced screenshots and external FFmpeg capture with Chromium tab capture.
- Added active tab audio on Linux, Windows, and macOS.
- Unified runtime and package ownership around the new capture path.
- Matched capture timing to the client display cadence.

## 0.8.0 - 2026-07-30

- Added browser audio capture on Windows.

## 0.7.0 - 2026-07-29

- Added bookmarks, history, downloads, settings, media controls, and Mobile
  Websites.
- Added touch browsing, performance diagnostics, content blocking, and 30 FPS
  H.264 defaults.

## 0.6.7 - 2026-07-29

- Reduced H.264 delay and allowed overlapping VideoToolbox decoding.
- Added optional NVIDIA NVENC encoding.
- Interrupted iOS client updates can now recover.

## 0.6.6 - 2026-07-29

- Backends may offer a newer compatible iOS client without an exact app version
  match.

## 0.6.5 - 2026-07-29

- Fixed iOS update metadata produced by ARM64 Linux builders.

## 0.6.4 - 2026-07-28

- Added Linux ARM64 packages and Docker deployment.

## 0.6.3 - 2026-07-28

- The iOS updater no longer replaces itself while running.
- Removed duplicate CI release builds.

## 0.6.2 - 2026-07-28

- iOS updates now verify the installed package before reporting success.

## 0.6.1 - 2026-07-28

- Added iOS performance diagnostics for decode, display, frame age, and input
  latency.

## 0.6.0 - 2026-07-28

- Added one `surf` program for the desktop app and foreground server.
- Added verified desktop and iOS update metadata.
- Added managed Chromium discovery and desktop packages for Linux, Windows, and
  macOS.
- Expanded package validation.

## 0.5.0 - 2026-07-27

- Combined the desktop supervisor and backend in `surf`.
- Desktop mode became the default. `surf serve` remained available for servers.
- Added packages for all desktop platforms.

## 0.4.0 - 2026-07-27

- Standardized video on H.264.
- Matched iOS presentation to the display refresh rate.
- Added Chromium process management and Windows capture.

## 0.3.0 - 2026-07-27

- Added a standalone Linux server.
- Added browser discovery, diagnostics, and host audio support.
- Made high detail H.264 the default.
- Split tabs, input, browser control, and media paths.
- Added editable saved server settings to iOS.

## 0.2.0 - 2026-07-26

- Required password authentication and expiring WebSocket tickets.
- Restricted diagnostics and uploads.
- Pinned the iOS build toolchain.

## 0.1.0 - 2026-07-26

- Added the backend, iOS client, container image, and release workflow.
