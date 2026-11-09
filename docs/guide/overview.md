# Overview

Surf runs Chromium on a Windows, macOS, or Linux computer. A native client
streams that page (video + audio) and sends input back over pinned TLS. Tabs,
address bar, and library are native; navigation, JS, cookies, and downloads
happen in the host browser.

| Piece | Role |
| --- | --- |
| Host (`surf`) | TLS, pairing, auth, Chromium/CDP, tab capture, local admin |
| Client | Pairing, decode, input, native UI |
| `client/core` | Shared C99 codecs, tabs, input order, media admission |
| Protocol | JSON control + `RBR1` media on one WebSocket |

```text
Client  -- HTTPS / WebSocket (pinned TLS) --  Host -- CDP --> Chromium
        <------- JSON + RBR1 media --------
```

**Clients:** jailbroken iOS 6-14 ([matrix](../clients/ios.md); main test device
iPad mini 1 / 6.1.3), Linux Dear ImGui ([desktop](../clients/desktop.md)).
Other OS targets: [porting](../clients/porting.md).

**Scope:** you run the host (not a cloud service). Pages are real Chromium, not
an HTML proxy. No plaintext mode. Widevine only if the host browser provides a
CDM. Trust model: [security](../reference/security.md).

Install [host](install-host.md) -> [iOS](install-ios.md) or
[Linux](install-desktop.md) -> [pair](pairing.md). Day to day after that:
[Daily use](daily-use.md).
