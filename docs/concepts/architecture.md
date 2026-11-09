# Architecture

Go host, native clients, shared C99 core. Chromium is a subprocess the host
owns (except during [browser setup](../guide/browser-setup.md)).

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
| UI + decode   |  HTTPS /api/v1 | identity, auth, hub           |
| C99 core      |  WebSocket JSON | browser.Manager               |
|               |  WebSocket RBR1 | media capture + pipelines     |
+----------------+                |        | CDP                 |
                                  |   Chromium + extensions       |
                                   +-------------------------------+
```

**Host:** identity under `SURF_HOME`; auth and WebSocket tickets; Chromium
lifetime (streaming vs setup) and profile lock; CDP / input / downloads; tab
capture -> `RBR1`; loopback admin + optional Bonjour.

**Client:** discover/pair/pin; authenticate; dispatch into `client/core`;
decode media off the UI thread; normalize input to protocol commands.

**Core:** codecs, browser/tab state, input order, media admission/recovery,
diagnostics, with no platform calls. Details:
[Client core](client-core.md). Porting: [Porting](../clients/porting.md).

## Isolation

- Do not hold browser tab locks across CDP round-trips.
- Media frames do not enter the UI reducer.
- Raw capture never crosses the transport hub unframed.
- Admin routes are loopback-only; device routes need a session.

[Media](media.md) | [Browser lifetime](browser-lifetime.md) | 
[Protocol](../reference/protocol.md)
