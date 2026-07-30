# Backend

The backend is started with `surf serve`. It runs managed Chrome in
`--headless=new`, captures and encodes the active tab through Chromium's
tab-capture and WebCodecs APIs, and streams video, audio, and control messages
to the native client.

## Desktop app

The recommended local-computer package is Surf. It is a single
executable containing a tray/menu-bar supervisor and an isolated backend child
mode. It generates and persists a strong password and reports health. Its
loopback-only Settings window shows the detected LAN address and stream state, provides editable
password and port settings, and exposes collapsed local logs. The tray menu
opens Settings, restarts the backend, and quits Surf. The backend
remains a separate process, so the same server implementation is used by both
desktop and headless deployments.

Settings checks GitHub Releases for a newer official build and can install it
in place. Headless installations provide the same operation as `surf update`.
The release manifest is fetched over HTTPS and every downloaded archive is
checked against its declared size and SHA-256 before replacement. A previous
executable is retained as `surf.previous`.

Desktop packages are built natively for Linux x86-64 and ARM64, Windows x86-64, macOS
Intel, and macOS Apple Silicon. Surf prefers a compatible system Chrome, then
system Chromium, and otherwise downloads a verified ungoogled-chromium release
into `SURF_HOME`.

macOS packages require macOS 12 Monterey or newer.

The desktop app accepts `SURF_HOME` and `SURF_PASSWORD` overrides. Its
generated configuration is stored at
`SURF_HOME/desktop.json`, and combined supervisor/backend output is written to
`SURF_HOME/desktop.log`.

The `serve` mode described below is intended for services, VPSes, and machines
without a desktop session.

## Dependencies

Surf resolves Chromium automatically. Audio uses a built-in Manifest V3
extension and Chromium's tab capture API on every supported host. Capturing a
tab suppresses its local playback, so audio is heard by the Surf client without
also playing on the host. Surf converts it to signed 16-bit, 16 kHz mono PCM
inside Chromium and carries it over a random, loopback-only WebSocket. No
virtual audio cable or external media process is required.

## Quick Start

Download the `surf` archive matching Linux x86-64 or ARM64, Windows x86-64, macOS
Intel, or macOS Apple Silicon from GitHub Releases. For Linux:

```sh
tar xf surf-*-linux-*.tar.gz
cd surf-*-linux-*
./surf doctor
SURF_PASSWORD='change-me' ./surf serve
```

Use a strong password before exposing Surf beyond local testing. Press `Ctrl-C`
to stop the foreground backend.

The default LAN URL is:

```text
http://YOUR_COMPUTER_IP:18080
```

## Build From Source

From the repo root:

```sh
make surf-binary
SURF_PASSWORD='change-me' ./backend/surf serve
```

Press `Ctrl-C` to stop the foreground backend.

## Useful Commands

```sh
make surf-binary         # build backend/surf
make surf-dist           # build the current host package
```

Run the binary directly:

```sh
make surf-binary
SURF_PASSWORD='change-me' ./backend/surf serve
```

## Runtime Behavior

Surf uses `CHROME` when explicitly set; that browser must permit unpacked
extensions. Otherwise Surf prefers an extension-capable installed Chromium and
then its managed ungoogled-chromium runtime because media capture uses the
built-in extension. Managed releases are downloaded from the official
ungoogled-chromium GitHub organization, checked against GitHub's declared size
and SHA-256 digest, installed atomically, and checked daily. One previous
version is retained for rollback. Chrome for Testing is not used.

The selected browser always runs with `--headless=new`. Surf's built-in
Manifest V3 extension captures the active tab. An offscreen document feeds raw
video frames to Chromium's H.264 WebCodecs encoder and sends complete Annex-B
access units over a random, loopback-only WebSocket. It samples tabCapture at
up to 60 Hz into a one-frame latest-only handoff, then emits H.264 on a stable
30 Hz clock. Short compositor scheduling gaps repeat the most recent image
instead of becoming transport and presentation gaps; repeats are marked
internally and never counted as fresh interaction responses. The same
document converts captured audio to 16-bit, 16 kHz mono PCM. Annex-B H.264 is
the sole client-facing visual format. Clients subscribe automatically after
connecting; encoder failures are reported as explicit `video-config` states
and require `video-retry`.

There is no desktop capture, headful, headless-shell, or screenshot-polling
mode. `CHROME=/path/to/browser` remains an explicit development/recovery
override.

Backend ownership is split by latency domain. `Controller` consumes concrete
typed commands and owns tab/input state. The extension bridge owns capture and
encoding. `VideoPipeline` tracks encoder generations and fans complete access
units out to clients. The WebSocket transport independently schedules ordered
control, drop-oldest audio, and a four-AU GOP-aware video queue;
`AudioPipeline` owns bounded audio fan-out. Video overflow requests an
immediate cooldown-protected IDR instead of accumulating delay.

The Go module mirrors those boundaries:

- `cmd/surf` is the unified CLI and desktop entrypoint.
- `internal/app` is the composition root and process lifecycle.
- `internal/browser` owns tabs, input, navigation, and browser features.
- `internal/media` owns Chromium tab capture, WebCodecs video, and audio/video fan-out.
- `internal/transport` owns each client WebSocket and its lane queues.
- `internal/web` owns login, native configuration, health, and feature routes.
- `internal/chromium` owns browser provisioning and profile preparation;
  `internal/cdp` owns the browser connection; `internal/process` contains the
  small Unix/Windows process-lifetime differences.
- `internal/protocol` is the typed client wire contract.

Protocol `20260729-4` uses a 96-byte extensible binary header carrying AU and
source sequences, coded size, interaction ID, backend timing stamps, encoder
generation, CDP scroll metadata, and the active adaptive profile. The
socket-write timestamp is stamped by the WebSocket writer immediately before
the write. Input receive and CDP-dispatch timestamps make the input-to-source
part of the path distinguishable from capture, encode, transport, decode, and
display.

Control messages are decoded into concrete command types. In addition to
ordered navigation and input, pan gestures use ordered `scroll` begin/move/end
commands carrying precise pixel deltas at UIKit's gesture callback cadence,
followed by a short inertial tail after release. Chromium applies every delta
to the page while the video pipeline independently selects the latest frame at
its configured rate; the client never shifts stale video locally. The protocol also exposes stateful page-media
controls (`media-query`, playback, mute, and volume) and a client-selected
mobile browsing mode. Mobile mode applies a coherent Android Chrome user
agent, client hints, touch viewport behavior, and reloads the active page so
both server-rendered and responsive sites can select their mobile UI. A user-created tab stays on
`about:blank#surf-new` until the native New Tab page chooses a destination;
the backend never inserts a delayed homepage navigation.

The client reports decode and presentation health every five seconds. Surf
keeps the client-requested coded size fixed by default, avoiding visible
resolution changes and encoder restarts. Experimental adaptive profiles can be
enabled explicitly; they require 30 healthy seconds before stepping back up.

On every platform, Surf invokes its built-in extension for Chromium's active
tab when the client subscribes. An offscreen extension document owns the media
stream and an AudioWorklet converts it to signed 16-bit, 16 kHz mono PCM before
it enters the bounded audio fan-out. Switching tabs moves the capture to the
newly active tab; stopping the client subscription releases the tab capture.
Chromium remains muted for its entire process lifetime, including while no
client is connected. This uses Chromium's own cross-platform audio-output
mute and does not require a host audio service or virtual audio device.

Data is stored under `~/.surf/` by default:

- `~/.surf/profile`: Chromium profile.
- `~/.surf/downloads`: browser downloads.
- `~/.surf/uploads`: temporary upload files.
- `~/.surf/runtime/ublock-origin-lite`: Surf's verified, managed content
  blocker. It is loaded into the browser profile automatically.
- `~/.surf/runtime/tab-capture-extension`: Surf's generated Chromium media bridge.

## Configuration

Common overrides:

- `SURF_PASSWORD`: required login password.
- `PORT`: listen port; defaults to `18080`.
- `BIND_ADDR`: listen address; defaults to `0.0.0.0`.
- `SURF_HOME`: data root; defaults to `~/.surf`.
- `CHROME`: explicit extension-capable browser override. Setting this disables
  managed browser selection.
- `SURF_BROWSER_DOWNLOAD=0`: prohibit downloading managed ungoogled-chromium.
- `SURF_CONTENT_BLOCKER=0`: disable the managed uBlock Origin Lite extension.
  It is enabled by default. The built-in tab-capture extension remains enabled.
- `SURF_CONTENT_BLOCKER_DOWNLOAD=0`: prohibit downloading a missing pinned
  uBlock Origin Lite release.
- `SURF_CHROME_GPU=0`: disable Chrome GPU acceleration. GPU acceleration is
  enabled by default; Chrome may still select a software renderer when the
  host has no usable graphics device.
- `SURF_ADAPTIVE_VIDEO=1`: opt in to automatic coded-size
  reduction when two consecutive motion windows miss their performance
  targets. Disabled by default to keep quality and resolution stable.
- `STREAM_SCALE`: maximum coded width and height, preserving aspect ratio;
  unset by default so the encoded frame exactly follows the client viewport.
  Set `768x1024` to cap encoder and decoder work on an original iPad.
- `STREAM_BITRATE`: target H.264 bitrate in kbit/s; defaults to `12000`.
- `SURF_ADVERTISE=0`: disable LAN discovery advertisement.
- `SURF_UPDATE_MANIFEST`: override the GitHub update manifest URL for release
  testing.

If you run the backend as root on a VPS, Chromium usually requires
`CHROME_NO_SANDBOX=1`. Surf enables this by default for root, but running as a
normal user is preferred.

## Security

Do not expose a weak-password backend to the public internet. For VPS usage, put
it behind HTTPS and use a strong password.

Official GitHub Releases are Surf's update source of truth. SHA-256 protects
against damaged or mismatched downloads; it is not a separate publisher
signature. Release builds embed the native package built from the same commit.
An authenticated native client with an older protocol can download that package
from `/updates/v1/client`; media WebSockets are never used for update payloads.

Video output is fixed at 30 FPS. Surf asks tabCapture for up to 60 source
frames per second, retains only the newest source image, and samples it on the
30 Hz encoder clock. Latest-only capture/presentation slots and bounded
encode/GOP transport queues discard stale work instead of accumulating
latency. Backend diagnostics report paced AU gaps separately from fresh-source
gaps and duplicate AUs, so a stable output clock cannot hide a Chromium
compositor stall.

The defaults target the original iPad at its exact viewport and 12 Mbps:

```sh
SURF_PASSWORD='choose-a-password' ./surf serve
```

Triple-tap the video on the iPad to open the diagnostics overlay. During
continuous motion, `AU RATE` should approach 30 and `VT CALLBACK` should
remain below the 33.3 ms frame budget. Lower `STREAM_SCALE` only if decode
time exceeds that budget.
