# Surf Backend

This archive contains the standalone Linux `surf-backend` binary. On first run
it downloads pinned Chrome for Testing and FFmpeg into `SURF_HOME/runtime`;
it does not bundle those large runtimes.

## Dependencies

Install the backend runtime tools with your distro package manager:

```sh
# Debian/Ubuntu package names vary by release.
sudo apt install pulseaudio-utils pulseaudio

# Fedora package names vary by release and enabled repositories.
sudo dnf install pulseaudio-utils pulseaudio

# Arch.
sudo pacman -S pulseaudio
```

PipeWire desktops usually need the PulseAudio compatibility service available to
`pactl`.

## Run

Check the runtime tools:

```sh
./surf-backend doctor
```

Start the backend:

```sh
SURF_PASSWORD='change-me' ./surf-backend serve
```

Use a strong password before exposing Surf beyond local testing. The default LAN
URL is:

```text
http://YOUR_COMPUTER_IP:18080
```

LAN discovery is enabled by default. Disable it with `SURF_ADVERTISE=0`.

## Install To User Bin

```sh
install -Dm755 surf-backend ~/.local/bin/surf-backend
SURF_PASSWORD='change-me' ~/.local/bin/surf-backend serve
```

## Optional User Service

The included `surf-backend.service` expects the binary at
`~/.local/bin/surf-backend` and an env file at `~/.config/surf-backend/env`.

```sh
install -Dm755 surf-backend ~/.local/bin/surf-backend
install -Dm644 surf-backend.service ~/.config/systemd/user/surf-backend.service
mkdir -p ~/.config/surf-backend
printf "SURF_PASSWORD=change-me\n" > ~/.config/surf-backend/env
chmod 600 ~/.config/surf-backend/env
systemctl --user daemon-reload
systemctl --user enable --now surf-backend
```

Logs:

```sh
journalctl --user -u surf-backend -f
```

## Common Overrides

- `PORT`: listen port; defaults to `18080`.
- `BIND_ADDR`: listen address; defaults to `0.0.0.0`.
- `SURF_HOME`: data root; defaults to `~/.surf`.
- `CHROME`: explicit browser override; otherwise Surf manages Chrome.
- `SURF_BROWSER_DOWNLOAD=0`: disable automatic browser downloading.
- `FFMPEG`: explicit encoder override; otherwise Surf manages FFmpeg.
- `SURF_FFMPEG_DOWNLOAD=0`: disable automatic FFmpeg downloading.
- `PULSEAUDIO`, `PACTL`: Linux audio tool paths.
- `SURF_ADVERTISE=0`: disable LAN discovery.
