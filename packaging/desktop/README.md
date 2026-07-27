# Surf Desktop

Surf Desktop runs the included `surf-backend` and keeps it available from the
system tray or menu bar.

Start `surf-desktop` (`surf-desktop.exe` on Windows). Open its menu to see or
copy the generated password, open diagnostics, view logs, or restart the
backend. Surf listens on port 18080 and advertises itself to iOS devices on the
local network.

The first launch downloads pinned Chrome and FFmpeg runtimes into `~/.surf`
(`%USERPROFILE%\.surf` on Windows). Allow inbound TCP port 18080 if the
operating-system firewall asks.

The app is currently unsigned. Windows SmartScreen or macOS Gatekeeper may
show a warning until signed release builds are available.

Linux tray integration uses StatusNotifier/AppIndicator. Clipboard copying
uses `wl-copy`, `xclip`, or `xsel` when installed; the password also remains
visible in the tray menu.
