# Porting Surf Clients

Surf's client is a real interactive browser host, not a recording or replay
program. A new platform host pairs with an ordinary Surf backend, authenticates,
decodes the live stream, sends input, and presents platform-native browser UI.
The reusable boundary is the C99 behavior core under `client/core`.

## Repository and dependency direction

```text
client/ios      Objective-C/UIKit adapters  ─┐
                                              ├─> client/core (C99)
client/desktop  Rust/egui adapters          ─┘

backend (Go) <──── pinned HTTPS + WebSocket + typed protocol ────> host
```

`client/core` has no UIKit, Rust, JNI, Win32, socket, TLS, decoder, renderer,
filesystem, or thread dependency. It owns deterministic semantics; hosts own
operating-system mechanisms and pixels.

## Core contract

A host must provide one single-owner control lane and separate bounded media
lanes.

### Control lane

1. Create `surf_core_t` with the current ABI and platform-appropriate limits.
2. Begin a new monotonically increasing connection generation after each fresh
   authenticated session.
3. Strictly decode original WebSocket JSON with a caller-owned protocol
   workspace, then dispatch the typed event with that connection generation.
4. Copy snapshot strings/collections before the next dispatch if the platform
   UI retains them.
5. Drain effects and execute them outside the reducer call.
6. Report platform-operation completion for dialogs, selects, file choice,
   clipboard, and toasts.
7. Discard host-only transient UI whenever the connection generation or active
   page changes.

Large collection entries and widget layout remain host-owned typed copies. The
core owns presence, identity, counts, control values, completion, and cleanup,
so a late event cannot resurrect an old modal or page state.

### Media lane

1. Validate every `RBR1` envelope before inspecting its payload.
2. Feed video generation, sequence, and IDR metadata into the core admission
   policy.
3. Keep at most the newest encoded work and newest decoded output; never build
   latency by queueing obsolete frames.
4. Decode off the control/UI lane and upload native YUV planes where possible.
5. Report a page frame as presented only after the renderer actually displays
   the matching connection/video generation and source sequence.
6. Keep audio in an independent bounded callback lane.

Opening a keyboard, showing a dialog, resizing chrome, or blocking a file
picker must not stop socket ingress or decoder progress.

## Platform adapter responsibilities

| Adapter | Required behavior |
| --- | --- |
| Discovery | DNS-SD when available, with manual endpoint entry always available |
| Transport | HTTPS and WebSocket with exact Surf certificate fingerprint pinning |
| Identity | Per-server RSA-2048 private key in platform secure storage |
| Persistence | Atomic saved-server records with private permissions |
| Video | H.264 Annex-B decode and stable YUV presentation |
| Audio | Signed 16-bit PCM output through a bounded device callback |
| Input | Platform pointer/touch, key, paste, and IME translation into normalized core samples |
| Services | Clipboard, files, downloads, sharing, lifecycle, and notifications |
| UI | Native or suitable toolkit layout, focus, accessibility, and theme |

The platform must never replace Surf's pin with ambient system trust, share one
private key between unrelated servers, or let a blocking service callback run
on the media ingress lane.

## Bring-up sequence

Each new host should land as independently reviewable vertical slices:

1. **ABI proof:** compile/link the C99 core and compare all binding layouts.
2. **Contract proof:** decode every canonical Go event and encode every command;
   reject malformed, oversized, duplicate-field, and invalid UTF-8 input.
3. **Session proof:** inspect, pair with six-word confirmation, persist identity,
   authenticate, pin TLS, and reconnect to an ordinary backend.
4. **Video proof:** present a real frame with bounded queues and generation/IDR
   recovery.
5. **Input proof:** focus a page control and verify pointer/touch, wheel, key,
   paste, and composition order.
6. **Browser proof:** tabs, omnibox, dialogs, selects, clipboard, uploads,
   downloads, find, settings, and diagnostics are usable without debug flags.
7. **Soak proof:** sustain at least 55 presented FPS during resize, text entry,
   and a deliberate UI stall, then recover after a backend restart.
8. **Package proof:** launch the actual distributable and verify its runtime
   dependencies, icon, metadata, version, and upgrade behavior.

The contract and reducer tests run in CI without a UI. A host-specific test may
drive a real backend and headless renderer; physical-device tests remain the
acceptance gate for hardware-specific decoder and lifecycle behavior.

## Platform support tiers

| Tier | Meaning | Current platforms |
| --- | --- | --- |
| Hardware-verified | Package plus real session accepted on named hardware/OS | UIKit client on iOS 6.1.3; other recorded device runs remain release-specific |
| Integration-verified | Real backend, renderer, input, restart, and package exercised in automation | Linux desktop on X11/x86-64 |
| Build-supported | Code and dependencies compile, but the full runtime matrix is not yet an acceptance claim | Linux Wayland; additional supported iOS slices |
| Port candidate | Adapter design exists; no distributable host exists yet | Android and legacy Windows |

The Linux desktop enables X11, Wayland, AccessKit, clipboard, HiDPI-aware egui
coordinates, and IME support. CI currently runs the real OpenGL/session soak on
X11; Wayland is compile-covered until a compositor-backed job is added.

## Android adapter outline

Use the C ABI through a narrow JNI layer. Keep the private key in Android
Keystore when the OS supports the required RSA operation, use the platform TLS
stack or a pinned Rust/C transport adapter, decode H.264 with `MediaCodec`, and
render through `Surface`/OpenGL ES without a CPU RGBA copy. Compose or Views may
own UI; neither belongs in the C core. Old Android versions may need a bundled
TLS implementation, but exact certificate pinning and per-server key separation
remain mandatory.

## Legacy Windows adapter outline

The first experiment should reuse the Rust desktop session and egui host while
keeping the C99 core unchanged. Evaluate OpenGL availability and FFmpeg runtime
packaging on the oldest target before promising support. A later native Win32
host can use Schannel/WinHTTP or a pinned bundled transport, Media Foundation or
a bundled decoder, WASAPI, and native accessibility. Do not fork protocol or
state policy to accommodate Windows; add a platform adapter or negotiated
capability instead.

The minimum Windows version, decoder choice, and binary distribution strategy
remain release decisions because they depend on hardware/driver testing, not on
the portable architecture.
