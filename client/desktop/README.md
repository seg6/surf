# Surf Desktop Client

This workspace contains Surf's compact Dear ImGui client for Linux. It is a
developer control surface over the same portable C99 behavior core used by iOS;
transport and media remain reusable Rust crates with no UI-toolkit dependency.

Current status: the preview is integration-verified on X11/x86-64. Winit owns
the host window and input loop, glutin owns the graphics context, and Dear ImGui
draws deliberately dense browser controls and overlays. Wayland remains
build-supported rather than part of the automated certification job.

This is a genuine usable client rather than a replay shell. It discovers nearby
servers (with manual entry as a fallback), persists a per-server device
identity, completes phrase-confirmed pairing, pins the exact server
certificate, authenticates, and opens the production WebSocket. A dedicated
FFmpeg worker decodes the newest eligible H.264 access unit into pooled YUV420
planes, and the OpenGL video surface uploads and converts those planes directly
on the GPU.
Networking, decoding, presentation, and UI work do not queue behind one
another. Reconnect backoff is decided by the C99 core policy available to every
platform host. Signed 16-bit backend PCM travels through its own bounded lane
and a 120 ms maximum jitter window; the CPAL device callback resamples and
duplicates mono into the host's native output format without blocking video.
The C99 core also owns surface-scoped input sequencing and normalized samples.
On a server advertising `pointer-input`, winit pointer, wheel, modifier,
keyboard, clipboard paste, and IME events use a bounded ordered desktop lane;
older servers receive the established single-contact touch fallback.

The desktop chrome is one 32-pixel row: back/forward/reload, a compact tab
selector with an independent new-tab button, a collapsed-hostname omnibox,
bookmark, and tools controls. Ctrl+L/T/W/R/D/F, F5, F11, Escape, and Alt+Arrow
shortcuts are available. Pairing, history, bookmarks, downloads, reader mode,
page search, media controls, file upload, JavaScript dialogs and selects,
settings, server management, and pipeline diagnostics are compact overlays
over the live page. Downloads stream into the user's Downloads folder and
uploads never block the control socket.

Settings includes Linux-only device-window presets for common iPhone and iPad
layout sizes. The dimensions are UIKit points, so they reproduce the amount of
interface space rather than multiplying it by a Retina scale factor. Choose a
device and orientation, then apply it to request that exact logical client
area. Tiling window managers can intentionally refuse application-requested
sizes; float the Surf window before applying a preset when that happens.

On Debian/Ubuntu development hosts, install the native media headers first:

```sh
sudo apt-get install libasound2-dev libavcodec-dev libavformat-dev libavutil-dev libgl1-mesa-dev
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
needs compatible FFmpeg, ALSA, window-system, and OpenGL runtime libraries;
these are ordinary distro packages on the initially supported Linux systems.

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
frames at no less than 55 FPS while resizing the real winit window twice,
actively editing the ImGui omnibox, and draining cleanly after a deliberate
180 ms UI stall forces the bounded decoded-frame slot to replace stale output.
The test also verifies those resized viewport dimensions reached the backend
and rejects transient invalid viewport sizes.
