# Surf

Surf is one executable for the desktop tray app and headless backend. It runs
the backend in an isolated child process when tray mode is active.

Launch `surf` without arguments for the tray app. Run `surf serve` for the
backend alone. Open Surf from its tray menu to see the
detected LAN address and stream state, change the password or port, expand
local logs, or restart the backend. Surf listens on port 18080 by default and
advertises itself to iOS devices on the local network.

Surf prefers an installed Chrome or Chromium. If neither is compatible, it
downloads a verified ungoogled-chromium release into `~/.surf`
(`%USERPROFILE%\.surf` on Windows). FFmpeg is included. Allow inbound TCP port
18080 if the operating-system firewall asks.

Surf captures the active Chromium tab through its built-in extension on every
platform. The captured tab is silent on the host, and Surf does not require a
virtual audio cable or record audio from unrelated applications.

On first run, Surf asks whether to start when you sign in. Change that choice
from Settings at any time. Starting Surf while it is already running simply
opens the existing Settings page.

The app is currently unsigned. Windows SmartScreen or macOS Gatekeeper may
show a warning until signed release builds are available.

Linux tray integration uses StatusNotifier/AppIndicator.
