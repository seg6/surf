# Client Architecture

Surf consists of a computer-side Go backend and one or more remote browser
clients. The native iOS application lives under `client/ios`. This document
records the portable-client boundary established by the rework.

## Platform hosts

The current client has several strong platform components:

- `RBSecureHTTPClient`, `RBSocket`, and `RBTunnelPipe` implement old-iOS HTTP,
  pinned TLS, WebSocket, and tunneled transport behavior.
- `RBDeviceIdentity` uses Security/Keychain for a per-server RSA identity.
- `RBVideoDecoder`, `RBSampleBufferRenderer`, and `RBStreamView` implement the
  old-iOS video decode and stable presentation paths.
- `RBAudioPlayer` owns AudioQueue output.
- UIKit controllers implement phone/tablet chrome, pairing, settings, library,
  dialogs, selects, sharing, and platform lifecycle.

Portable responsibilities now enter those components through explicit core
APIs:

| Former platform owner | Portable owner now |
| --- | --- |
| `RBProtocol` | `client/core` validates the 84-byte `RBR1` envelope |
| `rb_h264` | `client/core` owns Annex-B/configuration helpers |
| `RBInteractionTracker` | `client/core` owns causal IDs, timestamps, and input sequencing |
| `RBSession` | `client/core` owns connection epochs and reconnect policy |
| `RBRootViewController` | `client/core` owns tabs, navigation, loading, title, security, and editable state |
| `RBMediaPipeline` | `client/core` owns generation/gap/recovery admission policy |
| `RBDiagnostics` | `client/core` owns rolling metrics and health classification |

`RBRootViewController` still coordinates UIKit presentation, as intended. It
renders copied semantic snapshots and executes platform effects; original wire
bytes are strictly decoded in C before UIKit receives a valid control event.
Rich collection and modal widget models remain host-owned adapters because
their lifetime and interaction mechanics are platform presentation concerns.

## Repository layout

```text
client/
  core/       portable C99 behavior and protocol
  desktop/    Rust/egui desktop host, initially certified on Linux
  ios/        Objective-C/UIKit old-iOS host
```

The UIKit host was moved from `native/client` to `client/ios` only after the
core build boundary stabilized. The history-preserving move updated Theos,
buildenv, packaging, documentation, and CI paths together; toolchain support
continues to live under `native/buildenv`.

## Runtime boundaries

```text
                         semantic actions
  UIKit or egui  --------------------------------->  control core
       ^                                                   |
       |                 immutable snapshot                |
       +---------------------------------------------------+
                                                           |
                                                  effects to execute
                                                           |
       +---------------------------------------------------+
       v                                                   v
 platform adapters <------------------------------- operation results

 WebSocket binary -> frame parser/media policy -> platform decoder -> renderer
```

### Control core

The control core is single-owner and low-frequency. It receives typed events
for user intent, control messages, transport results, timers, lifecycle, and
platform-operation results. It updates deterministic state and queues effects.
It never calls a platform callback while dispatching.

### Media lane

The media lane is separate from UI state. It validates frame headers and
tracks generations, sequences, gaps, frame age, and recovery decisions on the
socket/decoder path. Platform decoders and renderers consume the original
payload buffer. Only periodic small metrics enter the control/UI lane.

This separation is required to keep opening an omnibox, changing keyboard
state, resizing chrome, or presenting a sheet from stalling a 60 FPS stream.

## Core API shape

The exact ABI will evolve behind tests, but it follows these rules:

```c
typedef struct surf_core surf_core_t;

surf_core_result_t surf_core_create(const surf_core_config_t *config,
                                    surf_core_t **out_core);
void surf_core_destroy(surf_core_t *core);

surf_core_result_t surf_core_dispatch_scoped(surf_core_t *core,
                                             uint64_t connection_generation,
                                             const surf_event_t *event);
int surf_core_next_effect(surf_core_t *core, surf_effect_t *out_effect);
surf_core_result_t surf_core_snapshot(const surf_core_t *core,
                                      surf_snapshot_t *out_snapshot);
```

- Public structs begin with a size/version field where ABI extension requires
  it.
- Wire strings are pointer/length views and never implicitly null-terminated.
- Collections have configured upper bounds.
- Snapshot/effect lifetime is explicit.
- Large media buffers remain owned by their platform host.
- Core allocation can be injected for failure and constrained-device tests.

## Platform ownership

| Responsibility | Core | iOS host | Desktop host |
| --- | --- | --- | --- |
| Protocol schema and validation | Yes | Adapter only | Adapter only |
| Browser/session state policy | Yes | Snapshot consumer | Snapshot consumer |
| Reconnect decision and delay | Yes | Timer/network effect | Tokio timer/network effect |
| HTTP/WebSocket implementation | No | Existing native code | Rust async transport |
| TLS fingerprint policy result | Modeled | Security/CFNetwork | rustls verifier |
| Device-key operations | Requested | Keychain/SecKey | Rust identity store |
| H.264 frame admission | Yes | Calls core media API | Calls core media API |
| H.264 decode | No | VideoToolbox | FFmpeg initially |
| Video presentation | No | sample-buffer/OpenGL | egui OpenGL callback |
| Audio output | No | AudioQueue | desktop audio adapter |
| UI layout and theme | No | UIKit | egui |
| Clipboard/files/dialog widgets | Requested | UIKit services | desktop services |

## Completed initial migration

1. Build/test guardrails and ownership documentation.
2. Binary framing and pure-C H.264 helpers.
3. Complete two-way Go/C typed protocol contracts.
4. Browser reducer, effects, immutable snapshots, and connection epochs.
5. iOS shadow comparison followed by navigation/editable-state cutover.
6. Raw plus safe Rust bindings and direct YUV OpenGL presentation.
7. Genuine Linux pairing, pinned transport, bounded media, and browser UI.
8. Atomic history-preserving UIKit relocation to `client/ios`.

No step requires a trace file or simulated connection for normal use. Trace
capture may support deterministic tests, but the desktop milestone is a real
Surf client paired to an ordinary backend.
