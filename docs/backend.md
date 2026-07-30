# Backend

The backend is started with `surf serve`. It runs managed Chrome in
`--headless=new`, transcodes CDP screencast frames to H.264 with bundled
FFmpeg, and streams video, audio, and control messages to the native client.
Audio capture uses Chromium's tab-capture API on Linux, Windows, and macOS.

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
Intel, and macOS Apple Silicon. They include FFmpeg. Surf prefers a compatible
system Chrome, then system Chromium, and otherwise downloads a verified
ungoogled-chromium release into `SURF_HOME`.

The desktop app accepts `SURF_HOME` and `SURF_PASSWORD` overrides. Its
generated configuration is stored at
`SURF_HOME/desktop.json`, and combined supervisor/backend output is written to
`SURF_HOME/desktop.log`.

The `serve` mode described below is intended for services, VPSes, and machines
without a desktop session.

## Dependencies

Surf resolves Chromium automatically. Linux's fallback audio path additionally
needs:

- `pactl` with PulseAudio or PipeWire Pulse compatibility (Linux fallback).
- `pulseaudio` when no existing PulseAudio-compatible server is available
  (Linux fallback).

Example package installs:

```sh
# Debian/Ubuntu package names vary by release.
sudo apt install pulseaudio-utils pulseaudio

# Fedora package names vary by release and enabled repositories.
sudo dnf install pulseaudio-utils pulseaudio

# Arch.
sudo pacman -S pulseaudio
```

Audio uses a built-in Manifest V3 extension and Chromium's tab capture API on
every supported host. Capturing a tab suppresses its local playback, so audio
is heard by the Surf client without also playing on the host. Surf converts it
to signed 16-bit, 16 kHz mono PCM inside Chromium and carries it over a random,
loopback-only WebSocket. No virtual audio cable or FFmpeg input device is
required for the primary path.

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
then its managed ungoogled-chromium runtime because tab audio always uses the
built-in extension. Managed releases are downloaded from the official
ungoogled-chromium GitHub organization, checked against GitHub's declared size
and SHA-256 digest, installed atomically, and checked daily. One previous
version is retained for rollback. Chrome for Testing is not used.

Release packages embed the pinned FFmpeg executable and install it into
`SURF_HOME/runtime` on demand. Development builds retain the verified download
fallback. The selected browser always runs with
`--headless=new`; the capture source is event-driven `Page.startScreencast`,
and its JPEG is internal only. FFmpeg sends H.264/RTP over localhost so RTP
marker bits delimit access units without waiting for a following frame.
Annex-B H.264 is the sole client-facing visual format.
Clients subscribe automatically after connecting; encoder failures are
reported as explicit `video-config` states and require `video-retry`.

The Linux runtime uses a pinned BtbN GPL build because Surf needs both the
Pulse input device and libx264. Windows and macOS use pinned `ffmpeg-static`
executables. Primary audio capture bypasses FFmpeg input and receives 16 kHz
mono PCM from the built-in Chromium tab-capture extension.

There is no desktop capture, headful, headless-shell, or screenshot-polling
mode. `CHROME=/path/to/browser` and `FFMPEG=/path/to/ffmpeg` remain explicit
development/recovery overrides.

Backend ownership is split by latency domain. `Controller` consumes concrete
typed commands and owns tab/input state. `ScreencastSource` exclusively owns
CDP screencast start/stop/ACK and rejects stale tab/viewport completions by
generation. Immutable source frames bypass the controller loop into
`VideoPipeline`'s latest-frame mailbox. `ClientTransport` independently
schedules ordered control, drop-oldest audio, and a four-AU GOP-aware video
queue; `AudioPipeline` owns capture and bounded fan-out. Video overflow
requests an immediate cooldown-protected IDR instead of accumulating delay.

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
keeps the configured capture quality and coded size fixed by default, avoiding
visible quality changes and encoder restarts. Experimental adaptive profiles
can be enabled explicitly; they require 30 healthy seconds before stepping
back up. Static pages are excluded from FPS and presentation-gap decisions
because CDP intentionally emits only sparse bootstrap frames when nothing
changes.

For audio on Linux, Surf first checks whether `pactl info` can reach an
existing PulseAudio-compatible server. On common PipeWire desktops this
succeeds, so Surf creates a `surf_output` null sink there and unloads it on
shutdown. If no server is available, Surf starts its own PulseAudio process.

On every platform, Surf invokes its built-in extension for Chromium's active
tab when the client subscribes. An offscreen extension document owns the media
stream and an AudioWorklet converts it to signed 16-bit, 16 kHz mono PCM before
it enters the bounded audio fan-out. Switching tabs moves the capture to the
newly active tab; stopping the client subscription releases the tab capture.
Linux retains its PulseAudio monitor source as a fallback if tab capture cannot
start.

Data is stored under `~/.surf/` by default:

- `~/.surf/profile`: Chromium profile.
- `~/.surf/downloads`: browser downloads.
- `~/.surf/uploads`: temporary upload files.
- `~/.surf/runtime/ublock-origin-lite`: Surf's verified, managed content
  blocker. It is loaded into the browser profile automatically.
- `~/.surf/runtime/tab-audio-extension`: Surf's generated Chromium audio bridge.

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
  It is enabled by default. The built-in tab-audio extension remains enabled.
- `SURF_CONTENT_BLOCKER_DOWNLOAD=0`: prohibit downloading a missing pinned
  uBlock Origin Lite release.
- `FFMPEG`: explicit encoder override; otherwise Surf uses its bundled FFmpeg.
- `SURF_FFMPEG_DOWNLOAD=0`: prohibit downloading a missing managed FFmpeg.
- `SURF_CHROME_GPU=0`: disable Chrome GPU acceleration. GPU acceleration is
  enabled by default; Chrome may still select a software renderer when the
  host has no usable graphics device.
- `SURF_ADAPTIVE_VIDEO=1`: opt in to automatic capture-quality and coded-size
  reduction when two consecutive motion windows miss their performance
  targets. Disabled by default to keep quality and resolution stable.
- `PULSEAUDIO`, `PACTL`: Linux audio tool paths.
- `SOURCE_JPEG_QUALITY`: quality of the internal CDP-to-FFmpeg capture;
  defaults to `100` and does not select a client wire format.
- `STREAM_FPS`: capture and H.264 pacing target; defaults to `30`. Use `60`
  only when the encoder host and client can sustain it.
- `STREAM_SCALE`: maximum coded width and height, preserving aspect ratio;
  defaults to `1024x1024`. For the original iPad, `768x1024` reduces encoder
  and decoder work without scaling portrait content above its native width.
- `STREAM_ENCODER`: H.264 encoder; defaults to portable `libx264`. Use
  `h264_nvenc` on a host with a supported NVIDIA GPU and driver.
- `STREAM_BITRATE`: target H.264 bitrate in kbit/s; defaults to `6000`.
- `STREAM_MAXRATE`: H.264 VBV peak bitrate in kbit/s; defaults to `8000`.
- `STREAM_BUFSIZE`: H.264 VBV buffer size in kbit; defaults to `1800`.
- `SURF_MANAGE_PULSE=1`: force Surf to start a private PulseAudio process
  (Linux).
- `SURF_ENSURE_PULSE_SINK=0`: do not create `PULSE_SINK` automatically when
  using an existing PulseAudio/PipeWire server (Linux).
- `PULSE_SINK`: sink Chromium should use; defaults to `surf_output` (Linux).
- `AUDIO_SOURCE`: source FFmpeg captures; defaults to `surf_output.monitor`
  (Linux).
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

For a 60 FPS original-iPad experiment, start with:

```sh
SURF_PASSWORD='choose-a-password' \
STREAM_FPS=60 \
STREAM_SCALE=768x1024 \
SOURCE_JPEG_QUALITY=75 \
STREAM_BITRATE=2000 \
STREAM_MAXRATE=3000 \
./surf serve
```

Triple-tap the video on the iPad to open the diagnostics overlay. During
continuous motion, `AU RATE` should approach 60 and `VT CALLBACK` should remain
below the 16.7 ms frame budget. Lower `STREAM_SCALE` if decode time exceeds that
budget.
