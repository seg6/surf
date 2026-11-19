# Architecture

Surf is a Go host, native clients, and a shared C99 core. The host owns
Chromium as a subprocess, except during [browser setup](../guide/browser-setup.md).

## Layout

```text
backend/          Go host: serve, tray, CDP, media, auth, HTTP API
client/core/      C99 shared client logic (no UI/TLS/decode)
client/ios/       Objective-C / UIKit -> rootful .deb
client/desktop/   Rust + Dear ImGui Linux client
protocol/         Cross-language fixtures
packaging/        Host archives, Windows NSIS, apt repo, site
native/           iOS cross-build toolchain
docs/  site/      Documentation and Astro site
```

## Runtime

```text
+----------------+   pinned TLS    +-------------------------------+
| Native client  |<--------------->| Go host                       |
| UI + decode    |  HTTPS /api/v1  | identity, auth, hub           |
| C99 core       |  WebSocket JSON | browser.Manager               |
|                |  WebSocket RBR1 | media capture + pipelines     |
+----------------+                 |        | CDP                  |
                                   |   Chromium + extensions       |
                                   +-------------------------------+
```

The host keeps its identity under `SURF_HOME` and handles authentication and
WebSocket tickets. It also manages Chromium's lifetime (streaming versus setup)
and the profile lock, drives Chromium through CDP for input and downloads, and
turns tab capture into `RBR1` frames. On the side it runs a loopback admin
interface and, optionally, Bonjour.

The client discovers, pairs with and pins a server, then authenticates. It
dispatches events into `client/core`, decodes media off the UI thread, and
normalizes input into protocol commands.

The core handles the codecs, browser and tab state, input ordering, media
admission and recovery, and diagnostics, all without any platform calls. For
more, see [Client core](client-core.md), and for writing a new client see
[Porting](../clients/porting.md).

## Isolation

These are the rules that keep the pieces from blocking each other:

- Don't hold browser tab locks across CDP round-trips.
- Media frames never enter the UI reducer.
- Raw capture never crosses the transport hub unframed.
- Admin routes are loopback-only, and device routes need a session.

See also [Media](media.md), [Browser lifetime](browser-lifetime.md) and the
[Protocol](../reference/protocol.md).
