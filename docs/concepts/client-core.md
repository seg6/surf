# Client core

`client/core` is a C99 library (`surf_client_core`) shared by Surf clients. It
has no UI, network, TLS, decoder, renderer, audio, filesystem, thread, or Rust
dependency.

## What it owns

1. Control event and command codecs
2. Media frame (`RBR1`) parser helpers and H.264 helpers
3. Session, compatibility, and capability state
4. Browser and tab state reduction
5. Input ordering
6. Media admission and recovery policy
7. Diagnostics calculations

Platform code handles operating system APIs and presentation. Hosts dispatch
typed events, read snapshots and effects, perform platform work, and return
results as later events.

## Responsibilities

| Area | C99 core | Platform host |
| --- | --- | --- |
| Control protocol | Decode events and encode commands | Transport bytes |
| Browser state | Reduce events into snapshots and effects | Draw snapshots and perform effects |
| Reconnect | Choose timing and state transitions | Run timers and network operations |
| TLS and identity | Model results | Use platform TLS and key storage |
| Media | Validate frames and choose admission or recovery | Decode and present video, play audio |
| Input | Order and normalize samples | Read platform events |
| UI and services | Request work through effects | Layout, clipboard, files, dialogs, notifications |
| Diagnostics | Calculate rolling health | Display and export results |

## ABI shape

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

Public headers live under `client/core/include/surf`. Structures carry size or
version fields where extension needs them. Strings are pointer/length views.
Collections have configured limits. Snapshot and effect lifetimes are explicit.
Media parsers return borrowed views; the host keeps the WebSocket buffer. Tests
can replace the allocator. `SURF_CORE_ABI_VERSION` is currently `1`.

## Control vs media

```text
platform action -> control core -> snapshot + effects -> platform adapters

WebSocket bytes -> frame parser -> media policy -> decoder -> renderer
```

The control core has one owner. It accepts typed events, updates state, and
queues effects. Platform operations run after dispatch and return their results
as later events.

Media does not pass through the UI reducer. The media path tracks connection
and encoder generations, sequences, gaps, frame age, and recovery. Decoders use
the original payload buffer. Small metrics are sent to the control path at
intervals. Separating these paths prevents UI work from blocking stream ingress
and decode.

## Build and test

```sh
make client-core-test
make client-core-sanitize
```

```sh
cmake -S client/core -B .local/build/client-core \
  -DCMAKE_BUILD_TYPE=Debug -DSURF_CORE_WARNINGS_AS_ERRORS=ON
cmake --build .local/build/client-core
ctest --test-dir .local/build/client-core --output-on-failure
```

## Hosts

| Host | Binding |
| --- | --- |
| iOS | Objective-C via `RBCoreBridge` |
| Linux desktop | Rust `surf-core` / `surf-core-sys` |

The portable core now handles framing, H.264 helpers, protocol codecs, browser
state, connection epochs, input sequencing, rich state cleanup, media policy,
and diagnostics. Both the UIKit and Linux clients use it. The UIKit source
moved from `native/client` to `client/ios` after the core boundary was
established.

Porting contract: [Porting](../clients/porting.md).
Network and ABI versions: [Versioning](../reference/versioning.md).
