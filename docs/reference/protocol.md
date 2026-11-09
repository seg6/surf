# Protocol

One pinned WebSocket: authenticated control JSON + binary media. Go defines
events/commands/frames; C99 core implements the client side.

Truth in code: `backend/internal/protocol/commands.go`, `events.go`,
`protocol/fixtures/`. Both sides reject unknown commands/fields, wrong types,
invalid UTF-8, trailing JSON, and out-of-range values. Browser/CDP maps do not
pass through as untyped JSON.

## Versions

| Dimension | Meaning | Changes when |
| --- | --- | --- |
| App version | Surf release | A release is published |
| Compatibility generation | Gate between clients and backends | Older peers cannot interoperate safely |
| Wire range | Control and media envelopes | An envelope changes incompatibly |
| Capabilities | Optional protocol features | An additive feature is introduced |
| Core ABI | C library <-> host app | The C ABI changes incompatibly |

App version differences alone do not make peers incompatible. See
[Versioning](versioning.md) and [Capabilities](capabilities.md).

Generation 1 uses wire token `20260831-1`. Binary media uses the `RBR1`
header.

## Client commands

Each command is a JSON object with required field `t`. Optional causal fields:
`iid`, `clientNs`.

| `t` | Purpose |
| --- | --- |
| `browser-watch` | Subscribe to `browser-mode` events |
| `browser-resume` | Resume from setup (`revision`, optional `force`) |
| `size` | Report page surface (`w`, `h`) |
| `clock` | Clock sync sample (`c0`) |
| `tab` | Tab action (`action`, `id`) |
| `nav`, `opennew` | Navigate / open URL |
| `back`, `fwd`, `reload`, `stop` | Navigation chrome |
| `audio`, `mobile`, `dark`, `fullscreen` | Toggles (`on`) |
| `touch` | Touch snapshot / edges |
| `pointer`, `wheel` | Pointer input (`pointer-input` cap) |
| `key`, `paste`, `compose` | Keyboard / IME |
| `find` | Find in page |
| `suggest`, `history` | Omnibox / history query |
| `hist`, `bookmark`, `downloads` | Library fetches |
| `histdel`, `bmdel`, `dldel` | Delete history / bookmark / download |
| `clear` | Clear scoped data (`what`) |
| `dialogreply`, `selectreply` | JS dialog / `<select>` |
| `reader` | Reader mode |
| `reqkeyframe`, `video-retry` | Media recovery |
| `media-stats`, `media-volume` | Stats / volume |
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

`touch` carries UIKit-style contacts: lifecycle edges contain changed contacts;
move contains the full active set with stable contact IDs. Coordinates are
fractions of the presented video surface. The backend rejects stale
generations/sequences, maps through Chromium's CSS visual viewport, and
dispatches Chromium touch events. Navigation, tab changes, viewport changes,
website mode changes, and disconnects cancel active input.

**Mobile Websites** changes Chromium metrics and browser identity. It does not
turn touch into mouse input. Focus events from Chromium control the native
keyboard; plain text uses Chromium insertion and marked text uses IME
composition.

## Backend events

| `t` (typical) | Role |
| --- | --- |
| Hello / viewport | Initial or updated view size |
| `browser-mode` | Setup / streaming state machine |
| Tabs list | Open tabs |
| URL / history / security | Address bar state |
| Video / audio config | Encoder readiness and parameters |
| Library / suggest / history page / downloads | Library payloads |
| Dialog / file chooser / select / editable | Page UI prompts |
| Reader / media state | Extracted reader HTML; HTML5 media chrome |
| Clipboard / clipboard sync | Host clipboard bridge |
| Toast / text notices | Human-readable notices |
| Page frame / bool toggles | Frame ack and feature flags |

Exact field sets live in `events.go` and the fixtures. Prefer fixtures over
re-deriving shapes by hand.

## Browser setup

Capability `browser-setup`. Clients send `{"t":"browser-watch"}` after opening
the socket. Only subscribers receive `browser-mode`; older clients get a toast.

`browser-mode` fields: `state`, monotonic `revision`, `host`, `message`,
`standalone`, `canForce`. States: `starting`, `streaming`, `opening`, `setup`,
`resuming`, `failed`. Resume:

```json
{"t":"browser-resume","revision":7,"force":false}
```

Force is accepted only after a failed normal close, with a new revision and a
second confirmation. Remote clients cannot open setup on the host.

HTTP mirrors: authenticated `/api/v1/browser-mode`; admin
`/api/v1/admin/browser`. See [API](api.md) and
[Browser setup](../guide/browser-setup.md#lifetime).

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

Video: complete H.264 Annex B access units. Audio: signed little-endian PCM.
The parser validates header and exact payload length; the host retains the
buffer while decoding.

## Changing the contract

1. Classify additive vs breaking.
2. Update the Go protocol type.
3. Add valid and invalid fixtures.
4. Update the C codec.
5. Run contract tests both ways.
6. Add a capability for optional behavior.
7. Advance wire or compatibility generation only when older peers cannot continue safely.
