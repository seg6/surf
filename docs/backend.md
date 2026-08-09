# Backend

Surf runs Chromium on a Windows, macOS, or Linux host and exposes one encrypted
remote-browser port. The desktop build supervises the backend and provides
local Settings, pairing, device revocation, logs, clipboard delivery, and
updates. `surf serve` is the equivalent foreground process for a service
manager. Add `--pair` to open one pairing invitation at startup.

## Start and pair

Desktop users open **Paired Devices** and choose **Pair device**.
Headless users start one persistent server and connect to it from another
terminal:

```sh
./surf serve
./surf status
./surf pair
```

Pairing is closed by default. The desktop button or `surf pair` creates one
single-use invitation with a six-digit manual code and a QR code. The QR is
self-contained: it carries the reachable address, expected server identity,
and a random 128-bit one-time token. A manual client enters the address and
six-digit code separately.

The invitation has no timer; it remains open until used, cancelled, the server
restarts, or five incorrect manual codes close it. Only one matching client key
can consume it. QR pairing already pins the identity carried by the code.
Manual pairing compares a six-word value independently derived from the TLS
certificate and client key; the six-digit code authorizes the request but does
not by itself authenticate a self-signed endpoint against an active relay.

Manage paired clients without restarting the browser:

```sh
./surf devices list
./surf devices revoke DEVICE_ID
```

Revocation closes that device's active WebSockets immediately and invalidates
its sessions, challenges, and tickets.

## Direct TLS

Surf creates a persistent RSA-2048/SHA-256 certificate on first launch and
serves TLS 1.2+ directly. Clients pin the SHA-256 leaf fingerprint, so a public
CA, domain, reverse proxy, and installed iOS certificate are not required.
TLS resumption is disabled; every new transport presents the pinned identity.
The full trust model and first-pairing MITM boundary are documented in
[Security](security.md).

For a VPS, expose the configured Surf TCP port and tell pairing codes which
reachable address to use:

```sh
SURF_PUBLIC_ADDRESS=surf.example.net:18080 ./surf serve
```

This does not provide NAT traversal. LAN firewalls and VPS security groups must
allow the selected port. Protect `SURF_HOME`: copying it copies the server
identity and paired-device registry.

Cloudflare Tunnel can provide NAT traversal without changing Surf's trust
anchor. Route a hostname to the Surf HTTPS listener, set that hostname in
`SURF_TUNNEL_HOST`, and use it as `SURF_PUBLIC_ADDRESS`. The public WebSocket is
only an opaque carrier; the client establishes a second, certificate-pinned
Surf TLS connection through it.

## Configuration

Surf reads configuration from environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `SURF_HOME` | `~/.surf` | Identity, devices, browser profile, downloads, logs |
| `SURF_SERVER_NAME` | `Surf` | Friendly name shown while pairing and in Bonjour |
| `SURF_PUBLIC_ADDRESS` | empty | Reachable host/IP and optional port for pairing QR codes |
| `SURF_TUNNEL_HOST` | empty | Exact public hostname that enables the opaque WebSocket roaming transport |
| `SURF_ADVERTISE_IP` | auto | Explicit LAN address for Bonjour; useful in containers and PRoot |
| `BIND_ADDR` | `0.0.0.0` | Listener address |
| `PORT` | `18080` | TLS/API port |
| `CHROME` | auto | Chromium or Edge executable |
| `PROFILE` | `$SURF_HOME/profile` | Chromium profile |
| `START_URL` | Google | Initial page |
| `DOWNLOADS` | `$SURF_HOME/downloads` | Browser downloads |
| `UPLOADS` | `$SURF_HOME/uploads` | Temporary client uploads |
| `VW`, `VH` | `768`, `934` | Initial viewport before the client reports its exact size |
| `STREAM_BITRATE` | `48000` | H.264 variable-rate target in kbit/s |
| `STREAM_QUANTIZER` | `12` | H.264 constant-quality QP (0–51; lower is sharper) |
| `STREAM_SCALE` | empty | Optional maximum stream size |
| `SURF_CONTENT_BLOCKER` | `1` | Manage uBlock Origin Lite |
| `SURF_ADAPTIVE_VIDEO` | `0` | Experimental adaptive stream profile |
| `CHROME_NO_SANDBOX` | automatic for root | Disable Chromium sandbox where required |

There is no password variable and no plaintext mode.

## Persistent files

The security-sensitive files are permission-restricted and replaced
atomically:

```text
$SURF_HOME/identity/server.crt
$SURF_HOME/identity/server.key
$SURF_HOME/identity/session.key
$SURF_HOME/devices.json
$SURF_HOME/daemon.json
```

`daemon.json` is a permission-restricted, per-run server control descriptor.
CLI commands use it to find and authenticate the server's loopback TLS control
listener; they never start a second browser or create a server identity.

Desktop Surf owns its managed server through a private parent pipe. Normal
Quit performs a graceful shutdown; if the tray is force-closed, the pipe closes
and the server exits with its Chromium process tree. `desktop.lock` and
`server.lock` are kernel-owned locks: their empty files may remain on disk, but
the locks themselves are released when their owner exits.

The browser profile, downloads, uploads, managed browser, updates, and desktop
configuration also live below `SURF_HOME`. Back up the entire directory if you
want to preserve the server identity and existing pairings.

## Logs and clipboard

Surf keeps bounded, rotating host logs under `SURF_HOME`:

```text
$SURF_HOME/logs/server.log
$SURF_HOME/logs/desktop.log
$SURF_HOME/logs/devices/<device-id>.ndjson
```

The native client sends each structured record over its authenticated session
as soon as it is written. It also uploads bounded snapshots at connection and
background boundaries so reconnects can repair any missed interval. Desktop
Settings follows one selected source live; terminal access is available with:

```sh
surf logs
surf logs --follow
surf logs --source device --device DEVICE_ID
```

Clipboard synchronization is controlled by the host owner:

```sh
surf clipboard status
surf clipboard sync on
surf clipboard get
surf clipboard set
surf clipboard set --device DEVICE_ID
surf clipboard sync off
```

With two-way sync on, copying text on the host or any connected iOS client
updates the others. Surf uses the native Windows clipboard API, `pbcopy` and
`pbpaste` on macOS, and `wl-clipboard`, `xclip`, or `xsel` on Linux. A headless
host without one of those providers can still synchronize connected Surf
clients and expose the current memory-only value through `surf clipboard get`.

With sync off, desktop Settings presents a **Send once** text box. The
equivalent `surf clipboard set` terminal prompt hides input; redirected
standard input is preserved byte-for-byte, including whitespace and newlines.
Surf never accepts clipboard text as a command-line argument. Only the enabled
preference is stored under `SURF_HOME`; clipboard text is not persisted or
included in logs. One-off device values expire after two minutes if unchanged.

Replaceable state recovers conservatively. Invalid desktop settings, device
registries, and runtime descriptors are moved beside the original with an
`.invalid-<timestamp>` suffix. After two browser startup failures within ten
minutes, Surf preserves the Chromium profile as
`profile.startup-failed-<timestamp>`, restores Surf bookmarks and history into
a clean profile, and retries. This recovery never replaces the pinned
`identity/server.crt` or `identity/server.key`; an invalid server identity stays
failed closed and requires an explicit owner decision.

## Versioned API

Surf 0.10 intentionally has no old-route aliases. Network interfaces live
under one root:

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

`/api/v1/admin/*` is loopback-only and requires the per-run control token.
Other protected routes require a signed, short-lived device session;
WebSockets use a fresh device-bound one-time ticket. The signed challenge is
bound to API v1, the server ID, device ID, challenge ID, and nonce.

The unauthenticated health endpoint is useful for basic reachability:

```sh
curl -k https://127.0.0.1:18080/api/v1/health
```

Detailed runtime statistics are available to the local desktop and paired
clients. They include capture, video/audio subscribers, frame/drop counters,
and Widevine capability state.

## Browser and media

Surf prefers a compatible installed Google Chrome, Microsoft Edge, or Chromium,
otherwise it uses its verified managed Chromium build. Capture and
content-blocking extensions are loaded through CDP so branded browsers do not
depend on the ignored `--load-extension` switch. Active-tab `tabCapture`
supplies both video and audio; no FFmpeg, PulseAudio, desktop capture, or
virtual audio device is required.

Surf strictly selects Chromium's software AVC encoder on every host. This does
not disable GPU rendering or compositing inside Chromium; it only keeps the
transmitted H.264 stream independent of platform encoders whose reference-frame
and buffering choices may exceed older clients' real-time decode budget. Surf
first requests constant-quality encoding with `STREAM_QUANTIZER`, then uses the
`STREAM_BITRATE` variable-rate configuration when the software encoder does not
support quantizer mode.

The native client reports the exact even-sized stream surface left by its
current chrome; the backend does not choose from hard-coded device profiles.
The 64-1600 dimension bounds are resource guards and cover the supported
iOS 6-14 catalog, whose largest logical screen dimension is 1366 points.
Rapid intermediate sizes are coalesced before Chromium and WebCodecs are
reconfigured, so rotation, Surf fullscreen, and page Fullscreen API transitions
retain the authenticated socket and media subscription. Page fullscreen state
is synchronized to native fullscreen in both directions.

### Physical input and website mode

The iOS `UIEvent` stream is the touch source of truth in both website modes.
Each physical contact gets one stable client ID for its lifetime. Start/end/
cancel edges are reliable ordered messages; move messages contain the complete
active-contact snapshot and may replace an older queued move. Coordinates and
contact radii are normalized against the exact encoder generation currently
presented on the device.

The backend owns the corresponding Chromium touch state on one serialized
worker. It rejects stale generations, clients, contact IDs, and sequence
numbers, and sends `touchCancel` across navigation, tab, viewport, website-mode,
and disconnect boundaries. Before dispatch it maps normalized surface points
through `Page.getLayoutMetrics().cssVisualViewport`, then sends real
`Input.dispatchTouchEvent` events. This distinction matters on mobile pages
whose CSS viewport is wider than the streamed surface. Client contact IDs are
remapped to dense Chromium-local IDs `0` through `4` for the active gesture;
this preserves stable multi-touch identity without allowing lifetime IDs to
degrade Chromium's velocity tracking after extended use.

Touch dispatch is ordered but nonblocking, so an expensive website event
handler cannot stall later move or release delivery. Chromium events are
timestamped when the backend dispatches them while the client clock validates
input ordering. On lift, the backend briefly separates the last real UIKit
move from the empty all-contact release; it does not invent another motion
sample from `touchesEnded`. Chromium can therefore commit the gesture's final
velocity and continue compositor-driven fling after the finger leaves the
screen.

**Mobile Websites** changes Chromium's mobile metrics and browser identity; it
does not change an iPad into a mouse. Physical input remains touch in both
modes, so Chromium owns tap activation, compositor scrolling and fling,
multi-touch pinch zoom, Pointer Events, and Touch Events. A page may still
restrict zoom through its own viewport policy, as it can in an ordinary touch
browser.

Editable focus is event-driven. A runtime binding listens to `focusin` and
`focusout`, follows the active element through open shadow roots, and tells the
client when to show its native keyboard. Plain insertions use Chromium text
insertion; marked iOS text uses IME composition update/commit/cancel messages.
The protocol is gated by the exact value in `PROTOCOL_VERSION`; obsolete input
commands are not accepted.

Surf does not distribute Widevine. A working CDM supplied by the selected host
browser may be used, subject to each service's DRM and output-protection rules.

## Updates

Desktop releases use the signed release manifest and SHA-256-verified assets.
On Windows, the installer closes the installed Surf process tree before
replacing the executable and starts the new version after a silent update.
The native `.deb` update travels over its already authenticated, pinned Surf
connection and is verified again before the privileged installer applies it.
See [Security](security.md#updates) for what that protects and which host trust
boundary remains.
