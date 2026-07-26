# Backend

The backend is named `surf-backend`. It runs Chromium directly on Linux,
captures a private Xvfb display with FFmpeg, and streams video, audio, and
control messages to the native client.

## Dependencies

Install the host runtime tools before starting the backend:

- Chromium (`chromium`, or set `CHROME`).
- Xvfb (`Xvfb`).
- xrandr (`xrandr`).
- FFmpeg (`ffmpeg`).
- `pactl` with PulseAudio or PipeWire Pulse compatibility.
- `pulseaudio` when no existing PulseAudio-compatible server is available.

Example package installs:

```sh
# Debian/Ubuntu package names vary by release.
sudo apt install chromium xvfb x11-xserver-utils ffmpeg pulseaudio-utils pulseaudio

# Fedora package names vary by release and enabled repositories.
sudo dnf install chromium xorg-x11-server-Xvfb xrandr ffmpeg pulseaudio-utils pulseaudio

# Arch.
sudo pacman -S chromium xorg-server-xvfb xorg-xrandr ffmpeg pulseaudio
```

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

Surf starts a private Xvfb display for Chromium. Wayland desktops still use this
path; Surf does not need to attach to the visible compositor. Child processes
force X11 and clear Wayland display variables so Chromium stays inside Xvfb.

For audio, Surf first checks whether `pactl info` can reach an existing
PulseAudio-compatible server. On common PipeWire desktops this succeeds, so Surf
creates a `surf_output` null sink there and unloads it on shutdown. If no server
is available, Surf starts its own PulseAudio process.

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
- `CHROME`, `FFMPEG`, `XVFB`, `XRANDR`, `PULSEAUDIO`, `PACTL`: tool paths.
- `SURF_MANAGE_DISPLAY=0`: use an existing `DISPLAY` or `SURF_DISPLAY`.
- `SURF_MANAGE_PULSE=1`: force Surf to start a private PulseAudio process.
- `SURF_ENSURE_PULSE_SINK=0`: do not create `PULSE_SINK` automatically when
  using an existing PulseAudio/PipeWire server.
- `PULSE_SINK`: sink Chromium should use; defaults to `surf_output`.
- `AUDIO_SOURCE`: source FFmpeg captures; defaults to `surf_output.monitor`.
- `SURF_ADVERTISE=0`: disable LAN discovery advertisement.

If you run the backend as root on a VPS, Chromium usually requires
`CHROME_NO_SANDBOX=1`. Surf enables this by default for root, but running as a
normal user is preferred.

## Security

Do not expose a weak-password backend to the public internet. For VPS usage, put
it behind HTTPS and use a strong password.
