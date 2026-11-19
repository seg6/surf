# Porting Surf clients

A client has to pair, authenticate, decode media, send input and draw native
chrome. The shared behavior lives in `client/core`, which has no platform,
transport, codec, storage or threading code. The host application supplies
those.

```text
client/ios --------+
                   +-> client/core
client/desktop ----+

backend <-> pinned HTTPS / WebSocket / typed protocol <-> client host
```

## Control path

A host has to do the following:

1. Create `surf_core_t` with the current ABI and suitable limits.
2. Increment the connection generation for each authenticated session.
3. Decode WebSocket JSON with a host supplied workspace.
4. Dispatch the typed event with its connection generation.
5. Copy snapshot data before the next dispatch if the UI retains it.
6. Drain effects and perform them outside dispatch.
7. Return operation results as events.
8. Drop transient UI when its connection or page is no longer current.

The core tracks the identity and lifetime of dialogs, selects, collections and
other remote state. Platform code stores the content it needs to draw them.

## Media path

1. Validate each `RBR1` header before reading the payload.
2. Pass generation, sequence and IDR metadata to the admission policy.
3. Keep only current encoded and decoded work.
4. Decode outside the control path.
5. Report presentation after the renderer shows the matching frame.
6. Keep audio on a separate bounded callback path.

UI work and file dialogs must never stop socket reads or decoding.

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

A host has to keep certificate pinning, use separate keys for unrelated
servers, and keep its media paths bounded. Blocking platform calls should stay
out of media ingress.

## Bring up

1. Build and link the C99 core, and verify the shared structure layouts.
2. Decode every event fixture and encode every command fixture.
3. Pair with a real backend, save the identity, authenticate and reconnect.
4. Present live video with bounded queues and IDR recovery.
5. Verify the ordering of pointer or touch, wheel, key, paste and composition
   input.
6. Implement tabs, address entry, dialogs, selects, clipboard, files, search,
   settings and diagnostics.
7. Run a stream while resizing, typing, pausing the UI and restarting the
   backend.
8. Test the packaged program and its runtime dependencies.

The core contract tests run without a UI. Decoder, graphics, lifecycle and
package acceptance are still platform tests.

## Support levels

| Level | Meaning | Current platforms |
| --- | --- | --- |
| Hardware verified | Tested package and session on named hardware | UIKit client on iOS 6.1.3 |
| Integration verified | Automated backend, renderer, input, restart, and package test | Linux desktop on X11 and x86_64 |
| Build supported | Builds without full runtime acceptance | Linux Wayland and other iOS package targets |
| Candidate | Design only | Android and old Windows |

Community ports live outside this repo for now, such as the
[PS Vita client](https://github.com/jmverges/surf-vita).

## Android notes

A narrow JNI layer can expose the C ABI. The platform work is RSA keys, pinned
TLS, H.264 through `MediaCodec`, audio, and a `Surface` or OpenGL ES renderer.
Compose or Views can handle the UI. Old Android releases may need a bundled TLS
implementation.

## Old Windows notes

A first port could reuse the Rust desktop host. Before stating a minimum
version, test the oldest intended target against OpenGL, FFmpeg and the package
dependencies.

A later Win32 host could use Schannel or WinHTTP, Media Foundation or a bundled
decoder, WASAPI, and native accessibility. If a platform has limits, handle
them in the adapter or through a negotiated capability instead of forking the
protocol.
