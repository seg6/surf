# Operations and Maintenance

This page is the runbook for building, configuring, deploying, testing, and debugging rbrowser.

## Runtime model

The production process is intentionally supervised rather than self-healing in place:

1. `start.sh` starts Xvfb on `:99`.
2. `rbrowser` launches headful Chromium against that display.
3. The Go process serves HTTP/WebSocket traffic and controls Chromium over CDP.
4. If Chromium or the DevTools socket dies, the Go process exits fatally.
5. The container supervisor restarts the whole unit with a clean control plane and the persistent profile intact.

That failure model is simpler and safer than attempting to reconstruct every tab, session, screencast, and encoder subscription after an in-process Chromium restart.

## Build and run

### Go verification

```sh
go build ./...
go vet ./...
go test ./...
```

When the web client changes:

```sh
node --check public/app.js
```

### Container build

The multi-stage `Dockerfile` produces a static Go binary and a Debian runtime containing Chromium, Xvfb, xrandr utilities, ffmpeg, fonts, and the native libraries Chromium needs.

```sh
docker build -t rbrowser .
docker run --rm -p 8080:8080 \
  -v rbrowser-data:/data \
  -e AUTH_HASH='<bcrypt hash>' \
  rbrowser
```

Persist `/data`: it contains the Chromium profile, authentication secrets, browser history, bookmarks, cookies, and normally downloads.

## Configuration reference

All configuration is environment-driven.

| Variable | Default | Purpose |
|---|---:|---|
| `PORT` | `8080` | HTTP and WebSocket listen port |
| `CHROME` | `/usr/bin/chromium` | Chromium executable |
| `START_URL` | `https://duckduckgo.com` | URL for the first or replacement tab |
| `PROFILE` | `/data/profile` | Chromium profile and rbrowser persistence root |
| `VW`, `VH` | `1024`, `768` | Initial remote viewport |
| `XFB_W`, `XFB_H` | max of `VW`/`VH` | Xvfb framebuffer bounds; must cover every supported orientation |
| `QUALITY` | `60` | Steady-state web JPEG quality |
| `MOTION_QUALITY` | `40` | Web JPEG quality during interaction |
| `MOTION_SCALE` | `0.5` | Web motion-frame resolution factor; accepted range is greater than `0.2` through `1.0` |
| `SHARP_QUALITY` | `82` | Post-settle screenshot quality |
| `SETTLE_MS` | `180` | Quiet period before restoring a sharp frame |
| `NATIVE_QUALITY` | `100` | Native steady-state JPEG quality |
| `NATIVE_MOTION_QUALITY` | `85` | Native motion JPEG quality |
| `AUTH_HASH` | bundled development hash | bcrypt password hash; override in every real deployment |
| `AUTH_DAYS` | `180` | Authentication-cookie lifetime |
| `ADBLOCK` | enabled unless `0` | Enable Fetch-domain blocking |
| `DOWNLOADS` | `/data/downloads` | Download storage |
| `STREAM_FPS` | `24` | H.264 encoder frame rate |
| `STREAM_SCALE` | empty | Optional coded size such as `960x720`; empty follows viewport |
| `STREAM_BITRATE` | `2200` | x264 target bitrate, kbit/s |
| `STREAM_MAXRATE` | `3000` | x264 maximum rate, kbit/s |
| `STREAM_BUFSIZE` | `800` | VBV buffer, kbit |
| `STREAM_PRESET` | `superfast` | x264 preset |

### Tuning guidance

Change one rendering parameter at a time and measure both ends:

- server CPU and encoder stability;
- frame gaps and WebSocket backpressure;
- client decode time, heat, and visual quality;
- recovery after a network stall.

For the JPEG lane, reducing `MOTION_SCALE` often improves perceived responsiveness more than reducing quality. For the H.264 lane, bitrate, frame rate, coded size, and preset trade CPU for motion quality. Preserve the fixed IDR cadence and repeat-headers behavior unless the client protocol changes with it.

## Persistent files

Under `PROFILE`:

| Path | Contents |
|---|---|
| Chromium profile files | Cookies, site storage, session data, preferences |
| `.authsecret` | HMAC key used to sign login-cookie expiries |
| `.wstoken` | Token embedded in authenticated clients and used for WebSocket upgrade gating |
| `history.jsonl` | Append-only browsing history, bounded in memory to 5,000 entries |
| `bookmarks.json` | Bookmark list |

Under `DOWNLOADS`:

- Chromium initially writes files using its download GUID.
- rbrowser records and sanitizes the suggested filename.
- on completion, the GUID file is renamed to a deduplicated final name.
- authenticated clients retrieve it through `/downloads/<name>`.

Back up the profile and downloads together. Losing the profile also rotates the WebSocket token and cookie-signing secret, forcing clients to authenticate again.

## HTTP surface

| Route | Authentication | Purpose |
|---|---|---|
| `GET /health` | none | Process-level liveness; returns `ok` |
| `GET/POST /login` | none | Password form and bcrypt verification |
| `GET /logout` | none | Clears the auth cookie and redirects |
| `GET /` | cookie | Embedded web client with injected token and client version |
| `GET /app.js`, `/style.css` | cookie | Embedded static client assets |
| `GET /native-config` | cookie | Native token, viewport, host, and native protocol version |
| `GET /ws?...` | query token + version | WebSocket upgrade for web or native clients |
| `GET /tabicon/<id>` | cookie | Cached active tab favicon |
| `GET /downloads/<name>` | cookie | Completed download |

`/health` proves the Go HTTP server is responsive; it does not prove Chromium can render, CDP is alive, or frames reach a client. Use a protocol probe for end-to-end health.

## Authentication and exposure

The application provides a password gate, not transport encryption.

- Passwords are checked against a bcrypt hash.
- Successful login creates an HMAC-signed expiry cookie.
- Login attempts are limited to five per minute per source IP.
- The WebSocket uses a persisted random token because legacy iOS clients and proxies cannot be trusted to forward cookie or Basic-Auth state during upgrade.
- Web and native protocol versions are independently gated.

Put TLS and public-network policy in a reverse proxy or private network. Treat anyone who obtains the WebSocket token as able to control the browser until the token is rotated. Rotation is accomplished by replacing or deleting `PROFILE/.wstoken` while the service is stopped, then restarting and re-authenticating clients.

## Logs worth recognizing

Healthy lifecycle messages include:

- browser ready, with viewport, display, quality, and profile;
- tab attached and active-tab transitions;
- web or native WebSocket connection with its version;
- cast start/park/restart parameters;
- video subscriber and encoder lifecycle;
- screen resize and stream encoder resize.

Important warnings:

| Log symptom | Likely meaning | First checks |
|---|---|---|
| `chromium did not expose a DevTools endpoint` | Chromium failed during startup | Chromium stderr, profile permissions, X display, shared memory |
| `chromium connection lost` | Browser or CDP socket died | container OOM/crash logs; allow supervisor restart |
| `ws rejected: bad token or version` | stale client, rotated token, or wrong endpoint | `/native-config`, injected web version, proxy query handling |
| `startCast ... failed after retries` | page session is navigating, detached, or CDP is unhealthy | tab/session lifecycle and CDP latency |
| `captureScreenshot ... slow` | Chromium compositor or server CPU is saturated | Xvfb state, CPU, page load, background-throttling flags |
| `stream: sub lagging, dropping to next IDR` | native consumer or network cannot keep up | client decode/network, bitrate/fps, WebSocket outbox |
| `stream: encoder crashed twice within a minute` | ffmpeg/x264 lane is disabled for current subscribers | ffmpeg stderr, display geometry, resource pressure |
| repeated client `poke` | JPEG acks or frame delivery stalled | client must ack every type-1 frame, including discarded frames |

## Troubleshooting by symptom

### Login works but WebSocket returns 403

1. Confirm the client uses the token from the current authenticated page or `/native-config`.
2. Confirm it sends exactly one matching version: `v=<ClientVersion>` for web or `nv=<NativeVersion>` for native.
3. Check whether the persistent profile was replaced, which rotates `.wstoken`.
4. Verify the reverse proxy preserves query parameters on the upgrade.

### Screen freezes after roughly three frames

The JPEG ack window is exhausted. The client must send `{"t":"ready"}` for every received type-1 frame, even if it skips decoding or replaces it with a newer frame. A missing ack is a protocol bug, not a reason to increase the server window.

### Scrolling is responsive but text remains soft

Check that:

- input calls `beginMotion`;
- the settle timer is firing;
- the active tab leaves motion mode;
- `sendFreshFrame` succeeds;
- a video-mode client accepts the sharp JPEG exception and overlays it;
- `SETTLE_MS` is not excessively high.

### Video starts green, smeared, or undecodable after loss

A dropped P-frame invalidates dependent frames. Confirm the subscriber enters drop mode, waits for the next IDR, and that x264 emits SPS/PPS before every IDR. Never resume on an arbitrary P-frame.

### Video is cropped after rotation

There are three related sizes:

1. client viewport (`VW`/`VH` at runtime),
2. Chromium emulated page viewport,
3. X display / ffmpeg coded surface.

Confirm the client sent `size`, `applyView` ran, `syncScreen` was attempted, and `Streamer.SetSize` restarted the encoder. `XFB_W`/`XFB_H` must be large enough for both orientations even when RANDR cannot create a new mode.

### A page is stuck while ad blocking is enabled

Every `Fetch.requestPaused` event must be answered. The CDP event dispatcher must remain unblocked, and allowed requests must receive `Fetch.continueRequest`. Temporarily set `ADBLOCK=0` to isolate list false positives from event-loop or request-continuation bugs.

## Verification matrix

| Layer | Automated verification | Human/device boundary |
|---|---|---|
| Go packages | build, vet, unit tests | none |
| Web syntax | `node --check public/app.js` | old Safari behavior and feel |
| Auth/handshake | web protocol probe and `nativeprobe` | credential entry UX |
| JPEG wire | header checks, JPEG decode, ack behavior | perceived latency and heat |
| H.264 wire | capture AUs, desktop ffmpeg decode, IDR/SPS/PPS assertions | private VideoToolbox behavior on iOS 6 |
| Native build | reproducible containerized `.deb` build | installation, launch, UIKit behavior |
| Deployment | health, logs, web and native probes | real network and device soak |

Never report iPad behavior as verified from server tests alone. Device acceptance is explicitly human-run.

## Deployment discipline

The repository's native plan records a deliberately conservative production flow:

1. sync from an absolute, known source path;
2. do not use a broad `--delete` from an uncertain working directory;
3. rebuild and restart with the container supervisor;
4. inspect startup logs;
5. run both web and native protocol probes;
6. verify the web client remains unaffected by native-server changes.

Documentation here describes the pattern rather than embedding one operator's hostnames. Keep environment-specific commands in private infrastructure documentation.

## Maintainer invariants

Before changing concurrency, streaming, or protocol code, preserve these rules:

1. Never hold `Browser.mu` across a blocking `cdp.Call`.
2. Never block CDP response delivery or the event-dispatch path.
3. Route every client write through its single outbox/writer goroutine.
4. Keep client input coordinates as viewport fractions.
5. Ack every received type-1 frame.
6. Coalesce JPEG backlog to the newest pending frame.
7. After dropping any H.264 AU, resume only at an IDR.
8. Never send native-only binary frame types to web clients.
9. Bump the appropriate protocol version for client-visible changes.
10. Keep the web client working when adding native capabilities.

## Implemented versus reserved

Current code implements:

- type-1 JPEG frames;
- type-3 H.264 access units;
- web and native version gates;
- native video mode with JPEG fallback and sharp settle overlays.

Reserved or historical material in planning documents must not be mistaken for shipped behavior:

- type-2 region JPEG is reserved, not implemented;
- type-4 audio is not implemented;
- the decision log explicitly removes audio from current native scope;
- adaptive `perf` reporting remains future work unless code is added.

When prose and code disagree, the executable protocol and current decision log are authoritative; update both together when behavior changes.
