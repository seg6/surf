# Install the host

Download for your OS from the
[latest release](https://github.com/seg6/surf/releases/latest).

| OS | Notes |
| --- | --- |
| Windows | 64-bit (`surf.exe`) |
| macOS | 12+, Intel or Apple Silicon |
| Linux | x86_64 or ARM, AppImage or archive |

Needs a real desktop session (Chromium and **Browser setup** need a display).
Linux: logged-in desktop, not headless SSH. Uses installed Chrome/Edge/Chromium
when compatible; otherwise a verified Chromium under `SURF_HOME`.

**Tray app**: settings, pairing, devices, logs, clipboard, updates; supervises
the backend.

**Foreground:**

```sh
./surf serve          # add --pair to invite immediately
surf status
surf pair
surf devices list
surf quit
```

Listens on TCP `18080` by default (open the firewall for LAN clients).

**Data:** `SURF_HOME` (default `~/.surf`) holds identity, devices, profile,
downloads, logs. Keep it when updating; deleting it forces every client to
re-pair. Override with `./surf --home /path serve`.

```sh
surf status
curl -k https://127.0.0.1:18080/api/v1/health
```

**Updates:** `surf update` (or the tray). Keep `SURF_HOME`. Windows installer
stops Surf, replaces, restarts. Compatibility:
[Versioning](../reference/versioning.md).

Then [pair](pairing.md) or install a [client](install-ios.md). From source:
[Build](../develop/build.md) (`make surf-binary`, `make surf-dist`).
