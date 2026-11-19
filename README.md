<p align="center">
  <img src="backend/cmd/surf/surf-icon.png" alt="Surf icon" width="80" height="80">
</p>

<h1 align="center">Surf</h1>

<p align="center">
  A modern browser for devices that can't run one anymore.
</p>

<p align="center">
  <a href="https://github.com/seg6/surf/releases/latest">Download</a> | 
  <a href="https://seg6.space/surf/">Website</a> | 
  <a href="docs/index.md">Docs</a>
</p>

Surf runs Chromium on a computer and streams it to a native app on another
device. The app works like a regular browser, with tabs, an address bar,
bookmarks and history, while pages load and run in Chromium on the computer.
Sites see an up-to-date desktop browser, so they work on devices whose own
browser is long out of date.

Clients:

- iOS 6 through 14 (jailbroken), the original client
- Linux desktop
- PS Vita, a [community port](https://github.com/jmverges/surf-vita)

The [porting guide](docs/clients/porting.md) covers building a client for
another platform.

## Demo

https://github.com/user-attachments/assets/5e684036-9bbf-4fce-a088-0d95dcc1a890

The demo is 50 seconds long and has sound. It's also available as a
[video file](docs/media/surf-demo.mp4?raw=true), and the
[credits](docs/media/README.md) list the content shown in it.

## Getting started

1. Download the [latest release](https://github.com/seg6/surf/releases/latest)
   for your computer and start it. It runs from the tray, or from a terminal
   with `./surf serve`.
2. Install a client. On iOS, add `https://seg6.space/surf/` as a source in
   Cydia or Sileo and install Surf ([details](docs/guide/install-ios.md)). For
   Linux, see the [desktop client guide](docs/guide/install-desktop.md).
3. Pair them. Open Settings from the tray and choose Pair device under Paired
   Devices, or run `surf pair`. Then scan the QR code with the app, or type in
   the address and code. Confirm that the six words match on both screens.

For sites that are hard to sign in to through the stream, open Browser setup
from the tray (or run `surf browser`) and sign in on the computer. Paired
devices use the same browser profile afterwards. See the
[browser setup guide](docs/guide/browser-setup.md) for details, and
[remote access](docs/guide/remote-access.md) for connecting from outside your
network.

Surf keeps its data, including pairings and the browser profile, in
`SURF_HOME`. Keep that folder when updating.

## Requirements

The host runs on 64-bit Windows, macOS 12 or newer, and Linux (x86_64
and ARM). It uses Chrome, Edge or Chromium if one is already installed,
and downloads its own copy of Chromium otherwise. Surf doesn't bundle Widevine,
so DRM video like Netflix only plays when the installed browser provides it.

## Building from source

Everything lives in this repo: the Go host, the shared C client core, the iOS
app and the Linux client.

```sh
make surf-binary   # build the host for this computer
make test          # run the tests
```

The iOS package has its own [build guide](docs/develop/native-ios.md). The
[architecture](docs/concepts/architecture.md) and
[protocol](docs/reference/protocol.md) docs describe how the pieces fit
together, and the [changelog](CHANGELOG.md) lists changes by release.

## Support

Surf can be supported on [Ko-fi](https://ko-fi.com/seg6_).

Surf is developed with AI assistance. Direction, device testing and releases
are done by the maintainer. Licensed under [MIT](LICENSE).
