# Backend

The backend is named `surf-backend`. It runs Chromium headless, transcodes
its own CDP screencast frames to H.264 with FFmpeg, and streams video, audio,
and control messages to the native client. Windows and macOS are also
supported for the video/browsing path; audio capture is Linux-only for now.

## Dependencies

Install the host runtime tools before starting the backend:

- Chromium (`chromium`, or set `CHROME`).
- FFmpeg (`ffmpeg`).
- `pactl` with PulseAudio or PipeWire Pulse compatibility (Linux, for audio).
- `pulseaudio` when no existing PulseAudio-compatible server is available
  (Linux, for audio).

Example package installs:

```sh
# Debian/Ubuntu package names vary by release.
sudo apt install chromium ffmpeg pulseaudio-utils pulseaudio

# Fedora package names vary by release and enabled repositories.
sudo dnf install chromium ffmpeg pulseaudio-utils pulseaudio

# Arch.
sudo pacman -S chromium ffmpeg pulseaudio
```

On Windows, install Chromium (or Google Chrome) and FFmpeg; if neither
`chromium` nor `CHROME` resolves to a real binary, surf-backend looks for a
Chromium/Chrome/Edge install in the usual locations automatically. Audio
capture is not yet implemented on Windows.

## Quick Start

Download the `surf-backend-*-linux-*.tar.gz` archive from GitHub Releases, then:

```sh
tar xf surf-backend-*-linux-*.tar.gz
cd surf-backend-*-linux-*
./surf-backend doctor
SURF_PASSWORD='change-me' ./surf-backend serve
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
make backend-binary
SURF_PASSWORD='change-me' ./backend/surf-backend serve
```

Press `Ctrl-C` to stop the foreground backend.

## Useful Commands

```sh
make backend-binary      # build backend/surf-backend
make backend-dist        # build dist/surf-backend-*.tar.gz
```

Run the binary directly:

```sh
make backend-binary
SURF_PASSWORD='change-me' ./backend/surf-backend serve
```

## Runtime Behavior

Chromium runs headless (`--headless=new`) — no display server, virtual or
otherwise, is needed on any platform, and no window ever appears. Both the
JPEG lane (`Page.startScreencast`) and the H.264 lane (transcoded from those
same screencast frames via FFmpeg) work identically whether or not there's
even a desktop session to render into.

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
- `CHROME`, `FFMPEG`, `PULSEAUDIO`, `PACTL`: tool paths.
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
