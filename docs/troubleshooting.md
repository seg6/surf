# Troubleshooting

## Surf Cannot Connect To LAN Backend

Check the backend health endpoint from another device on the same network:

```text
http://YOUR_COMPUTER_IP:18080/health
```

If local health works but the iOS device times out, your computer firewall is
probably blocking inbound TCP `18080`.

For `ufw`:

```sh
sudo ufw allow from 192.168.0.0/16 to any port 18080 proto tcp
```

## Wrong Password

Restart the backend with the intended `SURF_PASSWORD`, then reconnect from Surf.

## Backend Tool Check Fails

Run:

```sh
make surf-binary
./backend/surf doctor
```

Surf installs managed ungoogled-chromium when no compatible system Chrome or
Chromium is present. Development builds may also download FFmpeg. If a download
is blocked, permit access to GitHub
Releases, or set `CHROME`/`FFMPEG` to explicit local executables. Linux audio
also requires the PulseAudio-compatible tools reported by `doctor`.

## Managed Runtime Download Fails

Retry with the default managed runtime settings:

```sh
make surf-binary
unset CHROME FFMPEG SURF_BROWSER_DOWNLOAD SURF_FFMPEG_DOWNLOAD SURF_CONTENT_BLOCKER_DOWNLOAD
SURF_PASSWORD='change-me' ./backend/surf serve
```

Downloads are checksum-verified and stored below `SURF_HOME/runtime`. This
includes the browser, FFmpeg, and uBlock Origin Lite. Set the corresponding
`*_DOWNLOAD=0` variable only when provisioning that runtime yourself.

## Browser Is Visible Or Frames Stop When Unfocused

The current backend always uses Chrome `--headless=new`; it does not use Xvfb
or desktop capture. A visible window means an old backend process or an
explicitly launched browser is still running. Stop all old `surf serve`
processes, rebuild, and start the current binary.

## App Installed But Icon Did Not Update

Run on the device:

```sh
uicache
```

If needed, respring.

## Logs

Backend:

```sh
SURF_PASSWORD='change-me' ./backend/surf serve
```

The backend runs in the foreground and writes logs to the terminal.

Native app:

```text
/var/mobile/Library/Surf/surf.log
```

Triple-tap the streamed page to show the on-device performance overlay. Test
FPS only while the page is visibly moving: a static page normally has a low AU
rate because CDP does not resend identical frames. Useful signals are:

- `AU RATE`: frames arriving from the backend.
- `PRESENTED`: frames shown by the display link, rather than merely decoded.
- `VT CALLBACK`: VideoToolbox decode completion time.
- `INPUT → SCREEN`: the complete interaction-to-presentation duration.
- `RTT`: current control connection round-trip time.

For a reproducible motion-only check, run the repository's local probe while a
development backend owns `~/.surf/profile`:

```sh
cd backend
go run ./tools/motionprobe -duration 30s
```

This verifies capture, encoding, transport, decode, and presentation. It does
not simulate an iPad touch, so use a real scroll to evaluate `INPUT → SCREEN`.
