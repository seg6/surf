# Configuration

You configure the host with environment variables. There's no password
variable, because pairing and device keys take the place of shared secrets.

## Variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `SURF_HOME` | `~/.surf` | Identity, devices, browser profile, downloads, logs |
| `SURF_SERVER_NAME` | `Surf` | Name shown during pairing and in Bonjour |
| `SURF_PUBLIC_ADDRESS` | empty | Address placed in pairing codes |
| `SURF_TUNNEL_HOST` | empty | Hostname for Cloudflare WebSocket transport |
| `SURF_ADVERTISE_IP` | automatic | LAN address advertised by Bonjour |
| `BIND_ADDR` | `0.0.0.0` | Listener address |
| `PORT` | `18080` | TLS and API port |
| `CHROME` | automatic | Chrome, Chromium, or Edge executable |
| `PROFILE` | `$SURF_HOME/profile` | Chromium profile |
| `START_URL` | Google | Initial page |
| `SURF_SEARCH_URL` | empty | Address-bar search template with `{searchTerms}` or `%s`; empty uses the search engine set in Chromium, then Google |
| `SURF_USER_AGENT` | empty | Replace the browser user agent on every tab; empty keeps Surf's own desktop and mobile identities |
| `SURF_VIEWPORT_SCALE` | `1` | Lay pages out in a viewport this many times (1 to 3) the client's size; the stream stays at the client's resolution, so a small screen shows a wider page |
| `SURF_AUDIO_QUALITY` | `high` | `high` streams 48 kHz stereo audio; `standard` streams 16 kHz mono, which uses less bandwidth on slow or remote connections |
| `DOWNLOADS` | `$SURF_HOME/downloads` | Downloads |
| `UPLOADS` | `$SURF_HOME/uploads` | Temporary uploads |
| `VW`, `VH` | `768`, `934` | Initial viewport before the client reports size |
| `STREAM_BITRATE` | `16000` | H.264 variable rate target in kbit/s |
| `STREAM_QUANTIZER` | `12` | H.264 constant quality fallback (0-51) |
| `STREAM_SCALE` | empty | Optional maximum stream size |
| `SURF_CONTENT_BLOCKER` | `1` | Manage uBlock Origin Lite |
| `SURF_ADAPTIVE_VIDEO` | `0` | Enable adaptive frame-rate profiles with `1` |
| `CHROME_NO_SANDBOX` | automatic for root | Disable the Chromium sandbox when required |

The Settings page can also set the search engine, user agent and page width
scale. From the tray you can change the server name, port and related settings,
and restart the backend. Command line tools have to use the same `SURF_HOME` as
the running owner (see `daemon.json`).

## Files under `SURF_HOME`

```text
$SURF_HOME/identity/server.crt
$SURF_HOME/identity/server.key
$SURF_HOME/identity/session.key
$SURF_HOME/devices.json
$SURF_HOME/daemon.json
$SURF_HOME/browser-session.json
```

| Path | Role |
| --- | --- |
| `identity/` | TLS private key and certificate (server identity) |
| `devices.json` | Paired device public keys |
| `daemon.json` | How local CLI reaches the running server (includes control token) |
| `browser-session.json` | Tab restore state |
| `profile/` | Chromium user data |
| `downloads/`, `uploads/` | File transfer areas |
| `logs/` | Server, desktop, and per-device logs |

`daemon.json` tells local CLI commands how to reach the running owner, and it
contains that owner's control token. CLI commands control the running owner,
and `surf browser` only starts a standalone setup when there isn't one.

The desktop app supervises the server through a parent pipe, so closing the app
also closes the backend and the Chromium tree. `desktop.lock` and `server.lock`
use kernel locks, so an empty lock file left behind after exit doesn't hold
the lock.

The rest of `SURF_HOME` holds the browser profile, downloads, uploads, the
managed browser, updates, logs and desktop settings. If you back up the whole
directory, you keep the server identity and your pairings. If you replace only
the identity, saved clients will report Server Identity Changed.

Browser ownership, profile recovery and download staging are covered in
[browser lifetime](../concepts/browser-lifetime.md).

Related pages: [remote access](remote-access.md), [updates](updates.md),
[browser setup](browser-setup.md) and [architecture](../concepts/architecture.md).
