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

Surf installs managed ungoogled-chromium when no compatible system Edge or
Chromium is present. Compatible Microsoft Edge installations are detected on
each desktop platform.
If a download is blocked, permit access to GitHub Releases or set `CHROME` to an
explicit extension-capable local executable. Audio and video use Surf's built-in
Chromium extension on every platform and need no capture driver.

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

## Native Package Will Not Install Or Launch

The official native package is `iphoneos-arm` for rootful jailbreaks. It
contains an armv7 slice with an iOS 6.0 minimum and an arm64 slice with an iOS
7.0 minimum. It enables iPhone, iPod touch, and iPad through iOS 14; it is not a
rootless `iphoneos-arm64` package.

After a local build, verify the exact package recorded by Theos:

```sh
cat native/client/.theos/last_package
docker run --rm -v "$PWD:/src" surf-buildenv \
  bash /src/native/client/verify-package.sh
```

The verifier checks both binaries and fails if a CPU slice, minimum iOS
version, package identifier, architecture value, device family, required
phone/tablet resource, or privileged updater mode is wrong. On the device,
reinstall that exact package, run `uicache`, and respring before diagnosing a
launch failure.

## Widevine Or DRM Media Does Not Play

Surf never downloads or redistributes Widevine. It uses the selected browser's
own Encrypted Media Extensions and CDM installation on Linux, Windows, and
macOS. Component registration and updates remain enabled so a browser-provided
CDM can load.

After an HTTPS or localhost page finishes loading, authenticated
`/health?stats=1` output reports `widevine` as `available`, `unavailable`, or
`unknown`. `unknown` means Surf has not yet had a trustworthy page on which to
run the probe. The managed ungoogled-chromium fallback usually reports
`unavailable`; select a compatible browser that includes Widevine with
`CHROME=/path/to/browser`.

An `available` result confirms `com.widevine.alpha` is exposed to the page, but
does not override the streaming provider's license, HDCP, resolution, or
capture policy. Protected video can still be rejected or appear black if the
provider forbids this output path.

Account sign-in happens before DRM playback and can fail independently. Surf
preserves coherent browser and platform Client Hints for sites with anti-bot
checks, but it does not bypass CAPTCHAs or a provider's account-security rules.
If a provider returns a generic sign-in error, clear that site's cookies, leave
mobile-site mode off, and retry before treating it as a Widevine problem.

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
