# WRP Native — Master Plan

A native Objective-C client for **iOS 6.1.3 on a jailbroken iPad mini 1 (A5, armv7, 512MB, 1024×768 non-retina)**, talking to the existing Go rbrowser server. The web client stays fully supported forever; the native app is an additional, more capable head.

**Why native at all** (in priority order):
1. **H.264 lane** — ffmpeg/x264 grabs the Xvfb display server-side, the A5's hardware decoder (VideoToolbox, private on iOS 6, fine on jailbreak) plays it. Real ~30fps, motion handled by the codec, at a few % of client CPU. This is the payoff; everything else is supporting cast.
2. **Real clipboard** — `UIPasteboard` both directions (remote browser ↔ Notes/Mail).
3. **Audio** — PulseAudio in the container → ffmpeg → AudioQueue. The web client can never have this on iOS 6.
4. **Process dignity** — no Safari tab eviction, silent reconnect, background decode thread, fullscreen, native keyboard/menus.

**Explicit non-goal:** local pan prediction / latency masking via client-side transforms. We built it in the web client, it produced protocol-inherent artifacts (fixed-header flap, end-of-page void), and we removed it. With a real 30fps video lane it is unnecessary. Any agent proposing it should re-read this paragraph.

---

## Document map

| Doc | Covers | Primary phase |
|---|---|---|
| [docs/01-toolchain.md](docs/01-toolchain.md) | Dockerized theos/clang build env, iOS 6.1 SDK, ldid signing, .deb install, debugging on device | 0 |
| [docs/02-protocol.md](docs/02-protocol.md) | RBR1 recap (exact bytes), native extensions: frame types 2–4, control messages, handshake/auth | 1+ |
| [docs/03-server.md](docs/03-server.md) | Go work: `/native-config`, focus rects, `internal/stream` (ffmpeg lifecycle, AU fan-out), pulse audio | 1, 3 |
| [docs/04-client.md](docs/04-client.md) | The ObjC app: threading model, hand-rolled WebSocket, JPEG pipeline, input, keyboard, clipboard, chrome UI | 1, 2 |
| [docs/05-video-audio.md](docs/05-video-audio.md) | x264 flags, Annex-B→avcC, VTDecompressionSession on iOS 6, GL/BGRA render, AudioQueue | 3 |
| [docs/06-testing.md](docs/06-testing.md) | What agents can verify vs. what only the device can; probe harnesses; per-phase device checklists; deploy rules | all |

## Repo layout (target)

```
rbrowser/
  native/
    PLAN.md, docs/            # this plan
    buildenv/
      Dockerfile              # Linux theos+clang+cctools+ldid cross-build image
      sdk/                    # user drops iPhoneOS6.1.sdk here (gitignored, see 01)
    client/                   # theos application project (Objective-C sources)
      Makefile, control, Resources/, Classes/
  internal/stream/            # NEW: ffmpeg H.264/audio lane (phase 3)
  tools/nativeprobe/          # NEW: desktop harness that impersonates the native client
  (everything else unchanged)
```

---

## Architecture

```
┌────────────────────────── Hetzner VPS (Docker) ──────────────────────────┐
│  Xvfb :99 ── headful Chromium (CDP)                                      │
│     │            │                                                       │
│     │            ├─ Page.screencast JPEG ──► internal/browser ─┐         │
│     │            └─ Input/Fetch/Emulation ◄── control msgs     │         │
│     └─ ffmpeg x11grab ► x264 ► Annex-B AUs ► internal/stream ──┤         │
│        (+ pulse monitor ► PCM/AAC)                             ▼         │
│                                                    internal/ws hub       │
└──────────────────────────────────────────────────────│───────────────────┘
                                    ws:// (plain HTTP, port 80 via Caddy)
                ┌──────────────────────┴──────────────────────┐
        web client (canvas, iOS 6 Safari)          native app (this plan)
        JPEG lane only — unchanged                 JPEG lane + H.264 + audio
```

Key insight: **ffmpeg grabs the X display directly**, bypassing CDP entirely. The video lane and the screencast lane coexist without interfering — a web client and the native app can be connected at the same time, each on its own lane.

---

## Phases and gates

Each phase ends with (a) agent-verifiable acceptance criteria and (b) a device checklist the user runs on the iPad. A phase is not done until both pass. Details in docs/06.

### Phase 0 — Toolchain (docs/01)
Dockerized build environment that compiles armv7/iOS-6.0 Objective-C, packages a .deb, pseudo-signs with ldid. Hello-world app installs and launches on the iPad.
- **Agent gate:** `docker build` of buildenv succeeds; hello-world .deb builds reproducibly in it; `lipo -info`/`otool -l` show armv7 + `LC_VERSION_MIN_IPHONEOS 6.0`; ARC smoke object compiles & links (or the MRC fallback decision is recorded in the decision log).
- **Device gate (user):** dpkg -i + uicache; icon appears; app launches; a label renders; no crash log.

### Phase 1 — Core stream (docs/02, 03, 04)
`/native-config` endpoint on the server; hand-rolled WebSocket; RBR1 parsing; background JPEG decode; layer render; tap→click, pan→wheel (+ inertia), `ready` ack protocol. Server otherwise unchanged.
- **Agent gate:** `tools/nativeprobe` connects through the *same code path* the app will use (`/login` → `/native-config` → `/ws`) and receives well-formed frames; go vet/test clean; deployed and probed on prod.
- **Device gate:** browse DuckDuckGo on the iPad: frames render, taps land where the finger is, scroll works with inertia, reconnect survives app switch and Wi-Fi blips.

### Phase 2 — Interaction + chrome (docs/04)
Shadow keyboard (hidden UITextField, per-char + burst paste), Return submits, long-press select/drag with `UIMenuController`, **UIPasteboard both ways**, omnibox + progress + star, tab strip, popover menu, find bar, history/bookmarks/downloads sheets, settings/login screen, fullscreen mode, app icon.
- **Agent gate:** every control message emitted matches what `public/app.js` emits (byte-compare against a recorded session via nativeprobe); .deb builds.
- **Device gate:** type into DuckDuckGo search incl. Return; copy remote text into Notes; paste from Notes into a remote form; tabs/find/history/downloads all function.

### Phase 3 — Video + audio (docs/05, 03)
`internal/stream`: ffmpeg lifecycle, AU splitting, IDR-aware backpressure; client `{"t":"video","on":true}`; VTDecompressionSession decode to BGRA; sharp-JPEG overlay on settle; then PulseAudio → PCM chunks → AudioQueue.
- **Agent gate:** nativeprobe receives AUs, pipes them to desktop ffmpeg, gets valid decoded frames at target fps; server CPU measured under `docker stats` and within budget; web client verified unaffected.
- **Device gate:** scrolling looks like video, not a slideshow; text sharp after settle; audio from YouTube; battery/heat acceptable over 15 min.

### Phase 4 — Polish
Decode-time adaptive quality (client reports, server adapts fps/bitrate), jetsam hardening, debug overlay parity (fps / decode ms / RTT), reconnect edge cases, icon/branding pass.

**Ordering:** 0 → 1 → 2 → 3 → 4 strictly. Phase 3 server work (`internal/stream`) may start in parallel with Phase 2 since it's fully testable with nativeprobe and touches no client code.

---

## Ground rules for agents

1. **Never break the web client.** Every server change must leave `public/app.js` + iOS 6 Safari working. New JSON fields on existing messages are fine (the web client ignores unknown fields); new frame *types* must never be sent to web clients; changed semantics of existing messages are forbidden.
2. **The wire truth is the code**, not this plan's prose: `internal/protocol/protocol.go`, `internal/browser/*.go`, and `public/app.js`. Read them before implementing anything that touches the protocol. Where this plan and the code disagree, the code wins — then fix the plan.
3. **Version gates.** Web: bump `config.ClientVersion` on any client-affecting change. Native: `config.NativeVersion` (new, see docs/02) compiled into the app; `/ws` accepts `v==ClientVersion` **or** `nv==NativeVersion`.
4. **Concurrency invariants (server):** never hold `b.mu` across a `cdp.Call`; never block the CDP event-dispatch goroutine; long-running work spawned with `go`; client writes go through the hub's outbox, never directly.
5. **Input coordinates are viewport fractions (0..1)** in every message, native included. No pixel coordinates on the wire, ever.
6. **All native code must compile inside `native/buildenv`** — an agent finishing client work without a green containerized build hasn't finished. No "should compile".
7. **Memory discipline (client):** 512MB device. At most one encoded frame queued + one decoded image live per lane. Coalesce (drop-oldest) exactly like the web client, and **ack every received frame** even when skipping it — the server's 3-deep inflight window depends on it.
8. **Deploys:** absolute-path rsync only (`rsync -a /Users/null/workspace/personal/rbrowser/ hetzner:workspace/personal/vm/apps/rbrowser/`), then `ssh hetzner "cd workspace/personal/vm/apps/rbrowser && docker compose up -d --build"`. Never `--delete` from any other cwd. Probe after deploy (docs/06).
9. **Device testing is human-only.** End every work package with a concise device checklist. Do not claim device behavior as verified.
10. **No local pan prediction.** See top of this file.

---

## Risk register

| # | Risk | Likelihood | Mitigation |
|---|---|---|---|
| R1 | Toolchain: ARC on deployment target <7 needs `libarclite_iphoneos.a`, which modern toolchains dropped | High | Vendor libarclite next to the SDK (docs/01); Phase-0 gate tests ARC on device; recorded fallback: MRC with strict ownership rules |
| R2 | VideoToolbox is private on iOS 6; headers/behavior may differ from public iOS 8 API | Medium | Known-working jailbreak-era pattern documented in docs/05 (avcC via `CMVideoFormatDescriptionCreate` extensions, BGRA output). Fallback: JPEG lane remains; app auto-falls-back if VT session creation fails |
| R3 | x264 CPU on the shared-vCPU VPS alongside Chromium | Medium | Encoder runs **only** while a native video client is connected; knobs: 15fps default, `superfast`, optional 960×720; measure with `docker stats` before/after (docs/05 budget table) |
| R4 | Hand-rolled WebSocket interop bugs vs gorilla (fragmentation, ping, close) | Medium | Exact framing spec in docs/04; nativeprobe exercises the same spec against the real server; handle continuation frames on receive |
| R5 | iPad decode/render too slow even native (JPEG lane) | Low | Background ImageIO decode + forced decompression off-main; measured budget in docs/04; video lane is the real fix anyway |
| R6 | Old-device debugging friction (no modern lldb attach) | Medium | In-app log file + debug overlay + `NSSetUncaughtExceptionHandler`; retrieve logs/crashes over ssh (docs/01 §6, 06 §4) |
| R7 | Jetsam kills the app under memory pressure | Low–Med | Rule 7 above; respond to memory warnings by dropping every cache; measure RSS in overlay |

## Decision log

| Date | Decision | Why |
|---|---|---|
| 2026-07-15 | Native app approved; iPad confirmed jailbroken (6.1.3, p0sixspwn-class) | Unlocks install + VideoToolbox |
| 2026-07-15 | Extend RBR1, no RBR2 | Header is hdrlen-parsed and has reserved bytes; forking the protocol buys nothing and breaks the shared-server story |
| 2026-07-15 | No local pan prediction in the native client | Empirically failed in the web client for protocol-inherent reasons; video lane obsoletes it |
| 2026-07-15 | Hand-rolled WebSocket over CFStream instead of SocketRocket | Plain-HTTP ws only (no TLS on iOS 6 path anyway); ~400 controlled lines beats dependency archaeology on a 2013 library |
| 2026-07-15 | Linux Docker build environment, not host Xcode | Modern Xcode cannot target armv7/iOS 6; container is reproducible and lets agents actually run the build |
| 2026-07-16 | Phase 0 ARC build does not force-load libarclite | The available archive referenced Objective-C runtime symbols absent from iOS 6; the app links cleanly without it, with the device ARC smoke test as the gate |
| 2026-07-16 | v1 supports portrait and landscape | User wants orientation support day one; native client sends current viewport size on connect/rotation |
| 2026-07-16 | Native clients get full-resolution motion JPEGs | Device testing showed native JPEG path is good enough to avoid low-res motion frames; web-only sessions keep the old low-res motion path |
| 2026-07-16 | Audio removed from native scope | User does not need audio; keep the product focused on fast browsing, clipboard, and optionally video-only later |
| 2026-07-16 | Video lane deferred behind polished JPEG client | Private iOS 6 VideoToolbox is high-risk; current pass prioritizes q100 settled JPEG, full-res q85 motion, accurate viewport resize, and native UX polish |
| 2026-07-16 | Native chrome targets the iOS 6 Safari look, not graphite/gold | User wants "Safari UI, remote backend"; native gradient bars + tab strip + unified omnibox read as a real browser on the device. The graphite/gold language stays web-client-only. Supersedes the docs/04 design-language line |
| 2026-07-16 | Client 0.0.2: full chrome rewrite, wire protocol untouched | Tab strip w/ favicons, omnibox w/ suggest+progress, pinch/double-tap zoom, UIMenuController copy, popover surfaces, settings screen. No new server messages, so NativeVersion stays 20260716-1 (no lockstep deploy needed) |
| 2026-07-16 | Phase 3 video lane shipped (server + client 0.0.3, nv 20260716-2) | internal/stream (ffmpeg x11grab→x264, AUD splitter, IDR-resync backpressure), type-3 frames, hub video-mode routing w/ sharp-settle exception, cast parks when no JPEG consumers; client decodes via dlopen'd VideoToolbox (no link-time dep — sidesteps missing private-framework stubs in the SDK), zero-copy CGImage render, sharp overlay hidden on touch, settings toggle, auto-fallback to JPEG on any failure. Verified by nativeprobe -video against a local container: 15fps, 2s IDR cadence, in-band SPS/PPS, desktop ffmpeg decodes captures with zero errors |
| 2026-07-16 | editable extended with kind+rect (ClientVersion 20260716-1) | Server computes keyboard kind + focused-element rect (viewport fractions) in-page; native maps kind→UIKeyboardType/secure and slides the stream when the keyboard covers the field. Checkbox/radio/button inputs no longer raise the keyboard |

## Open decisions (user input wanted, none blocking Phase 0–1)

- **App name/bundle id**: plan uses `WRP` / `space.seg6.wrp`. Say if you want something else before Phase 0 bakes it in.
