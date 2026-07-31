# Backend

Surf runs Chromium on a Windows, macOS, or Linux host and exposes one encrypted
remote-browser port. The desktop build supervises the backend and provides
local Settings, pairing, device revocation, logs, and updates. `surf daemon` is
the equivalent foreground process for a service manager.

## Start and pair

Desktop users open **Paired Devices** and choose **Pair new client**.
Headless users start one persistent daemon and connect to it from another
terminal:

```sh
./surf daemon
./surf status
./surf pair
```

Pairing is closed by default. The desktop button or `surf pair` creates one
single-use invitation with a six-digit manual code and a QR code. The QR is
self-contained: it carries the reachable address, expected server identity,
and a random 128-bit one-time token. A manual client enters the address and
six-digit code separately.

The invitation has no timer; it remains open until used, cancelled, the daemon
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

For a VPS, expose the configured Surf TCP port and tell pairing codes which
reachable address to use:

```sh
SURF_PUBLIC_ADDRESS=surf.example.net:18080 ./surf daemon
```

This does not provide NAT traversal. LAN firewalls and VPS security groups must
allow the selected port. Protect `SURF_HOME`: copying it copies the server
identity and paired-device registry.

## Configuration

Surf reads configuration from environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `SURF_HOME` | `~/.surf` | Identity, devices, browser profile, downloads, logs |
| `SURF_SERVER_NAME` | `Surf` | Friendly name shown while pairing and in Bonjour |
| `SURF_PUBLIC_ADDRESS` | empty | Reachable host/IP and optional port for pairing QR codes |
| `BIND_ADDR` | `0.0.0.0` | Listener address |
| `PORT` | `18080` | TLS/API port |
| `CHROME` | auto | Chromium or Edge executable |
| `PROFILE` | `$SURF_HOME/profile` | Chromium profile |
| `START_URL` | Google | Initial page |
| `DOWNLOADS` | `$SURF_HOME/downloads` | Browser downloads |
| `UPLOADS` | `$SURF_HOME/uploads` | Temporary client uploads |
| `VW`, `VH` | `768`, `934` | Initial viewport before the client reports its exact size |
| `STREAM_BITRATE` | `24000` | H.264 fallback target in kbit/s |
| `STREAM_QUANTIZER` | `12` | H.264 constant-quality QP (0–51; lower is sharper) |
| `STREAM_SCALE` | empty | Optional maximum stream size |
| `SURF_CHROME_GPU` | `1` | Enable Chromium GPU support |
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

`daemon.json` is a permission-restricted, per-run control descriptor. CLI
commands use it to find and authenticate the daemon's loopback TLS control
listener; they never start a second browser or create a server identity.

The browser profile, downloads, uploads, managed browser, updates, and desktop
configuration also live below `SURF_HOME`. Back up the entire directory if you
want to preserve the server identity and existing pairings.

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

Surf prefers a compatible installed Chromium or Microsoft Edge, otherwise it
uses its verified managed Chromium build. Active-tab `tabCapture` supplies both
video and audio; no FFmpeg, PulseAudio, desktop capture, or virtual audio device
is required.

Surf does not distribute Widevine. A working CDM supplied by the selected host
browser may be used, subject to each service's DRM and output-protection rules.

## Updates

Desktop releases use the signed release manifest and SHA-256-verified assets.
The native `.deb` update travels over its already authenticated, pinned Surf
connection and is verified again before the privileged installer applies it.
