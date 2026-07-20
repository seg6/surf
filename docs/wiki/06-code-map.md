# Code Map and Change Guide

This page answers two maintainer questions:

1. Where does a behavior live?
2. What else must change with it?

## Top-level map

| Path | Responsibility |
|---|---|
| `cmd/rbrowser/main.go` | Process composition and fatal lifecycle |
| `embed.go` | Embeds the web client into the Go binary |
| `internal/config` | Environment parsing and client/native protocol versions |
| `internal/auth` | Password checking, cookie signing, WebSocket token, login rate limiting |
| `internal/httpd` | HTTP routing, login UI, static assets, native config, WebSocket upgrade gate |
| `internal/cdp` | Chromium process launch and raw Chrome DevTools Protocol transport |
| `internal/browser` | Browser state, tab lifecycle, input, rendering, features, persistence integration |
| `internal/ws` | Client WebSocket lifecycle, writer serialization, JPEG flow control |
| `internal/protocol` | Shared binary frame layout and client JSON message union |
| `internal/stream` | ffmpeg H.264 encoder, Annex-B splitting, subscriber backpressure |
| `internal/adblock` | Embedded domain list and suffix matching |
| `public` | ES5/iOS 6 web application and presentation |
| `native/client` | Objective-C native client and its UIKit/decoder/networking components |
| `native/docs` | Native design history, protocol details, testing and device gates |
| `tools/nativeprobe` | Native handshake and stream conformance probe |
| `Dockerfile`, `start.sh` | Production image and Xvfb/process bootstrap |

## Process composition

### `cmd/rbrowser/main.go`

This is the composition root:

1. load environment configuration;
2. create authentication state;
3. create the WebSocket hub;
4. create the browser and register it as the hub handler;
5. start Chromium/CDP;
6. terminate if CDP dies;
7. create the HTTP server;
8. register browser-owned routes;
9. listen.

Changes here should be rare. New browser features usually belong behind `Browser`, and new authenticated HTTP routes should be registered through `Browser.RegisterRoutes` or another explicit feature owner.

## Control plane

### `internal/cdp/cdp.go`

Owns:

- Chromium arguments and process launch;
- discovery of the browser DevTools WebSocket;
- request IDs and pending response channels;
- raw JSON envelopes and flat target sessions;
- an event queue separate from socket response delivery;
- command timeouts and connection-close propagation.

Change this package when adding a browser launch flag, altering CDP transport behavior, or fixing request/event concurrency—not when adding an ordinary page command.

**Coupled checks:** Chromium startup, tab discovery, screencast responsiveness, event dispatch, and deadlock behavior.

### `internal/browser/browser.go`

Owns the central browser state:

- tabs indexed by local ID, target ID, and CDP session;
- active-tab selection;
- target attach/detach and popup focus;
- top-level CDP event routing;
- viewport and zoom application;
- screencast frame intake;
- navigation-state broadcasts.

The key locking discipline is documented at the top of the file. Copy state under the mutex, release it, then call CDP.

### `internal/browser/input.go`

Owns:

- address-bar normalization;
- all baseline client-message dispatch;
- coordinate conversion from fractions to zoomed CSS pixels;
- navigation, reload, history, click, wheel, long-press, and keyboard input;
- viewport resize;
- tab commands;
- editable-field detection and keyboard metadata.

Change this file when adding a new core interaction or modifying existing message semantics. Add or update `internal/protocol.ClientMessage` fields at the same time.

**Coupled checks:** web input emission, native input emission, normalized-coordinate behavior, zoom, protocol versions, input tests.

## Rendering plane

### `internal/browser/cast.go`

Owns the JPEG lane:

- quality and resolution selection;
- motion/settle transitions;
- screencast start/stop/reconfiguration;
- full-resolution `captureScreenshot` settle frames;
- scroll metadata for client reconciliation.

Change this file for JPEG quality, latency, and motion behavior.

**Do not:** add per-client JPEG encoders here. The current screencast is active-tab-wide; per-client flow control belongs in the hub.

### `internal/ws/hub.go`

Owns client transport policy:

- connection registration and keepalive;
- one writer goroutine and bounded outbox per client;
- JSON broadcast;
- JPEG sequence stamping;
- three-frame inflight window;
- latest-frame coalescing;
- video-mode routing and sharp-JPEG exception.

Change this file for delivery policy, not browser semantics.

**Coupled checks:** ack accounting, slow clients, disconnect cleanup, web/native coexistence, outbox saturation.

### `internal/browser/video.go`

Bridges a native WebSocket client to `internal/stream`:

- subscribe/unsubscribe on `video` messages;
- wait for the first access unit;
- announce `video-config`;
- switch the client into video mode only after the lane proves healthy;
- send encoded type-3 frames;
- force IDR resynchronization after outbox loss;
- restore JPEG fallback.

Change this file for client mode transitions and fallback behavior.

### `internal/stream/stream.go`

Owns the H.264 process and codec-aware fan-out:

- ffmpeg command construction;
- process start, restart, linger, and crash budget;
- X11 capture and x264 parameters;
- Annex-B access-unit splitting on AUD NALs;
- IDR detection;
- subscriber generations and resize recovery;
- nonblocking queues with drop-to-IDR resynchronization.

Change this file for encoder internals or native video backpressure.

**Coupled checks:** desktop ffmpeg decode, IDR cadence, SPS/PPS availability, resize, crash loops, slow subscriber recovery.

### `internal/browser/screen.go`

Keeps the X display and encoder dimensions aligned with the client viewport. RANDR failure is nonfatal because Xvfb starts with an oversized framebuffer.

Change this file for orientation/cropping problems involving the H.264 lane, not for Chromium CSS viewport issues.

## Features and persistence

### `internal/browser/features.go`

Collects features layered over basic browser control:

- request interception and ad blocking;
- favicon discovery, fetching, cache, and HTTP serving;
- download lifecycle, sanitization, listing, and serving;
- zoom commit and recentering;
- paste, find, suggestions, history, bookmarks, and download messages;
- long-press text extraction.

This is the first place to inspect for a menu/sheet feature visible in either client.

### `internal/browser/store.go`

Owns local history and bookmarks. It is deliberately file-based and guarded by its own mutex.

Change this file for persistence format or suggestion ranking. A format change needs migration or backward-compatible loading because the profile is persistent.

### `internal/adblock/adblock.go`

Owns domain suffix matching against an embedded list. Updating the list is a data change; changing match semantics affects every request intercepted through CDP Fetch.

### `internal/browser/shim.go`

Injects compatibility behavior into every page and current document. Today it disables passkey flows that cannot succeed inside the remote container.

Changes here execute in untrusted page contexts. Keep injected scripts self-contained, defensive, and compatible with the Chromium version in the runtime image.

## Edge and security

### `internal/httpd/httpd.go`

Owns the public route table and authentication boundaries. It injects the current WebSocket token and web-client version into the authenticated HTML and exposes native configuration only after cookie authentication.

Change this file for routes, cache policy, login flow, or handshake gating.

**Coupled checks:** cookie gate, legacy clients, reverse proxy behavior, web/native version mismatch, static cache invalidation.

### `internal/auth/auth.go`

Owns persisted secrets, bcrypt verification, signed expiry cookies, and login throttling.

Change this file only with explicit security review. Cookie-format changes need versioning and a deliberate logout/migration decision.

## Shared protocol

### `internal/protocol/protocol.go`

This is the binary wire authority. It defines:

- RBR1 constants;
- header size and field encoding;
- JPEG and H.264 frame wrappers;
- the client JSON field union;
- tab metadata.

Any binary change must also update:

- `docs/wiki/03-wire-protocol.md`;
- native protocol parsing;
- web parsing when type 1 changes;
- nativeprobe fixtures/assertions;
- protocol tests;
- the correct version constant.

Never repurpose an existing frame type or fixed field for unrelated semantics.

## Web client map

### `public/index.html`

Defines the browser chrome, canvas, hidden keyboard input, overlays, sheets, and placeholders used to inject token/version values.

### `public/app.js`

One ES5 module owns the web runtime:

- layout and viewport reporting;
- WebSocket connection/reconnect;
- RBR1 parsing and JPEG decode queue;
- mandatory ready acknowledgements and poke watchdog;
- local scrolling prediction and server-offset reconciliation;
- touch, mouse, long-press, pinch, and inertia;
- hidden-input keyboard bridge;
- omnibox suggestions and navigation;
- tabs, menus, history, bookmarks, downloads, and find UI.

The code intentionally avoids modern JavaScript features. Syntax support is constrained by iOS 6 Safari, not the build machine.

### `public/style.css`

Owns responsive browser chrome and all legacy-prefixed presentation. UI changes must be checked in both orientations and with the software keyboard visible.

## Native client map

The native client is split by responsibility under `native/client/Classes`:

| Area | Representative classes |
|---|---|
| Session/auth/reconnect | `RBSession` |
| WebSocket transport | `RBSocket` |
| Wire parsing | `RBProtocol` |
| JPEG pacing/decode | `RBFrameQueue`, `RBJPEGDecoder` |
| H.264 decode | `RBVideoDecoder`, `rb_h264` helpers |
| Rendering | `RBStreamView` |
| Gestures/input | `RBInputController` |
| Keyboard/clipboard | `RBKeyboardShim` and controller integration |
| Browser chrome | `RBChromeController`, `RBTabStrip` |
| Settings/sheets | settings and sheet controller classes |
| Diagnostics | `RBLog`, debug overlay |

The native implementation has strict thread boundaries: network parsing off-main, JPEG decode on a serial queue, and UIKit/CALayer mutation on main.

## Common change recipes

### Add a new client-to-server command

1. Add any fields to `protocol.ClientMessage`.
2. Handle the message in `Browser.HandleMessage` or `handleFeatureMessage`.
3. Emit exactly the same shape from each participating client.
4. Decide whether web, native, or both consume the feature.
5. Bump `ClientVersion`, `NativeVersion`, or both when compatibility requires it.
6. Add protocol/input tests and probe coverage.
7. Update the wire-protocol wiki.

### Add a server-to-client JSON message

1. Define the authoritative shape near the producer.
2. Make parsers tolerate unknown messages and fields.
3. Implement each intended client consumer.
4. Version-gate any client that would malfunction rather than safely ignore it.
5. Record it in `03-wire-protocol.md`.

### Add a native-only binary frame type

1. Allocate the next unused type byte; never reuse one.
2. Add an encoder in `internal/protocol`.
3. Route only to `Client.Native` recipients.
4. Keep it out of the JPEG ack window unless its semantics explicitly require acknowledgements.
5. Add native parser/decoder support and conformance fixtures.
6. Bump `NativeVersion` on server and app together.

### Change viewport or zoom behavior

Inspect all three coordinate spaces:

- client view pixels;
- normalized wire fractions;
- zoomed page CSS pixels.

Then inspect the video surface separately: X display size and H.264 coded size. Test portrait, landscape, zoomed click accuracy, keyboard avoidance, and encoder resize.

### Change JPEG flow control

Treat server and client as one state machine. Every received frame consumes a server inflight slot until acknowledged, even when the client skips it. Update web and native implementations and add a deliberate frame-drop test.

### Change H.264 backpressure

Preserve dependency correctness. Queue overflow or socket loss means all following P-frames are unusable until an IDR. Test with a stalled consumer and require clean desktop decode after recovery.

### Change authentication

Review login, cookie, native config, token persistence, WebSocket upgrade, reverse proxy behavior, and client reauthentication together. Avoid placing the password itself in native defaults or logs.

## Review checklist

For any nontrivial change, ask:

- Does this alter the wire or only implementation?
- Which clients can observe it?
- Is a version bump required?
- Can a slow client block CDP or another client?
- Is a mutex held across I/O or CDP?
- Does reconnect restore a valid state?
- Does a resize or tab switch leave stale encoder/session state?
- Is persistent data backward compatible?
- Are web and native modes safe simultaneously?
- What can automation verify, and what still requires the physical iPad?
