# Client protocol

Surf uses one pinned WebSocket for authenticated control JSON and binary media.
The Go backend defines events, commands, and media frames. The C99 client core
implements the client side of that contract.

## Versions

| Dimension | Meaning | Changes when |
| --- | --- | --- |
| App version | Surf release | A release is published |
| Compatibility generation | Gate between installed clients and backends | Older peers cannot interoperate safely |
| Wire range | Understood control and media envelopes | An envelope changes incompatibly |
| Capabilities | Optional protocol features | An additive feature is introduced |
| Core ABI | Interface between the C library and a host app | The C ABI changes incompatibly |

A difference in app versions does not make peers incompatible. Optional features
use capability negotiation.

The current handshake uses `VERSION`, `COMPATIBILITY_VERSION`, and the native
wire marker. See [Versioning](versioning.md) for release rules.

## Pointer input

`pointer-input` enables normalized pointer and wheel commands. Each command
includes a nonzero surface generation and an increasing sequence number.
Coordinates and wheel values are relative to the rendered page surface.

Pointer moves and adjacent wheel samples may replace older queued samples.
Button changes, keyboard input, composition, and paste remain ordered. Clients
without the capability use touch commands and omit key modifiers.

## Control JSON

Client commands are defined in
`backend/internal/protocol/commands.go`. Backend events are defined in
`backend/internal/protocol/events.go`.

Both sides reject unknown commands, unknown fields, wrong field types, invalid
UTF 8, trailing JSON, and values outside the documented limits. Cross language
fixtures cover valid and invalid messages. Browser or CDP maps do not pass
through to clients as untyped JSON.

## Browser setup

The additive `browser-setup` capability enables a paused browser lifetime on an
otherwise live connection. A supporting client sends `{"t":"browser-watch"}`
after opening its socket. Only subscribed clients receive `browser-mode` events;
older clients receive a plain toast instead of an unknown event.

`browser-mode` includes `state`, monotonic `revision`, `host`, `message`,
`standalone`, and `canForce`. States are `starting`, `streaming`, `opening`,
`setup`, `resuming`, and `failed`. Only `streaming` accepts page input. Clients
clear page prompts, focus, keyboard, fullscreen and buffered media while paused.
They re-send their viewport and website preferences and start fresh media when
streaming returns. Video generations remain unique across browser replacements.

After user confirmation, a paired client can send
`{"t":"browser-resume","revision":7,"force":false}`. A force request is
accepted only after a failed normal close, with its new revision and a second
confirmation. Remote clients cannot open a browser on the host. The authenticated
`/api/v1/browser-mode` API offers GET status and POST resume for the same role.
Local administration uses `/api/v1/admin/browser` for GET status and POST
`{action, revision, force}` with `action` equal to `open` or `resume`.

The manager serializes ownership transitions. Stale, concurrent and unauthorized
requests fail without changing the current browser. No compatibility-generation
bump is needed for this optional feature.

## Media envelope

Binary messages start with an 84 byte `RBR1` header in network byte order.

| Offset | Bytes | Field |
| ---: | ---: | --- |
| 0 | 4 | Magic `RBR1` |
| 4 | 1 | Type, 3 for video and 4 for audio |
| 5 | 1 | Flags, with video bit 0 marking an IDR access unit |
| 6 | 2 | Header length, currently 84 |
| 8 | 4 | Access unit or audio sequence |
| 12 | 4 | Source frame sequence |
| 16 | 2 | Video width or audio sample rate |
| 18 | 2 | Video height or audio channel count |
| 20 | 4 | Payload length |
| 24 | 8 | Causal interaction ID |
| 32 | 8 | Backend source receive time in monotonic nanoseconds |
| 40 | 8 | Encode completion time in monotonic nanoseconds |
| 48 | 8 | Socket write time in monotonic nanoseconds |
| 56 | 4 | Encoder generation |
| 60 | 4 | Reserved |
| 64 | 8 | Backend input receive time in monotonic nanoseconds |
| 72 | 8 | CDP dispatch completion time in monotonic nanoseconds |
| 80 | 1 | Adaptive profile |
| 81 | 3 | Reserved |
| 84 | N | Payload |

Video payloads are complete H.264 Annex B access units. Audio payloads are
signed little endian PCM.

The parser validates the header and exact payload length before returning
metadata and a borrowed payload view. The host retains the WebSocket buffer
while the decoder uses it.

## Changing the contract

1. Classify the change as additive or breaking.
2. Update the Go protocol type.
3. Add valid and invalid fixtures.
4. Update the C codec.
5. Run contract tests in both directions.
6. Add a capability for optional behavior.
7. Advance the wire or compatibility generation only when older peers cannot
   continue safely.
