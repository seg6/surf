# Configuration

Host tunables are environment variables. No password variable. Pairing and
device keys replace shared secrets.

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
| `DOWNLOADS` | `$SURF_HOME/downloads` | Downloads |
| `UPLOADS` | `$SURF_HOME/uploads` | Temporary uploads |
| `VW`, `VH` | `768`, `934` | Initial viewport before the client reports size |
| `STREAM_BITRATE` | `16000` | H.264 variable rate target in kbit/s |
| `STREAM_QUANTIZER` | `12` | H.264 constant quality fallback (0-51) |
| `STREAM_SCALE` | empty | Optional maximum stream size |
| `SURF_CONTENT_BLOCKER` | `1` | Manage uBlock Origin Lite |
| `SURF_ADAPTIVE_VIDEO` | `0` | Enable adaptive frame-rate profiles with `1` |
| `CHROME_NO_SANDBOX` | automatic for root | Disable the Chromium sandbox when required |

The tray can change server name, port, and related settings and restart the
backend. CLI tools must use the same `SURF_HOME` as the running owner
(`daemon.json`).

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

`daemon.json` tells local CLI commands how to reach the running owner and
contains its control token. CLI commands control that owner; `surf browser`
starts standalone setup only when there is no running owner.

The desktop app uses a parent pipe to supervise the server. Closing the app
closes the backend and Chromium tree. `desktop.lock` and `server.lock` use
kernel locks; empty lock files after exit do not hold the lock.

The rest of `SURF_HOME` holds the browser profile, downloads, uploads, managed
browser, updates, logs, and desktop settings. Backing up the whole directory
preserves the server identity and pairings. Replacing identity alone makes saved
clients report **Server Identity Changed**.

Browser ownership, profile recovery, and download staging:
[Browser lifetime](../concepts/browser-lifetime.md).

Related: [Remote access](remote-access.md), [Updates](updates.md),
[Browser setup](browser-setup.md), [Architecture](../concepts/architecture.md).
