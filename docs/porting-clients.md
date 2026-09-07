# Porting Surf clients

A Surf client connects to a normal backend, pairs, authenticates, decodes live
media, sends input, and presents a native browser interface. Shared behavior
comes from `client/core`.

## Dependency direction

```text
client/ios --------+
                   +-> client/core
client/desktop ----+

backend <-> pinned HTTPS, WebSocket, and typed protocol <-> client host
```

The core has no platform, transport, media codec, storage, or threading
dependency. The host supplies those services.

## Control path

A host must:

1. Create `surf_core_t` with the current ABI and suitable limits.
2. Increment the connection generation for each authenticated session.
3. Decode WebSocket JSON with a host supplied workspace.
4. Dispatch the typed event with its connection generation.
5. Copy snapshot data before the next dispatch if the UI retains it.
6. Drain effects and perform them outside dispatch.
7. Return operation results as events.
8. Drop transient UI when its connection or page is no longer current.

The core tracks the identity and lifetime of dialogs, selects, collections, and
other remote state. Platform code stores the content needed to draw them.

## Media path

1. Validate each `RBR1` header before reading the payload.
2. Pass generation, sequence, and IDR metadata to the admission policy.
3. Keep only current encoded and decoded work.
4. Decode outside the control path.
5. Report presentation after the renderer shows the matching frame.
6. Keep audio on a separate bounded callback path.

UI work and file dialogs must not stop socket reads or decoding.

## Platform adapters

| Adapter | Contract |
| --- | --- |
| Discovery | DNS SD where available, plus manual address entry |
| Transport | HTTPS and WebSocket with the Surf certificate fingerprint |
| Identity | A separate RSA 2048 private key for each server |
| Persistence | Atomic server records with private permissions |
| Video | H.264 Annex B decode and stable YUV presentation |
| Audio | Signed 16 bit PCM through a bounded callback |
| Input | Pointer or touch, keys, paste, and IME converted to normalized samples |
| Services | Clipboard, files, downloads, sharing, lifecycle, and notifications |
| UI | Native layout, focus, input methods, and theme |

A host must keep certificate pinning, separate keys for unrelated servers, and
bounded media paths. Blocking platform calls stay outside media ingress.

## Bring up

1. Build and link the C99 core. Verify shared structure layouts.
2. Decode every event fixture and encode every command fixture.
3. Pair with a real backend, save the identity, authenticate, and reconnect.
4. Present live video with bounded queues and IDR recovery.
5. Verify pointer or touch, wheel, key, paste, and composition ordering.
6. Implement tabs, address entry, dialogs, selects, clipboard, files, search,
   settings, and diagnostics.
7. Run a stream while resizing, typing, pausing the UI, and restarting the
   backend.
8. Test the packaged program and its runtime dependencies.

Core contract tests run without a UI. Decoder, graphics, lifecycle, and package
acceptance remain platform tests.

## Support levels

| Level | Meaning | Current platforms |
| --- | --- | --- |
| Hardware verified | Tested package and session on named hardware | UIKit client on iOS 6.1.3 |
| Integration verified | Automated backend, renderer, input, restart, and package test | Linux desktop on X11 and x86_64 |
| Build supported | Builds without full runtime acceptance | Linux Wayland and other iOS package targets |
| Candidate | Design only | Android and old Windows |

The Linux client supports X11, Wayland, clipboard, scaled coordinates, and IME.
The full session test currently runs on X11.

## Android notes

A narrow JNI layer can expose the C ABI. Platform work includes RSA keys,
pinned TLS, H.264 through `MediaCodec`, audio, and a `Surface` or OpenGL ES
renderer. Compose or Views can handle UI. Old Android releases may need a
bundled TLS implementation.

## Old Windows notes

An initial port can reuse the Rust desktop host. The oldest intended target
must be tested against OpenGL, FFmpeg, and package dependencies before a
minimum version is stated.

A later Win32 host could use Schannel or WinHTTP, Media Foundation or a bundled
decoder, WASAPI, and native accessibility. Platform limits should be handled in
the adapter or through a negotiated capability, not by forking the protocol.
