# Install the host

Download the build for your system from the
[latest release](https://github.com/seg6/surf/releases/latest). Windows gets a
64-bit `surf.exe`. macOS needs version 12 or newer and runs on Intel or Apple
Silicon. On Linux you can use the AppImage or the archive, on x86_64 or ARM.

The host needs a real desktop session, since Chromium and Browser setup both
need a display. On Linux that means a logged-in desktop, not a headless SSH
session. Surf uses an installed Chrome, Edge or Chromium when it's compatible,
and otherwise downloads a verified Chromium into `SURF_HOME`.

The tray app gives you settings, pairing, devices, logs, clipboard and updates,
and it supervises the backend for you.

You can also run it in the foreground:

```sh
./surf serve          # add --pair to invite immediately
surf status
surf pair
surf devices list
surf quit
```

It listens on TCP port `18080` by default, so open the firewall if clients
connect over your LAN.

Surf keeps its data in `SURF_HOME`, which defaults to `~/.surf`. That includes
the identity, devices, browser profile, downloads and logs. Keep it when you
update, because deleting it makes every client pair again. To use a different
location, run `./surf --home /path serve`.

To check that everything is up:

```sh
surf status
curl -k https://127.0.0.1:18080/api/v1/health
```

To update, run `surf update` or use the tray. Keep `SURF_HOME` as it is. On
Windows the installer stops Surf, replaces it and starts it again. For version
compatibility, see [versioning](../reference/versioning.md).

Once it's running, [pair](pairing.md) a device or install a
[client](install-ios.md). If you're building from source, the
[build guide](../develop/build.md) covers `make surf-binary` and
`make surf-dist`.
