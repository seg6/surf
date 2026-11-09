# Desktop client (Linux)

Dear ImGui remote UI for a Surf host (not a second host). Session tests run on
X11 x86_64; Wayland builds. Install: [guide](../guide/install-desktop.md).
Packaging: [client/desktop/README.md](https://github.com/seg6/surf/blob/main/client/desktop/README.md).

LAN discovery or manual address; pairing with per-server identity and cert pin.
Platform stack: Rust, FFmpeg, OpenGL, CPAL, winit, and Dear ImGui, with the
shared C99 core through `surf-core` / `surf-core-sys`.

Tabs, library, clipboard, audio; keyboard/mouse/wheel when `pointer-input` is
on. Compact chrome, searchable Library, address suggestions, phone/tablet
resolution presets, bar position, dark mode, reduced motion, mobile websites.
Settings: `desktop-preferences.json` beside saved servers.

## Stack

| Layer | Tech |
| --- | --- |
| UI | Dear ImGui, winit |
| Core | C99 via `surf-core` / `surf-core-sys` |
| Session | `surf-session` (TLS, pairing, API) |
| Video | FFmpeg -> OpenGL YUV |
| Audio | CPAL, bounded queue |

Network, decode, audio, and UI run independently.

## Input

Servers with `pointer-input` receive pointer, wheel, keyboard, paste, and IME.
Older compatible servers receive single-contact touch commands.

## Shortcuts (summary)

| Key | Action |
| --- | --- |
| Ctrl+L | Focus address |
| Enter / Escape | Navigate / restore address |
| Ctrl+T / Ctrl+W | New / close tab |
| Ctrl+F | Find |
| F11 | Fullscreen (also follows page fullscreen) |

Fullscreen hides the browser bar and expands the remote viewport.

## Tests

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
client/desktop/test-secure-session.sh
bash client/desktop/test-ui-input.sh
```

The session test requires presented frames, resize delivery, reconnect/input/
audio, zero decode errors, and recovery after a UI stall. Optional
`SURF_TEST_MIN_FPS=55` adds a hardware FPS gate.
