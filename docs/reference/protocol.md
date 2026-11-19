# Protocol

Surf uses one pinned WebSocket that carries authenticated control JSON and
binary media. The Go backend defines the events, commands and frames, and the
C99 core implements the client side.

The source of truth is the code in `backend/internal/protocol/commands.go` and
`events.go`, together with `protocol/fixtures/`. Both sides reject unknown
commands and fields, wrong types, invalid UTF-8, trailing JSON and
out-of-range values. Browser and CDP maps are never passed through as untyped
JSON.

## Versions

| Dimension | Meaning | Changes when |
| --- | --- | --- |
| App version | Surf release | A release is published |
| Compatibility generation | Gate between clients and backends | Older peers cannot interoperate safely |
| Wire range | Control and media envelopes | An envelope changes incompatibly |
| Capabilities | Optional protocol features | An additive feature is introduced |
| Core ABI | Interface between the C library and the host app | The C ABI changes incompatibly |

Differences in app version alone don't make peers incompatible. See
[Versioning](versioning.md) and [Capabilities](capabilities.md).

Generation 1 uses the wire token `20260831-1`. Binary media uses the `RBR1`
header.

## Client commands

Each command is a JSON object with a required field `t`. The optional causal
fields are `iid` and `clientNs`.

| `t` | Purpose |
| --- | --- |
| `browser-watch` | Subscribe to `browser-mode` events |
| `browser-resume` | Resume from setup (`revision`, optional `force`) |
| `size` | Report page surface (`w`, `h`) |
| `clock` | Clock sync sample (`c0`) |
| `tab` | Tab action (`action`, `id`) |
| `nav`, `opennew` | Navigate, or open a URL in a new tab |
| `back`, `fwd`, `reload`, `stop` | Navigation chrome |
| `audio`, `mobile`, `dark`, `fullscreen` | Toggles (`on`) |
| `video` | Pause or resume only this client's video lane (`on`, `video-pause` cap) |
| `touch` | Touch snapshot and edges |
| `pointer`, `wheel` | Pointer input (`pointer-input` cap) |
| `key`, `paste`, `compose` | Keyboard and IME |
| `find` | Find in page |
| `suggest`, `history` | Omnibox and history queries |
| `hist`, `bookmark`, `downloads` | Library fetches |
| `histdel`, `bmdel`, `dldel` | Delete a history entry, bookmark or download |
| `clear` | Clear scoped data (`what`) |
| `dialogreply`, `selectreply` | Replies to a JS dialog or a `<select>` |
| `reader` | Reader mode |
| `reqkeyframe`, `video-retry` | Media recovery |
| `media-stats`, `media-volume` | Stats and volume |
| `media-playpause`, `media-mute`, `media-query` | In-page media chrome |
| `clipboard-result`, `clipboard-change` | Clipboard bridge |
| `log-record`, `log-cleared` | Live client logs |

### Pointer input

`pointer-input` enables normalized pointer and wheel commands. Each includes a
nonzero surface generation and an increasing sequence. Coordinates are relative
to the rendered page surface. Moves and adjacent wheel samples may coalesce;
button changes, keys, composition, and paste stay ordered. Clients without the
capability use `touch` and omit key modifiers.

### Touch input

`touch` carries UIKit-style contacts. Lifecycle edges contain the changed
contacts, and a move contains the full active set with stable contact IDs.
Coordinates are fractions of the presented video surface. The backend rejects
stale generations and sequences, maps coordinates through Chromium's CSS visual
viewport, and dispatches Chromium touch events. Navigation, tab changes, viewport changes,
website mode changes, and disconnects cancel active input.

The Mobile Websites setting changes Chromium metrics and browser identity. It
doesn't turn touch into mouse input. Focus events from Chromium control the native
keyboard; plain text uses Chromium insertion and marked text uses IME
composition.

## Backend events

| `t` (typical) | Role |
| --- | --- |
| Hello, viewport | Initial or updated view size |
| `browser-mode` | Setup / streaming state machine |
| Tabs list | Open tabs |
| URL, history, security | Address bar state |
| Video and audio config | Encoder readiness and parameters |
| Library, suggest, history page, downloads | Library payloads |
| Dialog, file chooser, select, editable | Page UI prompts |
| Reader, media state | Extracted reader HTML and HTML5 media chrome |
| Clipboard, clipboard sync | Host clipboard bridge |
| Toast, text notices | Human-readable notices |
| Page frame, bool toggles | Frame ack and feature flags |

The exact field sets are in `events.go` and the fixtures. Use the fixtures
instead of deriving the shapes by hand.

## Browser setup

This is the `browser-setup` capability. Clients send `{"t":"browser-watch"}`
after opening the socket. Only subscribers receive `browser-mode`, and older
clients get a toast instead.

The `browser-mode` event has the fields `state`, a monotonic `revision`, `host`,
`message`, `standalone` and `canForce`. The states are `starting`, `streaming`,
`opening`, `setup`, `resuming` and `failed`. To resume, a client sends:

```json
{"t":"browser-resume","revision":7,"force":false}
```

Force is accepted only after a failed normal close, with a new revision and a
second confirmation. Remote clients can't open setup on the host.

The same operations are available over HTTP at the authenticated
`/api/v1/browser-mode` and the admin `/api/v1/admin/browser`. See [API](api.md) and
[Browser setup](../guide/browser-setup.md).

## Media envelope

Binary messages start with an 84-byte `RBR1` header (network byte order).

| Offset | Bytes | Field |
| ---: | ---: | --- |
| 0 | 4 | Magic `RBR1` |
| 4 | 1 | Type: 3 video, 4 audio |
| 5 | 1 | Flags; video bit 0 = IDR |
| 6 | 2 | Header length (84) |
| 8 | 4 | Access unit or audio sequence |
| 12 | 4 | Source frame sequence |
| 16 | 2 | Video width or audio sample rate |
| 18 | 2 | Video height or audio channel count |
| 20 | 4 | Payload length |
| 24 | 8 | Causal interaction ID |
| 32 | 8 | Backend source receive time (monotonic ns) |
| 40 | 8 | Encode completion time |
| 48 | 8 | Socket write time |
| 56 | 4 | Encoder generation |
| 60 | 4 | Reserved |
| 64 | 8 | Backend input receive time |
| 72 | 8 | CDP dispatch completion time |
| 80 | 1 | Adaptive profile |
| 81 | 3 | Reserved |
| 84 | N | Payload |

Video payloads are complete H.264 Annex B access units, and audio payloads are
signed 16-bit little-endian PCM, interleaved when there are two channels, in
20 ms chunks. The sample rate and channel count are in the header and in the
`audio-config` event: 48 kHz stereo by default, or 16 kHz mono when the host
sets `SURF_AUDIO_QUALITY=standard`. The parser validates the header and the exact payload
length, and the host keeps the buffer while decoding.

## Changing the contract

A protocol change goes through these steps:

1. Decide whether the change is additive or breaking.
2. Update the Go protocol type.
3. Add valid and invalid fixtures.
4. Update the C codec.
5. Run contract tests both ways.
6. Add a capability for optional behavior.
7. Advance the wire or compatibility generation only when older peers cannot
   continue safely.
