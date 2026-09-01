# Surf Desktop Client

This workspace contains the real Rust/egui Surf client, initially certified on
Linux. It embeds the same portable C99 core used by the native iOS host.

This is a genuine usable client rather than a replay shell. It discovers nearby
servers (with manual entry as a fallback), persists a per-server device
identity, completes phrase-confirmed pairing, pins the exact server
certificate, authenticates, and opens the production WebSocket. A dedicated
FFmpeg worker decodes the newest eligible H.264 access unit into pooled YUV420
planes, and a custom egui/glow callback uploads and converts those planes on the
GPU. Networking, decoding, presentation, and UI work do not queue behind one
another. Reconnect backoff is decided by the same C99 core policy available to
other platform hosts. Signed 16-bit backend PCM travels through its own bounded
lane and a 120 ms maximum jitter window; the CPAL device callback resamples and
duplicates mono into the host's native output format without blocking video.
The C99 core also owns surface-scoped input sequencing and normalized samples.
On a server advertising `pointer-input`, egui mouse, wheel, modifier, keyboard,
clipboard paste, and IME events use a bounded ordered desktop lane; older
servers receive the established single-contact touch fallback.

The desktop chrome uses a horizontally scrollable tab runway with independent
new/close controls and a stable command rail. The omnibox shows the meaningful
hostname while idle, expands to the complete address for editing, and supports
the standard Ctrl+L/T/W/R/D/F, F5, Escape, and Alt+Arrow browser shortcuts.
Browser tools cover history, bookmarks, downloads, reader mode, page search,
media controls, file upload, JavaScript dialogs and selects, fullscreen,
clipboard handoff, light/dark appearance, mobile-site requests, and shared
pipeline diagnostics. Downloads are streamed into the user's Downloads folder
and uploads never block the control socket.

On Debian/Ubuntu development hosts, install the native media headers first:

```sh
sudo apt-get install libasound2-dev libavcodec-dev libavformat-dev libavutil-dev
```

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
client/desktop/test-secure-session.sh
client/desktop/package-linux.sh
```

The package command builds a locked release binary, rejects unresolved native
libraries, embeds the Surf icon and desktop entry, and writes a versioned
`dist/surf-desktop-<version>-linux-<arch>.tar.gz` archive. The target machine
needs compatible FFmpeg and ALSA runtime libraries; these are ordinary distro
packages on the initially supported Linux systems.

To install the extracted archive for all users:

```sh
sudo install -m 0755 bin/surf-client /usr/local/bin/surf-client
sudo install -m 0644 share/applications/space.seg6.surf.client.desktop \
  /usr/local/share/applications/space.seg6.surf.client.desktop
sudo install -m 0644 share/icons/hicolor/1024x1024/apps/space.seg6.surf.client.png \
  /usr/local/share/icons/hicolor/1024x1024/apps/space.seg6.surf.client.png
```

When exactly one paired server is saved, the desktop client verifies and
reconnects to it automatically. The integration test builds an ordinary Surf
backend, performs real phrase-confirmed pairing, receives real PCM, and decodes
a live browser frame. Its headless OpenGL run then presents 600 unique animated
frames at no less than 55 FPS while resizing twice, actively editing the
omnibox, and draining cleanly after a deliberate 180 ms UI stall forces the
bounded decoded-frame slot to replace stale output.
