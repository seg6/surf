# Client Protocol Contract

Surf carries authenticated control JSON and binary media over one pinned
WebSocket. The Go backend is currently the canonical encoder for events and
binary frames and the strict decoder for commands. The portable C99 client core
will provide the corresponding strict client implementation.

## Version dimensions

Surf keeps these dimensions independent:

| Dimension | Meaning | Changes when |
| --- | --- | --- |
| App version | User-facing Surf release | A release is cut |
| Compatibility generation | Currently deployed client/backend compatibility gate | Supporting the previous generation is impossible or unsafe |
| Wire range | Control/binary envelope generations understood by one component | An envelope interpretation breaks |
| Capability set | Optional additive protocol behavior | A backward-compatible feature is added |
| Core ABI | Host-to-C-library interface | The internal C ABI becomes incompatible |

An app-version difference alone never means two peers are incompatible.
Additive desktop input such as pointer hover or precise wheel scrolling belongs
in negotiated capabilities rather than forcing old touch clients to update.

The currently released config handshake continues to use `VERSION`,
`COMPATIBILITY_VERSION`, and the native wire marker while the portable range
and capability representation is introduced. Migration must remain compatible
with release clients throughout.

`pointer-input` is the first negotiated desktop-input capability. When present,
the client may send normalized `pointer` and `wheel` commands with a nonzero
surface generation and monotonically increasing sequence. Coordinates and
wheel deltas are normalized against the rendered page surface, so neither side
depends on a desktop DPI scale. Button transitions, key events, composition,
and paste share one backend input lane; replaceable pointer moves and wheel
samples may coalesce under overload without allowing text to overtake the click
that focused its target. A client that does not see the capability sends the
existing touch commands and omits the additive key modifier field.

## Control JSON

Client commands are defined by `backend/internal/protocol/commands.go`. The
backend rejects unknown commands, unknown fields, invalid field types, and
trailing JSON. Backend events are defined by
`backend/internal/protocol/events.go`; arbitrary CDP or browser maps may not be
sent directly to a native client.

The C client decoder will apply the same rules before application state sees an
event. Cross-language fixtures will cover every valid message and representative
invalid messages. Numeric widths, required fields, optional defaults, string
limits, and collection limits are part of the contract.

## Binary frame envelope

All current binary messages use an 84-byte network-byte-order `RBR1` header:

| Offset | Bytes | Field |
| ---: | ---: | --- |
| 0 | 4 | Magic `RBR1` |
| 4 | 1 | Type: 3 video, 4 audio |
| 5 | 1 | Flags; video bit 0 marks an IDR-containing AU |
| 6 | 2 | Header length, currently 84 |
| 8 | 4 | Access-unit/audio sequence |
| 12 | 4 | Source-frame sequence |
| 16 | 2 | Video width or audio sample rate |
| 18 | 2 | Video height or audio channel count |
| 20 | 4 | Payload length |
| 24 | 8 | Causal interaction ID |
| 32 | 8 | Backend source-receive monotonic nanoseconds |
| 40 | 8 | Encode-complete monotonic nanoseconds |
| 48 | 8 | Socket-write monotonic nanoseconds |
| 56 | 4 | Encoder generation |
| 60 | 4 | Reserved |
| 64 | 8 | Backend input-receive monotonic nanoseconds |
| 72 | 8 | CDP-dispatch-complete monotonic nanoseconds |
| 80 | 1 | Adaptive profile |
| 81 | 3 | Reserved |
| 84 | N | Payload |

Video payloads are complete H.264 Annex-B access units. Audio payloads are
signed little-endian PCM. A decoder must validate the complete envelope and
exact payload length before inspecting the payload.

The portable parser returns metadata plus a payload offset/view; it does not
copy or own the access unit. The platform host retains the WebSocket buffer for
as long as its decoder needs it.

## Contract change procedure

1. State whether the change is additive or breaking.
2. Add or update the Go protocol type.
3. Add canonical valid and invalid fixtures.
4. Update the C typed codec.
5. Run Go-to-C and C-to-Go contract verification.
6. Add a capability for optional behavior or advance the wire generation for
   a genuine breaking envelope change.
7. Update user-visible compatibility only if an installed peer truly cannot
   continue safely.
