# Surf

Surf brings the modern web to legacy iPhones, iPod touches, and iPads.

The iOS app is a native, touch-first remote browser. A computer running Surf
hosts Chromium, renders the page, and streams video and audio to the device;
the client sends navigation, keyboard, and touch input back. Modern TLS,
JavaScript, media, and optional Widevine support therefore come from the host
browser instead of the device's obsolete WebKit.

Surf is built around the device rather than a generic remote-desktop surface:
it has native tabs, an omnibox, bookmarks, history, downloads, uploads, sharing,
fullscreen media, and phone- and tablet-specific layouts inspired by classic
Safari.

## Project status

Surf is experimental. The primary tested client is an original iPad mini on
iOS 6.1.3. The release package is built for many more devices, but those
model/OS combinations should be considered build-supported until they have
been tested on hardware.

## Compatibility

One universal, rootful `iphoneos-arm` package contains both client
architectures and declares the iPhone/iPod and iPad device families:

| Client slice | Minimum OS | Hardware |
| --- | --- | --- |
| `armv7` | iOS 6.0 | 32-bit iPhone, iPod touch, and iPad models |
| `arm64` | iOS 7.0 | 64-bit A7 and newer devices through the iOS 14 generation |

The current compatibility target ends at iOS/iPadOS 14.8.1 and requires a
rootful jailbreak. A compatible jailbreak may not exist for every model and OS
combination in that range. iOS 15+, rootless packages, armv6 devices, iOS 5,
and the original iPad are not currently supported.

For the best experience, use an A5 device or newer; A7 and newer devices are
preferred. The 256 MB iPhone 3GS and iPod touch 4 are package-compatible but
experimental.

<details>
<summary>Device and OS details</summary>

### 32-bit devices

| Models | Surf-compatible OS range | Status |
| --- | --- | --- |
| iPhone 3GS | 6.0–6.1.6 | Experimental |
| iPhone 4 | 6.0–7.1.2 | Legacy candidate |
| iPhone 4s | 6.0–9.3.6 | Legacy candidate |
| iPhone 5 | 6.0–10.3.4 | Good candidate |
| iPhone 5c | 7.0–10.3.3 | Good candidate |
| iPod touch 4 | 6.0–6.1.6 | Experimental |
| iPod touch 5 | 6.0–9.3.5 | Legacy candidate |
| iPad 2 and iPad 3 | 6.0–9.3.5; 9.3.6 on cellular models | Legacy candidates |
| iPad 4 | 6.0–10.3.3; 10.3.4 on cellular models | Good candidate |
| iPad mini 1 | 6.0–9.3.5; 9.3.6 on cellular models | Verified on iOS 6.1.3 |

### 64-bit devices

| Models | Surf-compatible OS range |
| --- | --- |
| iPhone 5s | 7.0–12.5.7 |
| iPhone 6 and 6 Plus | 8.0–12.5.7 |
| iPad Air 1 and iPad mini 2 | 7.0–12.5.7 |
| iPad mini 3 | 8.0–12.5.7 |
| iPod touch 6 | 8.4–12.5.7 |
| iPhone 6s through iPhone 12, including SE 1 and SE 2 | Device launch OS–14.8.1 |
| iPad Air 2–4, iPad mini 4–5, and iPad 5–8 | Device launch OS–14.8.1 |
| iPad Pro models released through 2020 | Device launch OS–14.8.1 |
| iPod touch 7 | 12.3–14.8.1 |

</details>

The backend runs on:

- Windows x86-64
- macOS 12+ on Intel or Apple Silicon
- Linux x86-64 or ARM64

## Quick start

### 1. Start Surf on the computer

Download the package for your computer and the universal iOS `.deb` from the
[latest release](https://github.com/seg6/surf/releases/latest).

The desktop build provides a Settings window for the password, port, detected
LAN address, logs, and updates. For a terminal or server installation:

```sh
SURF_PASSWORD='choose-a-strong-password' ./surf serve
```

Surf listens on port `18080` by default. Windows archives contain `surf.exe`.

### 2. Install the iOS client

Install `space.seg6.surf_<version>_iphoneos-arm.deb` with Filza or iFile, or
copy it over SSH:

```sh
scp space.seg6.surf_*.deb root@DEVICE_IP:/tmp/surf.deb
ssh root@DEVICE_IP 'dpkg -i /tmp/surf.deb && uicache || true'
```

Respring or run `uicache` again if the icon does not appear.

### 3. Connect

Open **More > Surf Settings** in the client, tap **Find Local Surf**, select the
backend, enter its password, and connect. If discovery is unavailable, enter
the computer's LAN address manually:

```text
http://192.168.1.50:18080
```

Use the computer's address, not the address of the iOS device. If the backend
is unreachable, allow inbound TCP port `18080` through the host firewall.

## Browser and DRM support

Surf prefers a compatible installed Edge or Chromium and otherwise manages a
verified ungoogled-chromium build in its private data directory. Video and
audio come from Chromium's tab-capture APIs; no FFmpeg, PulseAudio, virtual
audio device, or desktop capture is required.

Surf does not distribute Widevine. If the selected host browser supplies a
working Widevine CDM, protected sites can use it, subject to that site's
license and output-protection rules.

## Build from source

Build the backend and the current host's desktop package:

```sh
make surf-binary
make surf-dist
```

Building the universal iOS package uses a reproducible Linux/WSL2/Docker
environment. See [Native Build](docs/native-build.md) for the SDK and packaging
steps.

## Documentation

- [Backend configuration and deployment](docs/backend.md)
- [Native client build](docs/native-build.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Changelog](CHANGELOG.md)

## Support Surf

If Surf has made an old device useful again and you'd like to buy me a coffee,
you can do that on Ko-fi.

<a href="https://ko-fi.com/seg6_">
  <img src="https://storage.ko-fi.com/cdn/kofi5.png?v=3" alt="Buy me a coffee on Ko-fi" height="36">
</a>

## AI disclosure

Surf is an AI-assisted project. Its direction, device testing, deployment
decisions, and release judgment are human-directed. Treat it like any other
experimental systems project: review the code, test your setup, and do not
expose it to the internet with a weak password.

## License

[MIT](LICENSE)
