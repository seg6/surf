# Surf

Surf is one executable for the desktop tray app and headless backend. It runs
the backend in an isolated child process when tray mode is active.

Launch `surf` without arguments for the tray app. Run `surf serve` for the
backend alone. Open Surf from its tray menu to see the
detected LAN address and stream state, change the password or port, expand
local logs, or restart the backend. Surf listens on port 18080 by default and
advertises itself to iOS devices on the local network.

The first launch downloads pinned Chrome and FFmpeg runtimes into `~/.surf`
(`%USERPROFILE%\.surf` on Windows). Allow inbound TCP port 18080 if the
operating-system firewall asks.

The app is currently unsigned. Windows SmartScreen or macOS Gatekeeper may
show a warning until signed release builds are available.

Linux tray integration uses StatusNotifier/AppIndicator.
