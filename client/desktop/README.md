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
other platform hosts.

On Debian/Ubuntu development hosts, install the native media headers first:

```sh
sudo apt-get install libavcodec-dev libavformat-dev libavutil-dev
```

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
client/desktop/test-secure-session.sh
```

When exactly one paired server is saved, the desktop client verifies and
reconnects to it automatically. The integration test builds an ordinary Surf
backend, performs real phrase-confirmed pairing, decodes a live browser frame,
and exits its headless OpenGL smoke app only after that frame reaches the GPU
presentation callback.
