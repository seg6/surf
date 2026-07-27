# Backend

The backend is started with `surf serve`. It runs managed Chrome in
`--headless=new`, transcodes CDP screencast frames to H.264 with managed
FFmpeg, and streams video, audio, and control messages to the native client.
Windows and macOS are also supported for the video/browsing path; audio
capture is Linux-only for now.

## Desktop app

The recommended local-computer package is Surf. It is a single
executable containing a tray/menu-bar supervisor and an isolated backend child
mode. It generates and persists a strong password and reports health. Its
loopback-only Settings window shows the detected LAN address and stream state, provides editable
password and port settings, and exposes collapsed local logs. The tray menu
opens Settings, restarts the backend, and quits Surf. The backend
remains a separate process, so the same server implementation is used by both
desktop and headless deployments.

Desktop packages are built natively for Linux x86-64, Windows x86-64, macOS
Intel, and macOS Apple Silicon. Chrome and FFmpeg are downloaded into
`SURF_HOME` on first launch.

The desktop app accepts `SURF_HOME` and `SURF_PASSWORD` overrides. Its
generated configuration is stored at
`SURF_HOME/desktop.json`, and combined supervisor/backend output is written to
`SURF_HOME/desktop.log`.

The `serve` mode described below is intended for services, VPSes, and machines
without a desktop session.

## Dependencies

Surf downloads Chrome and FFmpeg automatically. Linux audio additionally needs:

- `pactl` with PulseAudio or PipeWire Pulse compatibility (Linux, for audio).
- `pulseaudio` when no existing PulseAudio-compatible server is available
  (Linux, for audio).

Example package installs:

```sh
# Debian/Ubuntu package names vary by release.
sudo apt install pulseaudio-utils pulseaudio

# Fedora package names vary by release and enabled repositories.
sudo dnf install pulseaudio-utils pulseaudio

# Arch.
sudo pacman -S pulseaudio
```

Audio capture is not yet implemented on Windows and macOS.

## Quick Start

Download the `surf` archive matching Linux x86-64, Windows x86-64, macOS
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

On first run Surf downloads pinned Chrome for Testing and a pinned FFmpeg
executable into `SURF_HOME/runtime`. Linux x86-64, Windows x86-64,
macOS x86-64, and macOS arm64 are selected automatically. The pinned runtimes
keep browser and encoder behavior reproducible. Chrome always runs with
`--headless=new`; the capture source is event-driven `Page.startScreencast`,
and its JPEG is internal only. FFmpeg sends H.264/RTP over localhost so RTP
marker bits delimit access units without waiting for a following frame.
Annex-B H.264 is the sole client-facing visual format.
Clients subscribe automatically after connecting; encoder failures are
reported as explicit `video-config` states and require `video-retry`.

The Linux runtime uses a pinned BtbN GPL build because Surf needs both the
Pulse input device and libx264. Windows and macOS use pinned `ffmpeg-static`
executables; audio capture is not implemented on those platforms.

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

Protocol `20260727-3` uses a 64-byte extensible binary header carrying AU and
source sequences, coded size, interaction ID, backend timing stamps, and
encoder generation. The socket-write timestamp is stamped by the WebSocket
writer immediately before the write.

For audio (Linux only), Surf first checks whether `pactl info` can reach an
existing PulseAudio-compatible server. On common PipeWire desktops this
succeeds, so Surf creates a `surf_output` null sink there and unloads it on
shutdown. If no server is available, Surf starts its own PulseAudio process.

Data is stored under `~/.surf/` by default:

- `~/.surf/profile`: Chromium profile.
- `~/.surf/downloads`: browser downloads.
- `~/.surf/uploads`: temporary upload files.

## Configuration

Common overrides:

- `SURF_PASSWORD`: required login password.
- `PORT`: listen port; defaults to `18080`.
- `BIND_ADDR`: listen address; defaults to `0.0.0.0`.
- `SURF_HOME`: data root; defaults to `~/.surf`.
- `CHROME`: explicit browser override. Setting this disables managed browser
  selection.
- `SURF_BROWSER_DOWNLOAD=0`: prohibit downloading a missing managed browser.
- `FFMPEG`: explicit encoder override; otherwise Surf manages FFmpeg.
- `SURF_FFMPEG_DOWNLOAD=0`: prohibit downloading a missing managed FFmpeg.
- `PULSEAUDIO`, `PACTL`: Linux audio tool paths.
- `SOURCE_JPEG_QUALITY`: quality of the internal CDP-to-FFmpeg capture;
  defaults to `100` and does not select a client wire format.
- `SURF_MANAGE_PULSE=1`: force Surf to start a private PulseAudio process
  (Linux).
- `SURF_ENSURE_PULSE_SINK=0`: do not create `PULSE_SINK` automatically when
  using an existing PulseAudio/PipeWire server (Linux).
- `PULSE_SINK`: sink Chromium should use; defaults to `surf_output` (Linux).
- `AUDIO_SOURCE`: source FFmpeg captures; defaults to `surf_output.monitor`
  (Linux).
- `SURF_ADVERTISE=0`: disable LAN discovery advertisement.

If you run the backend as root on a VPS, Chromium usually requires
`CHROME_NO_SANDBOX=1`. Surf enables this by default for root, but running as a
normal user is preferred.

## Security

Do not expose a weak-password backend to the public internet. For VPS usage, put
it behind HTTPS and use a strong password.
