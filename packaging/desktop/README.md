# Desktop package

The desktop app supervises one Surf backend and opens a local Settings page.
Settings covers stream health, logs, host configuration, pairing, device
management, clipboard sync, and updates.

Manual pairing shows the same six word phrase on the host and device. Matching
phrases confirm the server identity. QR pairing carries that identity in the
code.

The backend serves pinned TLS on the network. Local administration uses a
private control token on the loopback interface.

Updates preserve `SURF_HOME`. That directory contains the server identity,
paired devices, browser profile, and other state. Replacing it makes saved
clients reject the host until they forget it and pair again.

The **Quit Surf** action closes the desktop app, backend, and managed Chromium
processes. `surf quit` performs the same shutdown from a terminal.
