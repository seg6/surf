# Client architecture

Surf has a Go backend and native clients. Shared client behavior lives in the
C99 library under `client/core`. Platform code handles operating system APIs
and presentation.

## Layout

```text
client/
  core/       C99 behavior and protocol
  desktop/    Rust and Dear ImGui host
  ios/        Objective C and UIKit host
```

The iOS toolchain remains under `native/buildenv`.

## Responsibilities

| Area | C99 core | Platform host |
| --- | --- | --- |
| Control protocol | Decode events and encode commands | Transport bytes |
| Browser state | Reduce events into snapshots and effects | Draw snapshots and perform effects |
| Reconnect | Choose timing and state transitions | Run timers and network operations |
| TLS and identity | Model results | Use platform TLS and key storage |
| Media | Validate frames and choose admission or recovery | Decode and present video, play audio |
| Input | Order and normalize samples | Read platform events |
| UI and services | Request work through effects | Layout, clipboard, files, dialogs, and notifications |
| Diagnostics | Calculate rolling health | Display and export results |

On iOS, `RBSecureHTTPClient`, `RBSocket`, and `RBTunnelPipe` handle
transport. `RBDeviceIdentity` uses Security and Keychain.
`RBVideoDecoder`, `RBSampleBufferRenderer`, and `RBStreamView` handle
video. `RBAudioPlayer` handles AudioQueue output. UIKit handles the interface
and lifecycle.

The desktop host provides the same platform services with Rust, FFmpeg, OpenGL,
CPAL, winit, and Dear ImGui.

## Runtime

```text
platform action -> control core -> snapshot and effects -> platform adapters

WebSocket bytes -> frame parser -> media policy -> decoder -> renderer
```

The control core has one owner. It accepts typed events, updates state, and
queues effects. Platform operations run after dispatch and return their results
as later events.

Media does not pass through the UI reducer. The media path tracks connection
and encoder generations, sequences, gaps, frame age, and recovery. Decoders use
the original payload buffer. Small metrics are sent to the control path at
intervals.

Separating these paths prevents UI work from blocking stream ingress and
decode.

## API

The ABI follows this shape:

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

Public structures carry a size or version where later extension needs one.
Strings use pointer and length views. Collections have configured limits.
Snapshot and effect lifetimes are explicit. Media buffers remain with the host.
Tests can replace the allocator.

## Migration record

The portable core now handles framing, H.264 helpers, protocol codecs, browser
state, connection epochs, input sequencing, rich state cleanup, media policy,
and diagnostics. Both the UIKit and Linux clients use it.

The UIKit source moved from `native/client` to `client/ios` after the core
boundary was established. Build and packaging paths moved with it.

See [Porting Surf Clients](porting-clients.md) for the host contract and
[Versioning](versioning.md) for network and ABI versions.
