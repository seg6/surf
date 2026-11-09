<p align="center">
  <img src="backend/cmd/surf/surf-icon.png" alt="Surf icon" width="80" height="80">
</p>

<h1 align="center">Surf</h1>

<p align="center">
  Chromium on your computer. A native client on the device. Pinned TLS between them.
</p>

<p align="center">
  <a href="https://github.com/seg6/surf/releases/latest">Download the host</a> | 
  <a href="https://seg6.space/surf/">Website</a> | 
  <a href="docs/index.md">Docs</a>
</p>

Surf runs the page in Chromium on Windows, macOS, or Linux and streams video and
audio to a client. The client draws a normal browser chrome (tabs, address bar,
library) and sends touch, pointer, keyboard, and files back. The device's own
web engine is not used for page content.

The repo is a Go host, a typed WebSocket protocol, a shared C99 client library,
and native clients. Today that means a jailbroken iOS app (tested on an iPad
mini 1 / iOS 6.1.3; packages cover iOS 6-14) and a Linux Dear ImGui client
(X11 session tests). [Porting](docs/clients/porting.md) covers building another.

## Demo

https://github.com/user-attachments/assets/5e684036-9bbf-4fce-a088-0d95dcc1a890

50 seconds. Unmute for audio. [Video file](docs/media/surf-demo.mp4?raw=true) | 
[Credits](docs/media/README.md)

## Layout

| | |
| --- | --- |
| Host (`surf`) | Chromium, pairing, TLS API, tab capture, tray or `surf serve` |
| Clients | [iOS](docs/clients/ios.md), [Linux desktop](docs/clients/desktop.md) |
| `client/core` | Shared session, browser state, media policy (C99) |
| Protocol | JSON control + binary `RBR1` media on one WebSocket |

## Quick start

**Host** - grab a [release](https://github.com/seg6/surf/releases/latest):

```sh
./surf serve
surf pair
```

Default port `18080`. Data lives in `SURF_HOME` (keep it across updates). Tray
builds add settings, devices, logs, clipboard sync, and updates. Sign in on the
computer with **Browser setup...** / `surf browser` ([guide](docs/guide/browser-setup.md)).

**Client** - iOS: add `https://seg6.space/surf/` in Cydia/Sileo
([install](docs/guide/install-ios.md), [devices](docs/clients/ios.md)).
Linux: [desktop client](docs/guide/install-desktop.md).

**Pair** - QR, or `host:port` + six-digit code, then match the six words on both
ends. Off-LAN: [configuration](docs/guide/configuration.md#remote-access),
[security](docs/reference/security.md).

## Host notes

64-bit Windows, macOS 12+, or Linux (x86_64 / ARM). Uses installed Chrome, Edge,
or Chromium when suitable; otherwise a verified Chromium under `SURF_HOME`.
Capture is Chromium tab capture. No Widevine bundle.

## Docs and build

[Docs index](docs/index.md) | [Overview](docs/guide/overview.md) | 
[Architecture](docs/concepts/architecture.md) | [Protocol](docs/reference/protocol.md) | 
[Changelog](CHANGELOG.md)

```sh
make surf-binary
make surf-dist
make test
```

iOS `.deb`: [native build](docs/develop/native-ios.md). Releases:
[build](docs/develop/build.md), [releasing](docs/develop/releasing.md).

## Support / License

[Ko-fi](https://ko-fi.com/seg6_). Surf is AI-assisted; direction, device testing,
and releases are human-owned. [MIT](LICENSE).
