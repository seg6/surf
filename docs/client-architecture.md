# Client Architecture

Surf consists of a computer-side Go backend and one or more remote browser
clients. The released client is currently the native iOS application under
`native/client`. This document records its current ownership and the target
portable-client boundary.

## Current client

The current client has several strong platform components:

- `RBSecureHTTPClient`, `RBSocket`, and `RBTunnelPipe` implement old-iOS HTTP,
  pinned TLS, WebSocket, and tunneled transport behavior.
- `RBDeviceIdentity` uses Security/Keychain for a per-server RSA identity.
- `RBVideoDecoder`, `RBSampleBufferRenderer`, and `RBStreamView` implement the
  old-iOS video decode and stable presentation paths.
- `RBAudioPlayer` owns AudioQueue output.
- UIKit controllers implement phone/tablet chrome, pairing, settings, library,
  dialogs, selects, sharing, and platform lifecycle.

Portable responsibilities are currently mixed into those components:

| Current owner | Mixed responsibility to extract |
| --- | --- |
| `RBProtocol` | 84-byte `RBR1` parsing and bounds validation |
| `rb_h264` | Annex-B and H.264 configuration helpers; already pure C |
| `RBInteractionTracker` | causal interaction IDs and timestamps |
| `RBSession` | session generations, compatibility decisions, and reconnect policy |
| `RBRootViewController` | browser/tab/loading/title/editable/dialog state and control-event dispatch |
| `RBMediaPipeline` | frame-generation, sequencing, gap, recovery, and health policy |
| `RBDiagnostics` | portable metric aggregation and health classification |

`RBRootViewController` also coordinates most UIKit presentation. The goal is
not to eliminate that coordination entirely. It should become a platform host
that renders a semantic snapshot and executes platform effects instead of
being the source of browser protocol truth.

## Target repository layout

```text
client/
  core/       portable C99 behavior and protocol
  desktop/    Rust/egui desktop host, initially certified on Linux
  ios/        Objective-C/UIKit old-iOS host
```

The physical move from `native/client` to `client/ios` occurs only after the
core build boundary is stable. It will be one history-preserving commit that
also updates Theos, buildenv, packaging, documentation, and CI paths.

## Target runtime boundaries

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

surf_result_t surf_core_create(const surf_core_config_t *config,
                               surf_core_t **out_core);
void surf_core_destroy(surf_core_t *core);

surf_result_t surf_core_dispatch(surf_core_t *core,
                                 const surf_event_t *event);
int surf_core_next_effect(surf_core_t *core, surf_effect_t *out_effect);
surf_result_t surf_core_snapshot(const surf_core_t *core,
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

## Migration sequence

1. Add build/test guardrails and document the boundary.
2. Extract binary framing and existing pure-C H.264 helpers.
3. Establish Go/C protocol-contract fixtures.
4. Add typed browser reducer, effects, and snapshots.
5. Run the core in iOS shadow mode, compare state, then cut over slices.
6. Add safe Rust bindings and verify custom 60 FPS egui presentation.
7. Build genuine desktop pairing, transport, media, and browser interaction.
8. Move the stable iOS host to `client/ios` atomically.

No step requires a trace file or simulated connection for normal use. Trace
capture may support deterministic tests, but the desktop milestone is a real
Surf client paired to an ordinary backend.
