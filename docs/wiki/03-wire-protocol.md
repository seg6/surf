# Wire Protocol

The client/server protocol has two layers:

- JSON text messages for control and browser state;
- RBR1 binary messages for encoded visual frames.

The same WebSocket carries both. Message ordering on the socket is preserved by one writer goroutine per client.

## Handshake

### Web client

```text
GET /                         authenticated cookie required
  -> HTML contains injected WS token and ClientVersion

GET /ws?k=<token>&v=<ClientVersion>
  -> WebSocket upgrade
```

### Native client

```text
POST /login                   password form; receives auth cookie
GET /native-config            cookie required
  -> {token,vw,vh,nv,host}
GET /ws?k=<token>&nv=<NativeVersion>
  -> WebSocket upgrade tagged as native
```

The server accepts a connection only when the token matches and either the web or native version matches exactly. A stale cached client therefore cannot silently speak incompatible semantics.

## RBR1 header

All integer fields are big-endian.

```text
offset  size  field
0       4     magic        ASCII "RBR1"
4       1     type
5       1     flags
6       2     headerLen    currently 32
8       4     sequence
12      2     reservedX
14      2     reservedY
16      2     width
18      2     height
20      4     payloadLen
24      4     scrollX
28      4     scrollY
32      ...   payload
```

Clients must locate the payload using `headerLen`, not a hard-coded offset. The current minimum is 32 bytes, but length-driven parsing preserves forward compatibility.

Width and height are encoded/coded pixel dimensions, not necessarily the server's CSS viewport. During web-only motion, a JPEG may be downscaled while still representing the full viewport.

### Frame types

| Type | Meaning | Payload | Delivery/backpressure |
|---|---|---|---|
| 1 | Full viewport JPEG | complete JPEG | web + native; three-frame ack window |
| 2 | Reserved region JPEG | not implemented | must not be emitted |
| 3 | H.264 access unit | complete Annex-B AU | native video mode only; IDR-resync |
| 4 | Historical audio reservation | not implemented | do not emit |

For type 3, flags bit 0 marks an IDR access unit. Scroll fields are zero. Width/height are the encoder's coded dimensions.

## Type-1 acknowledgement contract

A client sends:

```json
{"t":"ready"}
```

for every type-1 frame it receives. This includes:

- a frame decoded and rendered;
- a frame replaced by a newer pending frame;
- a frame discarded as stale;
- a malformed JPEG payload after the RBR1 envelope was accepted.

The acknowledgement is not a request for the next screenshot. It releases one slot in that client's bounded delivery window. Breaking the one-received/one-ready relationship eventually freezes the JPEG lane.

Type-3 H.264 frames are not acknowledged.

## Coordinate semantics

All position and delta fields in client control messages are fractions of the current viewport:

```text
0.0 <= x,y,dx,dy,cx,cy <= approximately 1.0
```

`dx`/`dy` may be signed for wheel motion. The clients negate finger movement so the resulting wheel delta follows page-scroll direction.

Do not add pixel-coordinate messages. Fractional coordinates are what keep input stable across:

- half/full-resolution JPEG switches;
- orientation and viewport changes;
- zoom/device-scale changes;
- native video versus JPEG mode.

## Client-to-server messages

The Go `protocol.ClientMessage` struct is a field union. `t` selects which fields are meaningful.

### Transport and lifecycle

| `t` | Fields | Meaning |
|---|---|---|
| `ready` | — | acknowledge one received type-1 frame |
| `poke` | — | reset JPEG window and request a fresh frame |
| `size` | `w`, `h` | set shared viewport; clamped to 320–1600 |
| `video` | `on` | native client enters/leaves H.264 mode |

### Navigation and tabs

| `t` | Fields | Meaning |
|---|---|---|
| `nav` | `url` | navigate after server-side normalization |
| `reload` | — | reload active page |
| `stop` | — | stop loading |
| `back` | — | previous navigation-history entry |
| `fwd` | — | next navigation-history entry |
| `tab` | `action`, `id` | `select`, `close`, or `new` |

### Pointer, gesture, and zoom

| `t` | Fields | Meaning |
|---|---|---|
| `click` | `x`, `y` | complete left click |
| `wheel` | `x`, `y`, `dx`, `dy` | wheel at a fixed gesture anchor |
| `lpdown` | `x`, `y` | begin held left-button press |
| `lpmove` | `x`, `y` | drag with left button held |
| `lpup` | `x`, `y`, `sel` | release; `sel` requests word selection |
| `zoom` | `scale`, `cx`, `cy` | absolute zoom and gesture focus |

### Keyboard and text

| `t` | Fields | Meaning |
|---|---|---|
| `key` | `text` | insert one printable character |
| `key` | `down`, `key`, `code`, `keyCode` | special-key down/up |
| `paste` | `text` | burst insertion through CDP `Input.insertText` |

### Features

| `t` | Fields | Meaning |
|---|---|---|
| `find` | `q`, `dir` | find next/previous in page |
| `suggest` | `q` | request history/bookmark suggestions |
| `hist` | — | request recent history and bookmarks |
| `bookmark` | — | toggle current URL |
| `downloads` | — | request downloadable-file list |

Unknown message types are ignored after the core switch reaches the feature handler.

## Server-to-client messages

### Initial and navigation state

```json
{"t":"hello","vw":1024,"vh":768}
{"t":"tabs","tabs":[{"id":1,"title":"...","url":"...","active":true,"icon":"/tabicon/1?v=..."}]}
{"t":"url","url":"https://...","starred":false}
{"t":"histstate","back":true,"fwd":false}
{"t":"loading","on":true}
```

`hello` is also a resynchronization signal. Clients should compare viewport state and resend `size` when needed.

### Editing and interaction

```json
{"t":"editable","on":true,"kind":"email","rect":[0.1,0.2,0.6,0.08]}
{"t":"copytext","text":"selected text"}
{"t":"zoom","scale":1.5}
{"t":"found","on":true}
```

The `editable` extension is additive. Clients must tolerate unknown fields so the web client can consume only `on` while the native client uses keyboard kind and bounds.

### Data and UI notifications

```json
{"t":"suggest","items":[...]}
{"t":"hist","hist":[...],"bookmarks":[...],"starred":true}
{"t":"starred","on":true}
{"t":"downloads","items":[{"name":"file.pdf","size":1234,"ts":...}]}
{"t":"download","name":"file.pdf"}
{"t":"toast","text":"bookmarked"}
```

### Video control

```json
{"t":"video-config","ok":true,"fps":24,"w":1024,"h":768}
```

`ok:true` is sent only after the first access unit proves the encoder lane is alive. `ok:false` tells the native client to remain on or fall back to JPEG.

## Versioning rules

### Bump `ClientVersion` when

- the embedded web client consumes changed fields or semantics;
- type-1 header/payload semantics change;
- web JSON message names or required fields change;
- a stale cached web asset could behave incorrectly with the new server.

### Bump `NativeVersion` when

- the native app consumes a changed or new message;
- native frame types or flags change;
- native handshake/config behavior changes;
- H.264 payload/config semantics change.

The native version constant in the Go server and the version compiled into the app must remain identical.

### Compatibility rules

1. Additive JSON fields are preferred.
2. Unknown JSON fields and message types must be tolerated by clients.
3. Never reuse an RBR1 frame type for new semantics.
4. Never send native-only frame types to web clients.
5. Preserve header-length parsing.
6. Coordinate units must remain fractions.
7. If type-1 delivery semantics change, update both clients and tests together.

## Example type-1 envelope

A 512×384 JPEG motion frame with 1234 bytes of payload and page scroll `(0,768)` is conceptually:

```text
52 42 52 31        RBR1
01                 type=JPEG
00                 flags
00 20              headerLen=32
00 00 00 2A        sequence=42
00 00 00 00        reserved
02 00 01 80        width=512, height=384
00 00 04 D2        payloadLen=1234
00 00 00 00        scrollX=0
00 00 03 00        scrollY=768
... JPEG bytes ...
```

## Example video recovery

Suppose a native client receives:

```text
IDR 100, P 101, P 102, [socket drop], P 104, P 105, IDR 106, P 107
```

After the drop, frames 104 and 105 are not useful because their reference chain is incomplete. The server marks that subscription for resync and resumes delivery at IDR 106. Repeated SPS/PPS in the IDR makes it independently decodable.

## Protocol change checklist

Before merging a protocol change:

- identify web, native, or shared audience;
- update the appropriate version constant(s);
- update `internal/protocol` and browser handlers;
- update `public/app.js` and/or native parser/emitter;
- update Go protocol tests and probe fixtures;
- verify web type-1 ready accounting;
- verify native type-3 IDR recovery if relevant;
- update this page and the detailed native protocol doc.