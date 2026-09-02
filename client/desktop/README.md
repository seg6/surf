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

The interface has one compact browser bar and overlays for pairing, history,
bookmarks, downloads, reader mode, search, media, uploads, settings, server
management, and diagnostics. Linux builds also include window presets matching
common iPhone and iPad layout sizes.

## Build

Debian and Ubuntu hosts need these development packages.

```sh
sudo apt-get install libasound2-dev libavcodec-dev libavformat-dev libavutil-dev libgl1-mesa-dev
```

Run the tests and client with:

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
client/desktop/test-secure-session.sh
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
