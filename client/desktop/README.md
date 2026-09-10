# Surf desktop client

The Linux development client uses Dear ImGui. Rust crates handle transport,
media, audio, and bindings to the same C99 core used by the iOS app.

The current test target is X11 on x86_64. Wayland builds but is not covered by
the full integration test.

The client discovers local servers or accepts a manual address. Pairing creates
a separate device identity for each server and pins its certificate. Saved
servers reconnect automatically when only one is configured.

FFmpeg decodes H.264 into YUV420 planes for the OpenGL surface. PCM audio uses a
separate bounded queue. Network, decode, audio, and UI work run independently.

Servers with the `pointer-input` capability receive pointer, wheel, keyboard,
paste, and IME input. Older compatible servers receive single contact touch
input.

The interface has a 42-point browser bar and overlays for pairing, history,
bookmarks, downloads, reader mode, search, media, uploads, settings, server
management, and diagnostics. Linux builds also include window presets matching
common iPhone and iPad layout sizes.

The bar starts at the bottom; Settings → Appearance moves it to the top.
Wide windows show horizontally scrollable tabs. Narrow windows use a tab
switcher. Ctrl+L selects the address; Enter navigates, Escape restores the current
address. Ctrl+T opens a tab, Ctrl+W closes one, Ctrl+F opens Find, and F11 toggles
fullscreen. Fullscreen also follows the page's own player controls, hides the
browser bar and expands the remote viewport. Escape exits; Ctrl+L, Ctrl+T and
Ctrl+F leave fullscreen before opening their controls. Find and Performance can
remain visible together outside fullscreen.

Settings separates Appearance, Browsing, Computers, Device testing, and About.
Wide windows use category navigation; narrow windows use a category selector.
History is grouped by local calendar date. Library row menus contain file/page
actions and confirmed removal. New Tab includes a directly editable search field.

Settings are saved beside the server identities in `desktop-preferences.json`.
Dark appearance, bar position, reduced motion, mobile websites and the selected
device preset survive restarts. A device preset only resizes the window when
Apply is pressed. Display scaling follows the desktop's exact scale factor.

## Build

Debian and Ubuntu hosts need these development packages.

```sh
sudo apt-get install libasound2-dev libavcodec-dev libavformat-dev libavutil-dev libgl1-mesa-dev libxkbcommon-x11-0
```

The X11 client loads `libxkbcommon-x11` at runtime, so it is needed even when
the build succeeds. The integration tests also need these tools:

```sh
sudo apt-get install ripgrep xauth xvfb libgl1-mesa-dri
```

The secure-session test requires working audio output. CI installs PulseAudio
and `libasound2-plugins`, creates a clocked null sink, and uses
`tests/alsa-headless.conf` to exercise real ALSA playback without a sound card.
Normal local runs use your existing audio device. An unavailable audio output
fails the audio probe immediately instead of timing out.

The session test always requires 600 presented frames within 40 seconds, resize
delivery, successful reconnect/input/audio, zero decode errors, bounded queues,
and recovery after a deliberate UI stall. It reports FPS but does not impose a
hardware benchmark on shared software-rendered CI runners. To also require
at least 55 FPS on a controlled test machine, run
`SURF_TEST_MIN_FPS=55 client/desktop/test-secure-session.sh`.

Run the tests and client with:

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
client/desktop/test-secure-session.sh
bash client/desktop/test-ui-input.sh
client/desktop/package-linux.sh
```

The package script writes
`dist/surf-desktop-<version>-linux-<arch>.tar.gz`. The target system needs
compatible FFmpeg, ALSA, window system, and OpenGL runtime libraries.

Install an extracted package system wide with:

```sh
sudo install -m 0755 bin/surf-client /usr/local/bin/surf-client
sudo install -m 0644 share/applications/space.seg6.surf.client.desktop \
  /usr/local/share/applications/space.seg6.surf.client.desktop
sudo install -m 0644 share/icons/hicolor/1024x1024/apps/space.seg6.surf.client.png \
  /usr/local/share/icons/hicolor/1024x1024/apps/space.seg6.surf.client.png
```

The secure session test pairs with a real backend, receives PCM, decodes live
video, exercises input and resize, and checks recovery after a forced UI stall.

The UI test uses Xvfb and XTest to send real keyboard and mouse events through
Winit and ImGui. It needs the dependencies above, not an iPad.
Presentation diagnostics count a frame after successful buffer swap, not after
upload or on every repaint; they do not claim to measure physical screen scanout.

## Visual checks

The opt-in gallery uses production widgets and isolated fixture data, without a
network worker. Scenes include `start`, `browser`, `new-tab`, `address`,
`settings`, `library`, `tools`, `tabs`, `find`, `performance`, `code`, `words`,
`reader`, `dialog`, `select`, `files`, `media`, and `error`.
Additional stress fixtures are `browser-fullscreen`, `start-populated`, `settings-testing`,
`settings-browsing`, `settings-about`, `library-long`, `library-empty`, and `downloads`.

```sh
SURF_UI_GALLERY=settings SURF_UI_SIZE=375x667 \
  cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
```

`SURF_UI_CAPTURE=/absolute/path.png` captures the actual OpenGL framebuffer and
exits after the entrance transition and at least 20 frames. `SURF_UI_THEME=light`
and `SURF_UI_CHROME=top` select gallery
variants. On X11, `WINIT_X11_SCALE_FACTOR=1.25` also exercises fractional scaling.
Use a temporary `SURF_CLIENT_HOME` when experimenting with persisted settings.

`SURF_UI_TRACE=1` also works in live sessions. It logs UI state, page URL/title,
loading and first-video readiness for opt-in automation. These logs can contain
browsing data: use an isolated profile and do not enable them for normal use.
The `ui-input` example supports `move X Y [MS]` and `scroll STEPS [MS]` as well
as clicks, keyboard input and resizing; it sends real X11 events to the client.
For isolated touch demonstrations, `SURF_UI_TOUCH=1` maps page drags through
the existing touch path even when the server supports desktop pointer input.
`ui-input swipe X1 Y1 X2 Y2 [MS]` emits a paced press/move/release sequence;
Chromium handles the resulting scroll and inertia. Normal mouse behavior and
the separate mobile-site setting are unchanged when this opt-in is absent.

Inter and Lucide are bundled, with their licenses included in the package.
Favicons are fetched only from the verified Surf server, with bounded downloads,
off-thread image decoding and GL-thread texture upload. SVG favicons fall back
to the tab title. Complex text shaping and full desktop accessibility are not
implemented. Windows, macOS and legacy iOS are not validated by the Linux UI test.
