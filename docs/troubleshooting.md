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
Chromium is present. If a download is blocked, permit access to GitHub Releases
or set `CHROME` to an explicit local executable. Audio and video use Surf's
built-in Chromium extension on every platform and need no capture driver.

## Managed Runtime Download Fails

Retry with the default managed runtime settings:

```sh
make surf-binary
unset CHROME SURF_BROWSER_DOWNLOAD SURF_CONTENT_BLOCKER_DOWNLOAD
SURF_PASSWORD='change-me' ./backend/surf serve
```

Downloads are checksum-verified and stored below `SURF_HOME/runtime`. This
includes the browser and uBlock Origin Lite. Set the corresponding
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

Triple-tap the streamed page to show the on-device performance overlay. The
H.264 lane remains paced at 30 FPS even on a static page; evaluate visible
smoothness on a moving or scrolling page. Useful signals are:

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

This verifies capture, encoding, transport, decode, and presentation. Exercise
Chromium's compositor scroll path separately with the same high-frequency
pixel-wheel events used by the native client:

```sh
go run ./tools/motionprobe -scroll -duration 30s
```

The scroll probe still bypasses the iPad input/network leg, so use a real
finger drag to evaluate `INPUT → SCREEN` and the backend's motion-source gap
and stall counters.
