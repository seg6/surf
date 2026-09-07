# Backend

Surf runs Chromium on a Windows, macOS, or Linux host. The desktop app manages
the backend. `surf serve` runs the same backend in the foreground for a service
manager.

## Start and pair

```sh
./surf serve
./surf status
./surf pair
./surf quit
```

Add `--pair` to `surf serve` to create an invitation at startup.

Pairing is otherwise closed. **Pair device** or `surf pair` creates one
invitation with a QR code and a six digit manual code. It remains open until one
device uses it, it is cancelled, the server restarts, or five wrong manual codes
are entered.

The QR code contains the address, expected server identity, and a random 128 bit
token. Manual pairing uses the address and numeric code, then compares six words
on the host and client. The words must match.

Paired devices can be listed or revoked without restarting Chromium.

```sh
./surf devices list
./surf devices revoke DEVICE_ID
```

Revocation closes the device connections and invalidates its sessions,
challenges, and tickets.

## Network and TLS

First launch creates an RSA 2048 certificate in `SURF_HOME`. Clients save its
SHA 256 fingerprint during pairing. Surf serves TLS 1.2 and later directly and
has no plaintext mode. A public certificate, domain, or reverse proxy is not
required.

A VPS needs an exposed Surf port and a public address in pairing codes.

```sh
SURF_PUBLIC_ADDRESS=surf.example.net:18080 ./surf serve
```

This setting does not provide NAT traversal. The selected port must be allowed
through the host firewall or VPS security group.

For Cloudflare Tunnel, route a hostname to the Surf HTTPS listener and set both
address variables.

```sh
SURF_PUBLIC_ADDRESS=surf.example.net:443 \
SURF_TUNNEL_HOST=surf.example.net \
./surf serve
```

The public WebSocket carries a separate pinned Surf TLS connection. See
[Security](security.md).

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `SURF_HOME` | `~/.surf` | Identity, devices, browser profile, downloads, and logs |
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
| `VW`, `VH` | `768`, `934` | Initial viewport before the client reports its size |
| `STREAM_BITRATE` | `16000` | H.264 variable rate target in kbit/s |
| `STREAM_QUANTIZER` | `12` | H.264 constant quality fallback from 0 to 51 |
| `STREAM_SCALE` | empty | Optional maximum stream size |
| `SURF_CONTENT_BLOCKER` | `1` | Manage uBlock Origin Lite |
| `SURF_ADAPTIVE_VIDEO` | `0` | Enable adaptive video with `1` |
| `CHROME_NO_SANDBOX` | automatic for root | Disable the Chromium sandbox when required |

There is no password variable.

## Files

These files contain the server identity and runtime state.

```text
$SURF_HOME/identity/server.crt
$SURF_HOME/identity/server.key
$SURF_HOME/identity/session.key
$SURF_HOME/devices.json
$SURF_HOME/daemon.json
$SURF_HOME/browser-session.json
```

`daemon.json` tells local CLI commands how to reach the running server and
contains its control token. CLI commands do not start another backend.

The desktop app uses a parent pipe to supervise the server. Closing the app
closes the backend and Chromium tree. `desktop.lock` and `server.lock` use
kernel locks. Empty lock files may remain after exit.

The rest of `SURF_HOME` contains the browser profile, downloads, uploads,
managed browser, updates, logs, and desktop settings. Backing up the whole
directory preserves the server identity and pairings.

Chromium starts and stops with the backend. Capture and transport stop when the
last client disconnects. If Chromium or its DevTools connection exits, the
backend exits so its supervisor can restart both.

Tabs, the active tab, website mode, appearance, cookies, and site storage are
restored after restart. Tab state is written after changes and once more during
a clean shutdown.

Malformed replaceable state is moved beside the original with an
`.invalid-<timestamp>` suffix. After two Chromium startup failures within ten
minutes, Surf preserves the old profile as
`profile.startup-failed-<timestamp>` and starts a clean profile with Surf
bookmarks and history. TLS identity files are never replaced by this recovery.

## Downloads

Chromium writes unfinished downloads into a private session directory under
`DOWNLOADS/.incomplete/`. When a download finishes, Surf publishes it into
`DOWNLOADS` without overwriting an existing file. Concurrent downloads with
the same filename receive different names. Only completed, non-hidden regular
files appear in Library or can be fetched through the downloads API.

Setup and filesystem failures are reported instead of announcing success.
Files left behind by a crash or failed publication remain in `.incomplete`
for manual recovery; Surf does not automatically resume them. A completed file
kept there after a publication error is named with Chromium's download ID;
the server log identifies its path.

The file endpoint supports authenticated GET, HEAD, byte ranges and ETags.
Desktop saves use unique temporary files and never overwrite local copies.
The iOS client uses bounded ranges and a unique local directory for each copy.

## Logs

Host logs rotate under `SURF_HOME`.

```text
$SURF_HOME/logs/server.log
$SURF_HOME/logs/desktop.log
$SURF_HOME/logs/devices/<device-id>.ndjson
```

Clients send structured records during their authenticated sessions and upload
bounded snapshots after connection and background changes.

```sh
surf logs
surf logs --follow
surf logs --source device --device DEVICE_ID
```

## Clipboard

```sh
surf clipboard status
surf clipboard sync on
surf clipboard get
surf clipboard set
surf clipboard set --device DEVICE_ID
surf clipboard sync off
```

With sync enabled, copied text moves between the host and connected iOS clients.
Windows uses its native clipboard API. macOS uses `pbcopy` and `pbpaste`.
Linux uses `wl-clipboard`, `xclip`, or `xsel`.

A headless host without a system clipboard can still sync connected Surf
clients and expose the in memory value through `surf clipboard get`.

With sync disabled, desktop Settings has a **Send once** field. The terminal
prompt for `surf clipboard set` hides typed input and preserves redirected
input exactly. Clipboard text is not accepted as a command argument, stored on
disk, or written to logs. A one time device value expires after two minutes if
unchanged.

## API

Network routes use the `/api/v1` prefix.

```text
/api/v1/health
/api/v1/server
/api/v1/pairing/*
/api/v1/auth/*
/api/v1/config
/api/v1/ws
/api/v1/client/logs
/api/v1/tab-icons/*
/api/v1/uploads
/api/v1/downloads/*
/api/v1/updates/client
/api/v1/admin/*
```

Administration routes accept only loopback requests with the current control
token. Other protected routes require a device session. WebSockets use a new
single use ticket bound to the device.

The health route is public and only reports reachability.

```sh
curl -k https://127.0.0.1:18080/api/v1/health
```

Paired clients and local Settings can read detailed capture, media, frame, drop,
and Widevine statistics.

## Browser and media

Surf uses a compatible installed Chrome, Edge, or Chromium. If none is found,
it uses a verified managed Chromium build. Extensions are loaded through the
DevTools protocol. Chromium tab capture supplies video and audio.

The stream uses Chromium's software AVC encoder. GPU page rendering remains
enabled. Surf requests variable rate encoding with `STREAM_BITRATE` and falls
back to constant quality using `STREAM_QUANTIZER`.

The default stream is the native client size at 60 FPS. Clients report renderer
health, throughput, queue depth, frame age, drops, and memory pressure every two
seconds.

`SURF_ADAPTIVE_VIDEO=1` enables four frame rate profiles at the same render
size.

| Profile | Frame rate |
| --- | ---: |
| Crisp | 60 FPS |
| Motion | 50 FPS |
| Balanced | 40 FPS |
| Recovery | 30 FPS |

The client reports the even sized page surface left by its controls. Width and
height must be between 64 and 1600. Rotation and fullscreen changes are
coalesced before Chromium and the encoder are resized.

Surf does not include Widevine. A selected host browser may provide it.

## Input

Desktop clients use normalized pointer and wheel input when the server
advertises `pointer-input`. Moves and wheel samples may coalesce. Button
changes, keyboard input, paste, and IME composition remain ordered. Older
servers receive touch input.

The iOS client sends complete active touch snapshots with stable contact IDs.
The backend rejects stale generations and sequence numbers, maps coordinates
through the current Chromium viewport, and dispatches Chromium touch events.
Navigation, tab changes, viewport changes, website mode changes, and disconnects
cancel active input.

**Mobile Websites** changes Chromium metrics and browser identity. It does not
change touch input into mouse input.

Focus events from Chromium control the native keyboard. Plain text uses
Chromium insertion. Marked text uses IME composition events.

Client and backend compatibility uses `COMPATIBILITY_VERSION`. Matching
generations connect. An older client can receive the embedded matching package.
A newer client requires a backend update.

## Updates

Desktop releases use a signed manifest and SHA 256 checked assets. The Windows
installer stops Surf before replacing it and starts the new version afterward.

The iOS package travels over the authenticated Surf connection. Its size, hash,
package identity, version, and architecture are checked before installation.

Updates replace programs in place. They do not require deleting `SURF_HOME` or
pairing again. See [Security](security.md#updates).
