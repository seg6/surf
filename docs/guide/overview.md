# Overview

Surf runs Chromium on a Windows, macOS or Linux computer. A native client on
another device shows you the page as a video stream with audio, and sends your
input back over a TLS connection pinned to your host. The tabs, address bar and
library are native to the client, while navigation, JavaScript, cookies and
downloads all happen in the browser on the host.

There are four pieces:

- The host (`surf`) handles TLS, pairing, authentication, Chromium through the
  DevTools protocol, tab capture and local administration.
- The client handles pairing, decoding, input and the native UI.
- `client/core` is the shared C99 code for codecs, tabs, input ordering and
  media admission.
- The protocol is JSON control messages plus `RBR1` media, all on one
  WebSocket.

```text
Client  -- HTTPS / WebSocket (pinned TLS) --  Host -- CDP --> Chromium
        <------- JSON + RBR1 media --------
```

The clients today are a jailbroken iOS 6 to 14 app
([iOS notes](../clients/ios.md)), a Linux client
([desktop notes](../clients/desktop.md)), and a community
[PS Vita port](https://github.com/jmverges/surf-vita). If you want to bring
Surf to another platform, start with the [porting guide](../clients/porting.md).

You run the host yourself, so no cloud service is involved. Pages are rendered by real Chromium rather than an HTML
proxy. Connections are always encrypted, and there's no plaintext mode. DRM
video only plays if the browser on the host provides Widevine. The trust model
is covered in [security](../reference/security.md).

To set up, install the [host](install-host.md), then the
[iOS](install-ios.md) or [Linux](install-desktop.md) client, and
[pair](pairing.md) them. [Daily use](daily-use.md) covers the rest.
