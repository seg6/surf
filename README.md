<p align="center">
  <img src="backend/cmd/surf/surf-icon.png" alt="Surf icon" width="80" height="80">
</p>

<h1 align="center">Surf</h1>

<p align="center">
  Surf brings the modern web to older iPhones, iPod touches, and iPads.
</p>

<p align="center">
  <a href="https://github.com/seg6/surf/releases/latest">Download Surf</a> ·
  <a href="#quick-start">Get started</a> ·
  <a href="#documentation">Documentation</a>
</p>

Websites run in Chromium on a Windows, macOS, or Linux computer and stream to
your device. You browse through a native iOS app with touch, keyboard input,
video, and audio, without being limited by the device's outdated web engine.

The iOS app has native tabs, an address bar, bookmarks, history, downloads,
uploads, sharing, fullscreen media, and separate phone and tablet layouts.

## See it in action

https://github.com/user-attachments/assets/5e684036-9bbf-4fce-a088-0d95dcc1a890

50 seconds. Unmute for audio. [Download the video](docs/media/surf-demo.mp4?raw=true) ·
[About the recording and credits](docs/media/README.md)

## Compatibility

The rootful `iphoneos-arm` package contains both client architectures.

| Slice | Minimum OS | Hardware |
| --- | --- | --- |
| `armv7` | iOS 6.0 | 32 bit iPhone, iPod touch, and iPad models |
| `arm64` | iOS 7.0 | A7 and newer devices through iOS 14 |

The current target ends at iOS and iPadOS 14.8.1. A rootful jailbreak is
required. iOS 15 and later, rootless packages, armv6 devices, iOS 5, and the
original iPad are unsupported.

A5 or newer hardware is recommended. The 256 MB iPhone 3GS and iPod touch 4
can install the package but remain experimental.

The main hardware test device is an original iPad mini running iOS 6.1.3.
Pairing, saved servers, touch and keyboard input, video, audio, rotation, and
fullscreen are tested there. Build support for other devices does not yet
mean each device and OS combination has been tested on hardware.

<details>
<summary>Device and OS details</summary>

### 32 bit devices

| Models | Surf compatible OS range | Status |
| --- | --- | --- |
| iPhone 3GS | 6.0 to 6.1.6 | Experimental |
| iPhone 4 | 6.0 to 7.1.2 | Legacy candidate |
| iPhone 4s | 6.0 to 9.3.6 | Legacy candidate |
| iPhone 5 | 6.0 to 10.3.4 | Good candidate |
| iPhone 5c | 7.0 to 10.3.3 | Good candidate |
| iPod touch 4 | 6.0 to 6.1.6 | Experimental |
| iPod touch 5 | 6.0 to 9.3.5 | Legacy candidate |
| iPad 2 and iPad 3 | 6.0 to 9.3.5, or 9.3.6 on cellular models | Legacy candidates |
| iPad 4 | 6.0 to 10.3.3, or 10.3.4 on cellular models | Good candidate |
| iPad mini 1 | 6.0 to 9.3.5, or 9.3.6 on cellular models | Verified on iOS 6.1.3 |

### 64 bit devices

| Models | Surf compatible OS range |
| --- | --- |
| iPhone 5s | 7.0 to 12.5.7 |
| iPhone 6 and 6 Plus | 8.0 to 12.5.7 |
| iPad Air 1 and iPad mini 2 | 7.0 to 12.5.7 |
| iPad mini 3 | 8.0 to 12.5.7 |
| iPod touch 6 | 8.4 to 12.5.7 |
| iPhone 6s through iPhone 12, including SE 1 and SE 2 | Launch OS to 14.8.1 |
| iPad Air 2 through 4, iPad mini 4 and 5, and iPad 5 through 8 | Launch OS to 14.8.1 |
| iPad Pro models released through 2020 | Launch OS to 14.8.1 |
| iPod touch 7 | 12.3 to 14.8.1 |

</details>

The host computer must run a 64 bit version of Windows, macOS 12 or later, or
Linux. Both x86_64 and ARM Linux hosts are supported.

## Quick start

### Start the host

Download a host package from the
[latest release](https://github.com/seg6/surf/releases/latest).

The desktop app includes settings, pairing, device management, logs, clipboard
sync, and updates. A terminal or server installation runs in the foreground.

```sh
./surf serve
```

Useful commands from another terminal are:

```sh
surf status
surf pair
surf devices list
surf quit
```

Surf listens on port `18080` by default. Windows archives contain `surf.exe`.

To sign in or change settings in Surf's browser on the computer, choose
**Browser setup…** in the tray/dashboard or run `surf browser`. Connected
devices pause until you close the setup windows or choose to resume.
See [browser setup](docs/backend.md#browser-setup-on-the-computer) for standalone
use, confirmations and what carries over.

Updates install over the existing copy. `SURF_HOME` contains the server
identity, paired devices, and browser profile, so it should remain in place.

### Install the iOS app

Add the [Surf package repository](https://seg6.space/surf/) to Cydia or Sileo
on the jailbroken device.

```text
https://seg6.space/surf/
```

The same page also offers the current `.deb`. A manual SSH installation uses:

```sh
scp space.seg6.surf_*.deb root@DEVICE_IP:/tmp/surf.deb
ssh root@DEVICE_IP 'dpkg -i /tmp/surf.deb'
ssh root@DEVICE_IP 'su mobile -c uicache || uicache || true'
```

Older jailbreaks may reject `uicache` when it runs as `root`. Run it as
`mobile`, then respring if the icon is still missing.

### Pair

Open **Paired Devices** on the host and choose **Pair device**. A headless host
uses `surf pair`. Each invitation accepts one device.

Camera equipped devices can scan the QR code. Manual pairing uses the host
address and six digit code, then compares the same six words on both ends.

```text
192.168.1.50:18080
```

Use the host address and allow inbound TCP port `18080` through its firewall.
The words confirm the server identity. Cancel pairing if they differ.

Surf handles TLS directly. A LAN or VPS setup needs one reachable Surf port.
Set `SURF_PUBLIC_ADDRESS=host:port` when the address in pairing codes must
differ from the listener address.

Cloudflare Tunnel can carry the pinned Surf connection for remote access.

```sh
SURF_PUBLIC_ADDRESS=surf-roam.example.net:443 \
SURF_TUNNEL_HOST=surf-roam.example.net \
./surf serve
```

The inner Surf connection remains encrypted between the device and host.
Cloudflare can see connection metadata and encrypted traffic volume. A direct
LAN endpoint gives lower local latency.

See [Security](docs/security.md) for the trust model.

## Browser support

Surf uses an installed Google Chrome, Microsoft Edge, or Chromium when one is
compatible. Otherwise it installs a verified ungoogled Chromium build in
`SURF_HOME`. Capture and content blocking extensions are loaded through the
DevTools extension API.

Video and audio come from Chromium tab capture. FFmpeg, PulseAudio, virtual
audio devices, and desktop capture are not required on the host.

The stream matches the page area left by the native controls. Rotation, Surf
fullscreen, and page fullscreen update that area without reconnecting the
browser session.

The optional adaptive video mode changes frame rate when an old device falls
behind. It does not reduce the page viewport or render size.

Surf does not include Widevine. Protected sites can use a working Widevine CDM
provided by the selected host browser, subject to the site's own rules.

## Build

Build the backend and a package for the current host.

```sh
make surf-binary
make surf-dist
```

The release target builds the desktop archives. Linux AppImage builds need
`appimagetool`, and Windows installer builds need `makensis` from NSIS.

```sh
APPIMAGETOOL=/path/to/appimagetool make surf-release-dist \
  CLIENT_DEB=path/to/space.seg6.surf_VERSION_iphoneos-arm.deb
```

macOS archives contain Intel or Apple Silicon `Surf.app` bundles. Their build
does not require Xcode, an Apple SDK, signing, or a macOS host.

The universal iOS package uses a pinned Linux, WSL2, or Docker environment.
See [Native Build](docs/native-build.md).

Run the normal test suite with `make test`. Backend download tests include a
real Chromium round trip using a temporary profile and local HTTP fixtures.
They discover an installed Chrome/Chromium automatically; `SURF_TEST_BROWSER`
can select another executable. Without Chromium these tests explicitly skip
locally and fail in CI. No separate download-test command or CI step is needed.

## Documentation

- [Backend](docs/backend.md)
- [Client architecture](docs/client-architecture.md)
- [Linux client and development tools](client/desktop/README.md)
- [Porting clients](docs/porting-clients.md)
- [Protocol](docs/protocol.md)
- [Versioning](docs/versioning.md)
- [Native build](docs/native-build.md)
- [Security](docs/security.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Changelog](CHANGELOG.md)

## Support

Donations are accepted on [Ko-fi](https://ko-fi.com/seg6_).

## AI disclosure

Surf is an AI-assisted project. Its direction, device testing, deployment
decisions, and release judgment are human-directed.

## License

[MIT](LICENSE)
