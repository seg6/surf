# Surf

Surf is one executable for the desktop tray app and headless backend. It runs
the backend in an isolated child process when tray mode is active.

Launch `surf` without arguments for the tray app. Run `surf serve` for the
backend alone. Open Surf from its tray menu to see the
detected LAN address and stream state, change the password or port, expand
local logs, or restart the backend. Surf listens on port 18080 by default and
advertises itself to compatible Surf clients on the local network. Release
desktop builds embed the same universal rootful iPhone/iPod/iPad package for
armv7/iOS 6 and arm64/iOS 7–14, regardless of the desktop computer's CPU
architecture.

Surf prefers an installed extension-capable Edge or Chromium. If neither is compatible, it
downloads a verified ungoogled-chromium release into `~/.surf`
(`%USERPROFILE%\.surf` on Windows). Allow inbound TCP port 18080 if the
operating-system firewall asks.

Surf captures the active Chromium tab through its built-in extension on every
platform. Chromium stays silent on the host even when the client disconnects or
the capture parks. Surf does not require a virtual audio cable or record audio
from unrelated applications.

Surf does not package Widevine, but it leaves browser CDM/component loading
enabled on every desktop platform. If the selected browser supplies Widevine,
authenticated runtime statistics report it after an HTTPS or localhost page
loads. Managed ungoogled-chromium generally does not supply it, and DRM output
policy can still prevent capture.

On first run, Surf asks whether to start when you sign in. Change that choice
from Settings at any time. Starting Surf while it is already running simply
opens the existing Settings page.

The app is not signed with a trusted publisher certificate or notarized.
Windows SmartScreen or macOS Gatekeeper may show a warning until signed release
builds are available.

After copying `Surf.app` from the DMG to Applications, macOS users can clear
the downloaded quarantine metadata if the app is blocked:

```sh
xattr -cr /Applications/Surf.app
open /Applications/Surf.app
```

Linux tray integration uses StatusNotifier/AppIndicator.
