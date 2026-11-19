# Surf desktop client

The Linux desktop client is built with Dear ImGui. Rust crates handle
transport, media, audio and the bindings to the C99 core that the iOS app also
uses.

The client is tested on X11 on x86_64. It also builds for Wayland, but the full
integration test doesn't cover it.

The client discovers servers on the local network, or you can enter an address
by hand. Pairing creates a separate device identity for each server and pins
that server's certificate. If only one server is saved, the client reconnects
to it automatically.

FFmpeg decodes H.264 into YUV420 planes for the OpenGL surface. PCM audio goes
through a separate bounded queue, and network, decode, audio and UI work each
run independently.

Servers with the `pointer-input` capability receive pointer, wheel, keyboard,
paste and IME input. Older compatible servers receive single-contact touch
input.

The interface has a 42-point browser bar, plus overlays for pairing, history,
bookmarks, downloads, reader mode, search, media, uploads, settings, server
management and diagnostics. The Linux build also includes window presets that
match common iPhone and iPad layout sizes.

The bar starts at the bottom, and Settings, then Appearance, moves it to the
top. Wide windows show horizontally scrollable tabs, and narrow windows use a
tab switcher. Ctrl+L selects the address. Enter navigates, and Escape restores
the current address. Ctrl+T opens a tab, Ctrl+W closes one, Ctrl+F opens Find
and F11 toggles fullscreen. Fullscreen also follows the page's own player
controls, hides the browser bar and expands the remote viewport. Escape exits
fullscreen, and Ctrl+L, Ctrl+T and Ctrl+F leave it before opening their
controls. Outside fullscreen, Find and Performance can be visible at the same
time.

Settings has the categories Appearance, Browsing, Computers, Device testing and
About. Wide windows show the categories as a navigation list, and narrow
windows use a selector. History is grouped by local calendar date. The menus on
Library rows hold the file and page actions and a confirmed removal. The New Tab
page has a search field you can edit directly.

Settings are saved next to the server identities in `desktop-preferences.json`.
Dark appearance, bar position, reduced motion, mobile websites and the selected
device preset persist across restarts. A device preset only resizes the window
when you press Apply. Display scaling follows the desktop's exact scale factor.

## Build

On Debian and Ubuntu, install these development packages first.

```sh
sudo apt-get install libasound2-dev libavcodec-dev libavformat-dev libavutil-dev libgl1-mesa-dev libxkbcommon-x11-0
```

The X11 client loads `libxkbcommon-x11` at runtime, so you need it even though
the build succeeds without it. The integration tests also need these tools:

```sh
sudo apt-get install ripgrep xauth xvfb libgl1-mesa-dri
```

The secure-session test needs working audio output. CI installs PulseAudio and
`libasound2-plugins`, creates a clocked null sink, and uses
`tests/alsa-headless.conf` to run real ALSA playback without a sound card.
Local runs use your existing audio device, and if no audio output is available
the audio probe fails immediately instead of timing out.

The session test always requires 600 presented frames within 40 seconds, resize
delivery, a successful reconnect, input and audio, zero decode errors, bounded
queues, and recovery after a deliberate UI stall. It reports FPS but doesn't
require a hardware benchmark on the shared software-rendered CI runners. To
also require at least 55 FPS on a controlled test machine, run
`SURF_TEST_MIN_FPS=55 client/desktop/test-secure-session.sh`.

To run the tests, the client and the packaging script:

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
client/desktop/test-secure-session.sh
bash client/desktop/test-ui-input.sh
client/desktop/package-linux.sh
```

The package script writes `dist/surf-desktop-<version>-linux-<arch>.tar.gz`.
The system you install it on needs compatible FFmpeg, ALSA, window system and
OpenGL runtime libraries.

To install an extracted package system wide:

```sh
sudo install -m 0755 bin/surf-client /usr/local/bin/surf-client
sudo install -m 0644 share/applications/space.seg6.surf.client.desktop \
  /usr/local/share/applications/space.seg6.surf.client.desktop
sudo install -m 0644 share/icons/hicolor/1024x1024/apps/space.seg6.surf.client.png \
  /usr/local/share/icons/hicolor/1024x1024/apps/space.seg6.surf.client.png
```

The secure session test pairs with a real backend, receives PCM, decodes live
video, exercises input and resizing, and checks recovery after a forced UI
stall.

The UI test uses Xvfb and XTest to send real keyboard and mouse events through
Winit and ImGui. It needs the dependencies above but no iPad. Presentation
diagnostics count a frame after a successful buffer swap, not after upload or on
every repaint, so they don't measure physical screen scanout.

## Visual checks

The opt-in gallery renders the production widgets with isolated fixture data
and no network worker. The scenes are `start`, `browser`, `new-tab`, `address`,
`settings`, `library`, `tools`, `tabs`, `find`, `performance`, `code`, `words`,
`reader`, `dialog`, `select`, `files`, `media` and `error`. There are also
stress fixtures named `browser-fullscreen`, `start-populated`,
`settings-testing`, `settings-browsing`, `settings-about`, `library-long`,
`library-empty` and `downloads`.

```sh
SURF_UI_GALLERY=settings SURF_UI_SIZE=375x667 \
  cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
```

`SURF_UI_CAPTURE=/absolute/path.png` captures the actual OpenGL framebuffer and
exits after the entrance transition and at least 20 frames. `SURF_UI_THEME=light`
and `SURF_UI_CHROME=top` select gallery variants. On X11,
`WINIT_X11_SCALE_FACTOR=1.25` exercises fractional scaling. Use a temporary
`SURF_CLIENT_HOME` when you experiment with persisted settings.

`SURF_UI_TRACE=1` also works in live sessions. It logs the UI state, the page
URL and title, and loading and first-video readiness, for opt-in automation.
The logs can contain browsing data, so use an isolated profile and don't enable
them for normal use. The `ui-input` example supports `move X Y [MS]` and
`scroll STEPS [MS]` as well as clicks, keyboard input and resizing, and it
sends real X11 events to the client.

For isolated touch demonstrations, `SURF_UI_TOUCH=1` maps page drags through the
existing touch path even when the server supports desktop pointer input.
`ui-input swipe X1 Y1 X2 Y2 [MS]` sends a paced press, move and release
sequence, and Chromium handles the resulting scroll and inertia. Without this
opt-in, normal mouse behavior and the separate mobile-site setting don't change.

Inter and Lucide are bundled, and their licenses are included in the package.
Favicons are only fetched from the verified Surf server, with bounded downloads,
image decoding off the UI thread and texture upload on the GL thread. SVG
favicons fall back to the tab title. Complex text shaping and full desktop
accessibility aren't implemented. The Linux UI test doesn't validate Windows,
macOS or legacy iOS.
