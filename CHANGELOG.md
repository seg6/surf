# Changelog

This is the user-facing history of Surf. Each entry starts with what changed in
the experience; lower-level details are included when they explain compatibility,
performance, security, or troubleshooting.

A **protocol change** means the Surf client and server must be updated together.

## 0.15.4 - 2026-09-01

*More dependable browser startup and recovery, especially on Windows.*

- **Fixed false “Video Unavailable” failures.** Chrome and Edge sometimes launch
  the real browser in another process and then close their small starter process.
  Surf now waits for the browser's actual control connection instead of treating
  that normal handoff as a crash. This addresses the failure that could make Surf
  work only while a Windows Remote Desktop session remained open.
- **Made restarts predictable.** The Surf server and its browser now run as one
  supervised unit. If the browser genuinely crashes, the desktop tray restarts
  the complete unit instead of trying to repair a half-running browser inside the
  old server.
- **Kept idle resource use low without closing the browser.** Tabs remain ready
  between client connections, while video and audio capture still stop when no
  device is watching.
- **Made tab restoration reflect the latest browser state.** Surf now saves tab,
  active-tab, URL, website-mode, and appearance changes shortly after they happen
  instead of waiting for server shutdown. Closing the final page correctly saves
  Surf's New Tab state rather than resurrecting an older tab after a restart.
- **Improved automatic profile recovery.** If Windows temporarily locks the
  browser profile, Surf no longer records a failed repair as successful. It exits
  cleanly and retries the recovery on the next start.
- **Added useful launch diagnostics.** A genuine startup failure now includes a
  short, size-limited tail of Chrome or Edge's error output without allowing logs
  to grow without bound.

## 0.15.3 - 2026-08-31

*Smooth system-rendered video and correct classic icon artwork on iOS 6 and 7.*

- **Brought the fast video path to iOS 6 and 7.** Surf hands compressed H.264
  video to the operating system instead of decoding and drawing every frame in
  the app. High-motion pages can now remain sharp and reach the requested frame
  rate instead of becoming blurry and settling around 30–40 FPS.
- **Kept video moving through keyboard transitions.** Opening or closing the
  keyboard no longer stalls the stream. Surf can recover from temporary display
  pressure and automatically falls back to the older renderer on devices that
  cannot use the direct path reliably.
- **Fixed an iOS 6 crash** when tapping the address bar and opening the keyboard.
- **Restored the intended app icon on every supported system.** iOS 6 receives
  the rounded classic icon with the plane extending beyond its tile; iOS 7 and
  later receive full-bleed artwork designed for the system's own icon mask.
  Installation refreshes the correct artwork automatically.

## 0.15.2 - 2026-08-31

*A new low-overhead video renderer for iOS 8 and later.*

- **Removed the expensive per-frame drawing pipeline.** On iOS 8+, Surf now gives
  compressed H.264 frames directly to the system display queue. The app no longer
  waits for a decode callback, moves each decoded surface through UIKit, and draws
  it with OpenGL. This isolates playback from interface animations and cuts the
  renderer's own work to roughly a millisecond on supported devices.
- **Stopped keyboard changes from restarting video.** The remote browser keeps one
  stable, even-sized surface while the keyboard or bottom browser controls move.
  iOS simply covers or repositions part of that surface locally.
- **Made overload recovery safe.** Surf uses a single checked H.264 sample builder,
  discards only frames that cannot be decoded independently when the system queue
  is full, and replaces a failed display layer cleanly. iOS 6 and 7 retain the
  compatible VideoToolbox/OpenGL renderer in this release.
- **Made performance readings match the active renderer.** Newer systems report
  accepted compressed frames, display-queue pressure, recoveries, and failures;
  older systems continue to report decode and OpenGL timing.
- **Kept the default stream sharp at 60 FPS.** The primary stream uses the native
  display size and a detail-focused 16 Mbit/s variable bitrate. Unnecessary key
  frames and competition between ordinary controls and incoming media were
  removed.
- **Compatibility:** protocol `20260831-1`; update the client and server together.

## 0.14.0 - 2026-08-31

*A complete redesign of Surf's native browser interface.*

### Browser controls

- Rebuilt the address bar, phone toolbar, iPad tabs, tab overview, new-tab page,
  connection screens, Library, pairing, Reader, media controls, and supporting
  screens as one consistent visual system.
- Replaced hand-drawn symbols with the bundled Lucide icon set and adopted the
  Apache-2.0-licensed Deta Surf artwork across the app, desktop tray, management
  page, and launch branding.
- Combined the iPad address bar and tabs into a compact edge rail. It can sit at
  the top or bottom of the screen, shows the hostname until editing begins, and
  expands to the full address without resizing the streamed page.
- Made every tab continuously scrollable instead of hiding overflow in a separate
  menu. Tabs use live page titles, clearly identify the active page, and keep the
  New Tab button directly beside the rail.
- Replaced the accidental system Share sheet behind **More** with a dedicated
  browser-tools menu. AirDrop and other destinations now appear only under the
  actual Share action, and menus size themselves to their contents.

### Settings and diagnostics

- Reorganized Settings into readable groups for browsing, performance, data,
  server controls, and version information. Explanations wrap instead of being
  clipped, unavailable actions say why, and the client and protocol versions are
  always visible.
- Rebuilt the Performance Monitor as a compact translucent panel. It shows the
  important health readings first, can reveal round-trip and pipeline details,
  never scrolls inside itself, and never resizes the browser stream.

### Appearance and state

- Added a neutral dark appearance to the entire native interface and told Chromium
  to expose the matching standard `prefers-color-scheme` value to websites. Surf
  does not override colors explicitly chosen by a site.
- Corrected dark popovers, arrows, selectors, Library controls, table cells,
  navigation bars, inactive controls, and address-bar placeholder contrast while
  keeping iOS-owned Share screens readable.
- Fixed tab titles that remained stuck on the URL. Surf now uses the document's
  real title and falls back to a short hostname only when no useful title exists.
- Fixed loading indicators becoming stuck or leaking into another tab. Loading,
  Stop, navigation, reconnect, activation, and close state are now tracked per tab
  and only for the page's main frame, with a timeout for sites that never send a
  final completion event.
- Standardized headings and controls on the native system font from iOS 6 through
  14, with more reliable touch targets, separators, selected states, and compact
  layouts on both phones and tablets.
- **Compatibility:** protocol `20260826-1`; update the client and server together.

## 0.13.7 - 2026-08-21

*Simpler installation on jailbroken iOS devices.*

- Added a public Cydia and Sileo repository at
  [seg6.space/surf](https://seg6.space/surf/), with an iOS 6-compatible page and a
  direct `.deb` download.
- Linked the installer from Paired Devices on the desktop, the terminal pairing
  flow, and the quick-start guide.
- Release automation now verifies the package hashes and publishes only packages
  built from a signed Surf release.

## 0.13.6 - 2026-08-21

*Lower battery use while Surf is in the background.*

- Surf now pauses audio, video decoding and presentation, clipboard checks, and
  diagnostics as soon as the iOS app enters the background. In particular, the
  silent audio queue can no longer keep older devices awake unnecessarily.
- The secure connection remains available for one minute, allowing a quick resume
  without reconnecting. After that grace period Surf disconnects and reconnects
  automatically when reopened.

## 0.13.5 - 2026-08-13

*Reduced server resource use between browsing sessions.*

- Chromium starts when the first authenticated client connects and closes two
  minutes after the last one leaves. Pairing, updates, logs, and clipboard tools
  remain available because the lightweight Surf server stays running.
- Tabs, the active tab, Mobile Websites mode, cookies, and site storage survive an
  idle shutdown. Active downloads delay shutdown, and Surf attempts to recover a
  browser that exits unexpectedly without restarting the server.
- Browser state is visible in diagnostics and the desktop management page. Server
  operators can change the delay with `SURF_BROWSER_IDLE_TIMEOUT`; setting it to
  `0` keeps Chromium running. This lifecycle was later replaced by the simpler
  supervised model described under Unreleased.

## 0.13.4 - 2026-08-11

*Reliable page audio on iOS.*

- Surf now opens an explicit playback audio session before creating its audio
  queue, so page sound is not muted by iOS's default ambient mode or the physical
  Silent switch. Failures appear in the client log.
- Diagnostics can distinguish silent audio captured by Chromium from a transport
  or device playback failure. They record small numeric signal summaries, never
  raw audio or its contents.

## 0.13.3 - 2026-08-09

*Fixed touch after switching website modes.*

- Switching between Desktop and Mobile Websites no longer leaves pages without
  DOM touch or Pointer Events. This restores swiping on sites such as TikTok and
  YouTube Shorts without requiring a restart or another navigation.
- Browser-backed tests now cover both inertial scrolling and native HTML selection
  controls so the two input paths cannot silently break each other.

## 0.13.2 - 2026-08-09

*Better keyboard intent and native website pickers.*

- The keyboard opens for the full interaction caused by a real touch, including a
  touched label or overlay that immediately focuses a field. Focus requested later
  by page load, a timer, or other script still stays silent.
- Replaced Chromium's invisible HTML `<select>` popup with a native picker. It is
  anchored and scrollable on iPad, full-screen on narrow devices, and supports
  disabled choices, multiple selection, rotation, web components, and embedded
  cross-origin frames.
- **Compatibility:** protocol `20260809-4`; update the client and server together.

## 0.13.1 - 2026-08-09

*Stopped websites from opening the keyboard by themselves.*

- Autofocus and scripted focus no longer summon the iOS keyboard. It opens only
  after the user physically touches the corresponding field.
- **Compatibility:** protocol `20260809-3`; update the client and server together.

## 0.13.0 - 2026-08-09

*Clipboard sharing, unified logs, and a cleaner server lifecycle.*

### Clipboard and keyboard

- Added owner-controlled, two-way clipboard sync between Windows, macOS, Linux,
  and connected iOS devices. It can be controlled from Settings or with
  `surf clipboard sync`; clipboard text remains in memory and is never logged.
- Added one-time text delivery from Settings or `surf clipboard set`. The client
  acknowledges receipt and can clear the temporary text after two minutes.
- Moved Paste above the iOS keyboard whenever a page field is active, and added
  nearby `Esc` and `Tab` keys for controlling desktop-style forms.
- Fixed the hidden clipboard switch making Settings wider than the screen.

### Logs and server operation

- Combined desktop, server, and iOS logs into one size-limited structured format.
  Settings can follow a selected source live, clear individual sources—including
  a device that is currently offline—and copy either filtered or complete JSON.
- Made the log viewer easier to scan with colors, aligned columns, contained
  horizontal scrolling, and no accidental line wrapping. `surf logs` provides the
  same information without requiring SSH access to the iPad.
- Renamed `surf daemon` to `surf serve` and added `surf serve --pair` for optional
  first-run pairing. Restarts, updates, desktop ownership changes, parent-process
  loss, and shutdown now follow one graceful path that cleans up Chromium on
  Linux, Windows, and macOS.
- **Compatibility:** protocol `20260809-2`; update the client and server together.

## 0.12.5 - 2026-08-08

*More consistent 60 FPS compatibility across servers.*

- Surf now uses Chromium's software H.264 encoder by default. Hardware encoders
  remain fast, but their output differed enough across platforms to overwhelm
  some legacy iOS decoders. Page rendering and compositing still use the GPU.

## 0.12.4 - 2026-08-07

*A simpler, native-GPU Chromium setup.*

- Removed old virtual-GPU, X11, forced software-rendering, and temporary shared-
  memory launch modes. Surf now uses one headless Chromium configuration with the
  host GPU, preferring hardware H.264 when compatible and falling back cleanly.

## 0.12.3 - 2026-08-07

*Sharper motion at 60 FPS and faster repeat release builds.*

- Raised capture and iOS presentation from 30 to 60 FPS, removed an extra
  JavaScript timing loop, and limited presentation buffering to two frames. The
  fallback H.264 bitrate increased to 48 Mbit/s so moving text remains legible.
- Release builds now reuse unchanged native toolchains and Go outputs, and build
  the Windows installer in a pinned container. This shortens repeat builds while
  preserving the signed Git tag as the source of every package.

## 0.12.2 - 2026-08-06

*One reproducible release process for every desktop platform.*

- Replaced the desktop tray's platform-specific build dependency with pinned Go
  bindings. Linux, Windows, Intel Mac, and Apple Silicon Mac builds can now all be
  produced from the same Linux build host.
- Combined native iOS and desktop packaging into one signed-tag workflow from one
  verified source checkout.
- Replaced host-only packaging tools with Linux-built macOS app archives and an
  NSIS Windows installer, including generated Mac icon metadata.

## 0.12.1 - 2026-08-06

*Fixed scrolling that gradually lost momentum.*

- Physical scrolling no longer becomes slower and loses fling inertia until the
  client is restarted, including after switching to Mobile Websites.
- Surf now gives Chromium small gesture-local touch IDs instead of identifiers
  that grow for the lifetime of the app, and timestamps input when the server
  actually sends it.
- The final finger position is delivered before touch release, preventing network
  or page delay from turning a fast flick into a drag that ends at rest.
- Real-Chromium tests reproduce fling behavior and sweep touch identifiers to
  prevent the regression from returning.

## 0.12.0 - 2026-08-06

*True multi-touch input and a more reliable native keyboard.*

- Replaced separate synthetic click, scroll, long-press, and zoom commands with
  the iOS device's actual touch contacts. Desktop and Mobile Websites modes now
  share the same natural taps, inertial scrolling, and pinch-to-zoom behavior.
- Touch input stays ordered without letting delayed moves build up: queued motion
  is bounded and safely combined, contacts keep stable IDs, and stale gestures are
  cancelled after navigation, tab changes, rotation, mode changes, or disconnects.
- Slow page event handlers no longer block incoming gestures, and Surf preserves
  the final movement needed for Chromium to calculate fling velocity.
- Replaced repeated field polling with immediate focus events from normal pages,
  web components, and embedded cross-origin frames. This restores the native
  keyboard for login fields such as Reddit's.
- Added full iOS marked-text composition for languages that build characters over
  several keystrokes, plus suitable keyboard layouts for common HTML field types.
- **Compatibility:** protocol `20260806-1`; update the client and server together.

## 0.11.0 - 2026-08-03

*Secure browsing away from the local network and much better diagnostics.*

### Roaming

- Added optional access through Cloudflare Tunnel. Surf's own certificate-pinned,
  encrypted session remains intact inside the tunnel; Cloudflare only relays an
  opaque WebSocket stream.
- A saved server can keep its fast direct LAN address nearby and use a separate
  roaming address elsewhere. Direct connections work exactly as before.
- Tunnel traffic has fixed capacity and idle limits, carries binary Surf data
  only, and cannot be used to expose arbitrary services on the server computer.
- Public address checks now have clear deadlines and use Apple's networking stack,
  while Surf continues to verify the exact paired server identity end to end.

### Logs and connection errors

- Rebuilt the on-device log as structured events with severity, component,
  message, and named details. The new Logs screen groups events by date and
  supports expansion, field inspection, copy, clear, and live following.
- Existing text logs are migrated without losing their timestamps or messages.
- Server Details now shows address verification as it happens and distinguishes
  DNS, TLS encryption, transport, and changed-server-identity failures.
- Logs deliberately exclude credentials, URL query strings, pairing tickets, and
  complete visited addresses.

## 0.10.7 - 2026-08-01

*More reliable startup on slower ARM64 servers.*

- Surf now waits for the browser's real page environment before reading its
  identity. The check uses a private local address, never contacts an outside
  website, and no longer fails merely because Chromium initialized slowly.

## 0.10.6 - 2026-08-01

*Websites now see a consistent, authentic browser identity.*

- Surf removes only Chromium's `HeadlessChrome` marker instead of replacing the
  whole user agent. Chrome, Edge, or Chromium keeps its real name, version,
  operating-system details, and modern Client Hints metadata.
- Mobile Websites changes only device-facing fields and keeps the host browser's
  real brand and version information intact.
- Identity checks run outside normal Surf tabs. If detection fails, Surf uses the
  browser's untouched identity instead of sending a partially rewritten one.

## 0.10.5 - 2026-08-01

*Better compatibility with Chrome, Edge, and protected streaming sites.*

- Surf now loads its capture and content-blocking extensions through Chromium's
  supported debugging interface. This works with branded Chrome and Edge builds
  that ignore the older extension command-line option.
- Surf prefers an installed compatible Google Chrome before Edge or Chromium, so
  it can use the browser's own Widevine support for DRM-protected media without
  copying protected components between installations.
- A live encrypted-media test determines whether Widevine actually works, even if
  it is absent from Chrome's components page.

## 0.10.4 - 2026-08-01

*Correct pairing links for Docker and remotely hosted servers.*

- `surf pair` now prefers `SURF_PUBLIC_ADDRESS`, so its invitation contains the
  externally reachable address instead of a private container or LAN address.

## 0.10.3 - 2026-08-01

*Reliable Windows upgrades.*

- The installer now closes the installed Surf process and its browser children
  before replacing files, preventing Windows Restart Manager “error 5.”
- A silent desktop update automatically launches the newly installed version.

## 0.10.2 - 2026-08-01

*Cleaner recovery after crashes or forced shutdowns.*

- Force-closing the tray no longer leaves its server, Chromium processes, network
  listener, or runtime lock behind.
- On startup, Surf removes dead runtime records and takes over an unresponsive
  process only after confirming that it really is Surf.
- Broken settings and paired-device files are preserved for diagnosis while Surf
  repairs only the affected state, instead of requiring all data to be deleted.
- After repeated Chromium startup failures, Surf can move the damaged browser
  profile aside and retry with clean browser state. It preserves the failed
  profile, Surf's encrypted identity and pairings, bookmarks, and history.
- Windows startup failures now reach the desktop supervisor and `desktop.log`
  with the correct nonzero exit status.

## 0.10.1 - 2026-07-31

*A tray that can safely recover and take ownership on Windows.*

- At startup or during an upgrade, the tray can authenticate to an already-running
  Surf server and adopt it instead of starting a duplicate or killing an unknown
  process.
- The tray restarts a failed server with a limited backoff and records the child
  lifecycle, so a temporary browser or port collision does not leave Surf stopped.
- The Windows installer closes existing Surf and Chromium processes before opening
  the newly installed version.

## 0.10.0 - 2026-07-31

*Secure per-device pairing, modern server management, and sharper fullscreen
browsing.*

### Pairing and security

- Replaced shared passwords and unencrypted connections with Surf's built-in TLS
  encryption. Every server keeps a persistent identity, every iOS device verifies
  that exact identity, and each paired device owns a separate private key stored
  in the iOS Keychain.
- Added single-use pairing invitations. Devices with a camera can scan an
  identity-bound QR code; manual pairing uses an address, six-digit code, and a
  separately calculated identity check. Five failed manual codes lock the
  invitation, and pairing remains closed until the owner starts it.
- Improved QR scanning on older cameras with a compact code, a larger low-glare
  desktop display, high-resolution capture, automatic focus and exposure, and an
  iOS 6-compatible software decoder.
- **Forget Server** revokes the device on the backend when reachable, then removes
  the local server and key even when offline.
- Added clear failures when a known server's identity changes, support for multiple
  verified addresses per server, Rename, Forget, and Pair Again controls, unlimited
  saved servers, and automatic reconnection only to the last selected server.
- Session cookies are short-lived, stored only in memory, and separated by the
  server's verified identity. Two Surf servers sharing a hostname cannot receive
  each other's authenticated session.

### Server and compatibility

- Added the persistent server command, authenticated local command discovery,
  `surf status`, `surf pair`, and `surf devices list/revoke` for immediate device
  management.
- Moved the public API to `/api/v1/...` and encrypted controls, media, files,
  diagnostics, and updates without requiring a reverse proxy or public web
  certificate. This was a breaking client/server protocol update.
- Added safe local-network discovery on Windows and hardened secure reconnection on
  iOS 6 when requests are cancelled, the backend restarts, or media and controls
  operate at the same time.
- Added favicon discovery and caching to the iPad tab strip and phone Pages screen.

### Video and fullscreen

- Increased image detail with constant-quality H.264 (QP 12, where a lower QP
  means less compression) and retained a 24 Mbit/s compatibility fallback at the
  original 30 FPS latency target.
- Rotation and fullscreen now settle in one viewport change without dropping the
  control or media connection, including exact small-phone landscape sizing.
- A website entering or leaving the browser Fullscreen API now updates Surf's
  native fullscreen mode in either direction. Embedded players such as YouTube
  receive a single unobtrusive **Exit** control.

## 0.9.0 - 2026-07-31

*Native phone support and a major browser-interface redesign.*

### Interface

- Rebuilt the browser controls around the structure of classic Safari. Phones use
  one address bar, a six-button bottom toolbar, and a swipeable Pages screen with
  readable titles; iPads keep a top toolbar and persistent compact tabs.
- Replaced the generic action menu with separate native Share and More menus, moved
  Settings under **More → Surf Settings**, and made Library full-screen on phones
  but an anchored popover on iPad.
- Added on-demand visual tab previews with a 12-item memory limit, useful title and
  favicon placeholders, and automatic cleanup when iOS reports low memory.
- Fullscreen now exposes translucent Back, Forward, and Exit controls and performs
  only one settled viewport update during fullscreen or rotation.
- Matched each operating system: iOS 6 keeps its classic gradients and shadows,
  while iOS 7–14 use a flatter appearance.

### Devices and websites

- Added iPhone and iPod touch support. The universal package contains 32-bit ARM
  support for iOS 6 and 64-bit ARM support for iOS 7–14, with layouts, menus,
  pickers, icons, and launch images sized for phones.
- Added release checks for both processor types, minimum iOS versions, device
  families, resources, package identity, and updater permissions.
- Added Microsoft Edge discovery while preserving the host browser's Widevine DRM
  support. Surf reports the result of a real encrypted-media capability test.
- Kept modern browser identity metadata consistent so websites do not see, for
  example, an Edge/Windows user agent paired with missing brand information.

### Stability

- Hardened the iOS 6 CoreVideo/OpenGL renderer against a recurring PowerVR graphics
  driver crash.
- Video subscriptions now survive rotation and fullscreen changes. Surf retries a
  temporary Chromium encoder failure and configures the replacement before showing
  its first frame.

## 0.8.4 - 2026-07-30

*Reliable audio, rotation, and capture recovery.*

- Restored Chromium tab audio on Linux while keeping the captured tab silent on
  the host computer, whether or not an iPad is connected.
- Fixed swapped or letterboxed video after rotating between portrait and landscape.
- Audio-only sessions can now add video later and survive reconnects, rotation, and
  encoder restarts.
- Prevented two Surf servers from sharing one browser profile, rejected outdated
  browser control endpoints, and improved Chromium cleanup after normal shutdowns
  and unexpected exits on Linux and Windows.

## 0.8.3 - 2026-07-30

*Smoother capture and complete Mac support.*

- Stabilized 30 FPS delivery by always preferring the newest frame from the current
  capture source, and added better motion diagnostics.
- Completed native packages for both Intel and Apple Silicon Macs.

## 0.8.2 - 2026-07-30

*Sharper streamed pages.*

- Preserved text edges and fine detail through Chromium tab capture and its
  built-in web video encoder.

## 0.8.1 - 2026-07-30

*A lower-latency, cross-platform capture pipeline.*

- Replaced repeated screenshots and an external FFmpeg process with Chromium's own
  tab capture and H.264 encoder.
- Captured audio from the same active tab on Linux, Windows, and macOS.
- Unified runtime and package ownership around the new capture path.
- Timed capture from the connected device's display cadence to reduce interaction
  latency.

## 0.8.0 - 2026-07-30

*Page audio on Windows.*

- Added capture of the browser's own audio output without recording unrelated
  system sounds.

## 0.7.0 - 2026-07-29

*Surf became a full native browser client.*

- Added bookmarks, history, downloads, Settings, media controls, Mobile Websites,
  and compact tab and address controls.
- Added low-latency touch browsing, on-device performance diagnostics, content
  blocking, and stable 30 FPS H.264 defaults.

## 0.6.7 - 2026-07-29

*Lower video latency and more dependable updates.*

- Reduced H.264 delay with faster frame ingestion, automatic FFmpeg threading, and
  overlapping VideoToolbox decoding on iOS.
- Added optional low-latency NVIDIA NVENC encoding and documented recommended
  settings for the original iPad.
- Surf can now recover a native-client update that was interrupted partway through
  installation.

## 0.6.6 - 2026-07-29

*More useful client updates.*

- The server can offer a newer iPad client whenever it still speaks the same Surf
  protocol, rather than requiring an exact app-version match.

## 0.6.5 - 2026-07-29

*Fixed iPad updates published from ARM64 Linux builders.*

- Release automation now generates valid native-client update metadata on those
  hosts.

## 0.6.4 - 2026-07-28

*More server platforms.*

- Added Linux ARM64 release packages and official Docker deployment support.

## 0.6.3 - 2026-07-28

*Safer native updates and simpler releases.*

- The iOS updater no longer tries to replace itself while it is running.
- Removed duplicate continuous-integration builds so release packaging has one
  clear owner.

## 0.6.2 - 2026-07-28

*Verified iOS updates.*

- Surf now checks the installed package after an update before reporting success.

## 0.6.1 - 2026-07-28

*On-device performance diagnostics.*

- Added the Performance Monitor and client logs for decode time, display time,
  frame age, and interaction latency.

## 0.6.0 - 2026-07-28

*Self-updating desktop and iPad releases.*

- Shipped one `surf` executable that can run either the desktop tray or standalone
  server.
- Added cryptographically verified update information and native packages for both
  desktop and iPad.
- Added automatic Chromium discovery and complete release packaging for Linux,
  Windows, and macOS.
- Strengthened package validation, Windows application resources, and Mac disk-
  image generation.

## 0.5.0 - 2026-07-27

*One application for the tray and server.*

- Combined the desktop supervisor and backend into the `surf` executable.
- Tray mode became the default, while `surf serve` remained available for headless
  servers.
- Added packages for all desktop platforms and dependable health and version
  information.

## 0.4.0 - 2026-07-27

*One observable H.264 streaming path.*

- Consolidated video around H.264, paced display updates with the iOS screen
  refresh, and shortened the path from Chromium capture to the encoder.
- Added cross-platform Chromium process management and Windows video capture.

## 0.3.0 - 2026-07-27

*A standalone Linux server and sharper default streaming.*

- Added browser discovery, diagnostics, and host-audio support to the Linux
  runtime.
- Made sharp H.264 video the default and removed manual stream presets.
- Separated tabs, input, browser control, and media responsibilities so commands
  remain ordered and obsolete work can be rejected safely.
- Added editable saved-server settings to the iOS client.

## 0.2.0 - 2026-07-26

*The first security-focused release.*

- Required password authentication and short-lived tickets before opening media
  or control WebSockets.
- Restricted diagnostic access, limited upload sizes, and tightened HTTP server
  limits.
- Pinned the native build toolchain so iOS packages can be reproduced.

## 0.1.0 - 2026-07-26

*Initial release.*

- Shipped the Surf backend, native client for legacy iOS, container image, and
  automated release workflow.
