# Surf Desktop Client

This workspace contains the real Rust/egui Surf client, initially certified on
Linux. It embeds the same portable C99 core used by the native iOS host.

The current checkpoint establishes safe C ownership and the styled OpenGL
application shell. Secure pairing, transport, media decode/presentation, and
complete browser interaction land as independently verified vertical slices.

```sh
cargo test --manifest-path client/desktop/Cargo.toml --workspace
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
```

The application uses egui's OpenGL backend so the streamed H.264 path can draw
reusable Y/U/V textures through a custom paint callback without converting
every decoded frame into a CPU RGBA image.
