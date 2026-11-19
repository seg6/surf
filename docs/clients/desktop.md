# Desktop client (Linux)

This is the Linux client for a Surf host, and it works on both X11 and
Wayland. For installation, see the [install guide](../guide/install-desktop.md).
Packaging details are in
[client/desktop/README.md](https://github.com/seg6/surf/blob/main/client/desktop/README.md).

It finds hosts on the local network or takes a manual address, and pairing gives
each server its own identity and a pinned certificate. The app is written in
Rust and uses FFmpeg, OpenGL, CPAL, winit and Dear ImGui, with the shared C99
core reached through `surf-core` and `surf-core-sys`.

It provides tabs, a library, clipboard sync and audio. Keyboard, mouse and wheel
input work when the server has the `pointer-input` capability. There's also a
compact interface, a searchable Library, address suggestions, phone and tablet
resolution presets, a choice of bar position, dark mode, reduced motion and
mobile websites. Settings are saved in `desktop-preferences.json`, next to the
saved servers.

## Stack

| Layer | Tech |
| --- | --- |
| UI | Dear ImGui, winit |
| Core | C99 via `surf-core` and `surf-core-sys` |
| Session | `surf-session` (TLS, pairing, API) |
| Video | FFmpeg, rendered as YUV in OpenGL |
| Audio | CPAL, bounded queue |

Network, decode, audio and UI each run independently of the others.

## Input

Servers with `pointer-input` receive pointer, wheel, keyboard, paste and IME
input. Older compatible servers only get single-contact touch commands.

## Shortcuts

| Key | Action |
| --- | --- |
| Ctrl+L | Focus the address bar |
| Enter, Escape | Navigate, or restore the address |
| Ctrl+T, Ctrl+W | New tab, close tab |
| Ctrl+F | Find |
| F11 | Fullscreen (also follows page fullscreen) |

Fullscreen hides the browser bar and expands the remote viewport.

## Tests

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
client/desktop/test-secure-session.sh
bash client/desktop/test-ui-input.sh
```

The session test requires presented frames, resize delivery, reconnect, input
and audio, zero decode errors, and recovery after a UI stall. Setting
`SURF_TEST_MIN_FPS=55` adds a frame rate check for real hardware.
