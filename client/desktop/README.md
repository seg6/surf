# Surf Desktop Client

This workspace contains the real Rust/egui Surf client, initially certified on
Linux. It embeds the same portable C99 core used by the native iOS host.

The current checkpoint is a genuine secure client rather than a replay shell.
It discovers nearby servers (with manual entry as a fallback), persists a
per-server device identity, completes phrase-confirmed pairing, pins the exact
server certificate, authenticates, and opens the production WebSocket. Control
and media ingress are bounded independently, and reconnect backoff is decided
by the same C99 core policy available to other platform hosts.

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
client/desktop/test-secure-session.sh
```

The application currently validates and counts live H.264 frames. The next
checkpoint adds the dedicated FFmpeg worker and reusable OpenGL Y/U/V textures,
without converting every decoded frame into a CPU RGBA image or routing media
through the egui control queue.
