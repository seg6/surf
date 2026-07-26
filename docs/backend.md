# Backend

The backend is named `surf-backend`. It runs Chromium inside Docker, captures
the X display with ffmpeg, and streams H.264/audio/control messages to the
native client.

## Local LAN Launcher

From `backend/`, `./start.sh` builds the Docker image, hashes required
`SURF_PASSWORD`, starts the container with host networking, and enables Bonjour
discovery. It refuses to start when `SURF_PASSWORD` is unset.

Configuration is read from `backend/.env`:

```sh
SURF_PASSWORD=change-me
PORT=18080
```

## Docker Compose

`backend/docker-compose.yml` defines a `surf-backend` service for deployments
where you want to provide `AUTH_HASH` yourself.

Generate a hash with:

```sh
cd backend
docker build -t surf-backend:lan .
docker run --rm --entrypoint /app/surf-backend surf-backend:lan -hash-password 'your-password'
```

## Important Ports

- `18080`: default LAN port used by `./start.sh`.
- `8080`: default internal container port in compose deployments.

## Experimental Linux Host Mode

`surf-backend` can also run directly on Linux without Docker. Host mode expects
runtime tools to be installed on the machine:

- Chromium (`chromium` or set `CHROME`).
- Xvfb (`Xvfb`).
- xrandr (`xrandr`, optional; resize falls back when missing).
- FFmpeg (`ffmpeg`).
- PulseAudio and pactl (`pulseaudio`, `pactl`), or PipeWire's PulseAudio
  compatibility service with `pactl`.

Example dependency installs:

```sh
# Debian/Ubuntu package names vary by release.
sudo apt install chromium xvfb x11-xserver-utils ffmpeg pulseaudio-utils pulseaudio

# Fedora package names vary by release and enabled repositories.
sudo dnf install chromium xorg-x11-server-Xvfb xrandr ffmpeg pulseaudio-utils pulseaudio

# Arch.
sudo pacman -S chromium xorg-server-xvfb xorg-xrandr ffmpeg pulseaudio
```

Build the binary from the repo root:

```sh
make backend-binary
```

Generate a password hash and start host mode:

```sh
AUTH_HASH="$(./backend/surf-backend -hash-password 'your-password')" \
SURF_RUNTIME=host \
PORT=18080 \
./backend/surf-backend
```

Check whether the configured host tools are present:

```sh
SURF_RUNTIME=host ./backend/surf-backend -doctor
```

Host mode stores data under `~/.surf/` by default and starts private Xvfb and
PulseAudio processes for Chromium. Docker remains the safest path until host
mode is smoke-tested on your target Linux system.

Wayland desktops still use this private Xvfb path; Surf does not need to attach
to the user's compositor. Install Xvfb even on Wayland systems.

On systems using PipeWire instead of the `pulseaudio` daemon, tell Surf to use
the existing PulseAudio-compatible server. Surf will create the `surf_output`
null sink with `pactl` and unload it on shutdown:

```sh
AUTH_HASH="$(./backend/surf-backend -hash-password 'your-password')" \
SURF_RUNTIME=host \
SURF_MANAGE_PULSE=0 \
PORT=18080 \
./backend/surf-backend
```

When `PULSE_SERVER` is unset in this mode, Chromium and ffmpeg use the default
PulseAudio/PipeWire socket for the current user. `PULSE_SINK=surf_output` keeps
Chromium's audio off the host speakers. Set `SURF_ENSURE_PULSE_SINK=0` only if
you already created the sink yourself.

If you run host mode as root on a VPS, Chromium may require
`CHROME_NO_SANDBOX=1`. Prefer running as a normal user when possible.

Useful host-mode overrides:

- `CHROME`, `FFMPEG`, `XVFB`, `XRANDR`, `PULSEAUDIO`, `PACTL`: tool paths.
- `SURF_HOME`: data root; defaults to `~/.surf`.
- `BIND_ADDR`: listen address; defaults to `0.0.0.0`.
- `SURF_MANAGE_DISPLAY=0`: use an existing `DISPLAY` or `SURF_DISPLAY`.
- `SURF_MANAGE_PULSE=0`: use an existing `PULSE_SERVER`.
- `SURF_ENSURE_PULSE_SINK=0`: do not create `PULSE_SINK` automatically when
  using an existing Pulse/PipeWire server.
- `PULSE_SINK`: sink Chromium should use; defaults to `surf_output`.
- `AUDIO_SOURCE`: source ffmpeg captures; defaults to `surf_output.monitor`.

## Security

Do not expose a weak-password backend to the public internet. For VPS usage,
put it behind HTTPS and use a strong password.
