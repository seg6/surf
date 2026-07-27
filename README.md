# Surf

Surf is a native iOS 6 client for a remote Chromium backend. It makes modern
websites usable on legacy jailbroken iOS devices without relying on the
obsolete system WebKit.

The native app provides the touch-first browser UI, input handling, H.264 video
decoding, audio playback, tabs, downloads, uploads, and device integration. The
backend, `surf-backend`, runs managed Chrome headlessly, captures compositor
frames through CDP, and streams H.264 to the device over WebSocket.

The result is closer to a purpose-built remote browser than a remote desktop:
Chromium does the web compatibility work, while the iOS app keeps the device
experience native.

## Status

Surf is a hobby project for jailbroken legacy iOS devices. It is usable, but
still experimental.

The main test target is the original iPad mini on iOS 6.1.3. Other iOS 6 iPads,
iPhones, and iPods may work, but layout and performance are less tested.

The video/browsing backend supports Linux x86-64, Windows x86-64, and macOS
Intel/Apple Silicon. It downloads pinned Chrome and FFmpeg runtimes into
`SURF_HOME` on first launch. Audio capture currently requires Linux plus
PulseAudio or PipeWire Pulse compatibility. See `docs/backend.md` for details.

## AI Disclosure

Surf is explicitly an AI-assisted project.

I am generally not an "AI codebase" person. Surf happened while I was on
vacation, with an old iPad mini in front of me and not enough personal time to
build the whole thing by hand. I used AI heavily to accelerate implementation,
iteration, and debugging. The direction, testing, device bring-up, deployment
decisions, and release judgment were human-directed.

That is disclosed here on purpose. If you are reading, modifying, packaging, or
deploying Surf, treat it like any other experimental systems project: review the
code, test your setup, and assume there are rough edges.

## Requirements

- A jailbroken iOS device.
- iOS 6 is the primary target.
- Filza, iFile, OpenSSH, or another way to install `.deb` packages.
- A Linux, Windows, or macOS computer for `surf-backend`.
- On Linux, `pactl` with PulseAudio or PipeWire Pulse compatibility for audio.
- Enough disk space for the managed Chrome and FFmpeg runtimes.
- A low-latency network between the device and backend. LAN is best.

You do not need to build the native app yourself if a `.deb` is available in
GitHub Releases for your device.

## Install The Native App

Download the latest Surf package from GitHub Releases. The filename looks like:

```text
space.seg6.surf_<version>-1_iphoneos-arm.deb
```

Install it with Filza or iFile, or install over SSH:

```sh
scp space.seg6.surf_*.deb root@DEVICE_IP:/tmp/surf.deb
ssh root@DEVICE_IP 'dpkg -i /tmp/surf.deb && uicache || true'
```

Replace `DEVICE_IP` with the IP address of your iOS device. If the icon does
not appear after installation, run `uicache` again or respring.

## Start The Backend

For a normal desktop computer, download the matching `surf-desktop` archive
from GitHub Releases. It includes both the tray app and backend:

- Windows: run `surf-desktop.exe`.
- macOS: open `Surf Desktop.app`.
- Linux: run `./surf-desktop`.

The tray/menu-bar menu shows the generated password and provides shortcuts for
logs, diagnostics, restart, and quit. The password is saved under `SURF_HOME`
so the iPad does not need to be reconfigured on every launch. The first launch
downloads the pinned Chrome and FFmpeg runtimes.

The desktop app is presently unsigned, so Windows or macOS may display a
security warning. Server and advanced users can instead use the standalone
CLI package:

On the backend computer, download the matching Linux, Windows, or macOS
`surf-backend` archive from GitHub Releases:

```sh
tar xf surf-backend-*-linux-*.tar.gz
cd surf-backend-*-linux-*
./surf-backend doctor
SURF_PASSWORD='change-me' ./surf-backend serve
```

Windows archives contain `surf-backend.exe`. macOS archives contain the same
CLI commands shown above and are published separately for Intel and Apple
Silicon.

Use a strong password before exposing Surf beyond local testing.

To build from source instead:

```sh
make backend-binary
SURF_PASSWORD='change-me' ./backend/surf-backend serve
```

Build the native desktop package on the matching operating system with:

```sh
make desktop-dist
```

By default, `surf-backend serve`:

- Listens on port `18080`.
- Advertises the backend on the local network for Surf discovery.
- Reads required `SURF_PASSWORD` from the environment.
- Downloads pinned Chrome and FFmpeg runtimes on first launch.
- Runs Chrome with `--headless=new` and uses CDP screencast capture.

To run the binary directly:

```sh
make backend-binary
SURF_PASSWORD='change-me' ./backend/surf-backend serve
```

## Connect From Surf

Open Surf on the iOS device.

The easiest path is local discovery:

1. Open Settings in Surf.
2. Tap `Find Local Surf`.
3. Select the discovered backend.
4. Enter the password from `SURF_PASSWORD`.
5. Tap `Connect`.

If discovery does not find the backend, enter the URL manually:

```text
http://YOUR_COMPUTER_IP:18080
```

Example:

```text
http://192.168.1.50:18080
```

Use the LAN IP address of the computer running `surf-backend`, not the IP
address of the iOS device.

## Firewall Notes

If Surf can see the backend but cannot connect, the computer firewall is often
blocking inbound TCP port `18080`.

For Linux systems using `ufw`:

```sh
sudo ufw allow from 192.168.0.0/16 to any port 18080 proto tcp
```

If you use another firewall, allow inbound TCP port `18080`.

## What Surf Is

- A native client for legacy jailbroken iOS devices.
- A cross-platform remote Chromium backend named `surf-backend`.
- A touch-first remote browser, not a generic remote desktop.
- Intended for LAN use, with VPS deployment possible if latency is acceptable.

## What Surf Is Not

- It is not a standalone WebKit replacement.
- It is not a browser that works without a backend.
- It is not an App Store app.
- It is not trying to make iOS 6 Safari compatible with the modern web.

## FAQ

### Does Surf require a computer or server?

Yes. Chromium runs on the backend. The iOS app is the native client for that
backend.

### Can the backend run on a VPS?

Yes. LAN usually feels better, but a VPS works if latency is reasonable. Use
HTTPS and a strong password for any internet-facing deployment.

### Why not build a normal browser for iOS 6?

Third-party browsers on iOS 6 are still tied to the system WebKit. That engine
is missing too much modern TLS, JavaScript, and CSS support for today's web.
Running Chromium elsewhere avoids that limitation.

### Does it work on iPhone or iPod touch?

The primary target is iPad-sized iOS 6 hardware. Smaller devices may work, but
they are less tested.

### Do I need the iOS 6 SDK?

Only if you want to build the native app yourself. Most users should install the
prebuilt `.deb` from Releases.

## More Docs

- `docs/backend.md`: backend configuration and deployment notes.
- `docs/native-build.md`: building the iOS `.deb`.
- `docs/troubleshooting.md`: common connection and install issues.

## License

MIT
