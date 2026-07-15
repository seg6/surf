# 02 — Protocol: RBR1 and its native extensions

Ground truth lives in `internal/protocol/protocol.go`, `internal/browser/*.go`, and `public/app.js`. This doc pins the byte layouts and the *additions*; when in doubt, read the code, then fix this doc.

## 1. Handshake & auth (native flow)

The web flow injects a token into the served page. The native app can't scrape HTML, so it gets one new endpoint:

```
POST /login                    form: password=...      → Set-Cookie: <auth cookie> (existing)
GET  /native-config            (auth cookie required)  → 200 JSON:
     { "token": "<wstoken>", "vw": 1024, "vh": 768,
       "nv": "<NativeVersion>", "host": "wrp.seg6.space" }
GET  /ws?k=<token>&nv=<NativeVersion>                  → WebSocket upgrade
```

Server changes (docs/03 §1): add `config.NativeVersion` (date-stamped like `ClientVersion`); `handleWS` accepts `v == ClientVersion` **or** `nv == NativeVersion`; the hub tags the client `native: true` when it arrived via `nv` — frame types ≥2 are only ever sent to native-tagged clients.

The app persists: server base URL, auth cookie, last-known token (NSUserDefaults; the cookie is HMAC-signed and ~180-day, matching web). On WS 403: re-fetch `/native-config`; if that 401s/redirects, show the login screen. Rejected version = the app is stale → show "update the app" with both version strings.

## 2. RBR1 binary frame header (32 bytes, big-endian) — unchanged layout

```
offset  size  field
0       4     magic          "RBR1"
4       1     type           see table below
5       1     flags          reserved, 0
6       2     hdrlen         currently 32 — ALWAYS parse payload from hdrlen, never assume 32
8       4     seq            uint32, monotonic per connection
12      2     rectX          reserved (0) for types 1; region x for type 2; see per-type notes
14      2     rectY          same
16      2     w              frame/region width in pixels
18      2     h              frame/region height in pixels
20      4     payloadLen     bytes after the header
24      4     scrollX        page scroll offset at capture, remote CSS px (types 1–2)
28      4     scrollY        same
```

### Frame types

| type | name | payload | audience | notes |
|---|---|---|---|---|
| 1 | Full JPEG | JPEG, whole viewport | web + native | Existing. `w/h` are *encoded pixel* dims (may be half-res during motion — display scaled to fit; never trust them to equal viewport CSS size) |
| 2 | Region JPEG | JPEG of a sub-rect | native only | **Reserved, not implemented yet.** `rectX/rectY` (viewport CSS px) + `w/h` = paste position/size. Future typing-crispness lane; defined now so headers never need renegotiation |
| 3 | H.264 access unit | one complete AU, Annex-B byte stream (start codes intact) | native, video mode | `seq` = AU index. flags bit0 = 1 if AU contains an IDR. `rectX/rectY/scrollX/scrollY` = 0. `w/h` = coded size (constant per encoder run; changes only after a `video` restart) |
| 4 | Audio chunk | PCM s16le mono (phase 3b; sample rate in the `video-config` JSON below) | native, audio on | `seq` = chunk index; ~50–100ms per chunk |

**Type-1 ack discipline (unchanged, critical):** the server pipelines up to 3 unacked type-1 frames. The client sends `{"t":"ready"}` for **every received type-1 frame — including ones it drops/coalesces**. Types 3–4 are *not* acked (video/audio flow control is IDR-drop based, docs/03 §3; audio is fire-and-forget with client-side jitter buffer).

## 3. JSON control messages

### Client → server (existing — the native app emits exactly what `public/app.js` emits)

All coordinates are **fractions of the viewport (0..1)**. Full field union: `protocol.ClientMessage`.

| t | fields | notes |
|---|---|---|
| ready | — | type-1 frame ack (§2) |
| poke | — | watchdog: no frame for a while → ask for one |
| size | w, h | clamped 320–1600 server-side; v1 native sends 1024×768 once at connect |
| nav / reload / stop / back / fwd | url (nav) | |
| click | x, y | server double-checks focus → may reply `editable` |
| wheel | x, y, dx, dy | dx/dy are fractions too; **negated** finger deltas (see app.js `sendScroll`) |
| lpdown / lpmove / lpup | x, y (+ sel on lpup) | long-press drag; `sel:true` on lpup = no movement happened → server selects word, replies `copytext` |
| key | down, key, code, keyCode / text | printable chars go as `{t:'key', text:ch}`; Return = keyCode 13 down+up; Backspace = 8 |
| zoom | scale, cx, cy | commit after local pinch preview |
| find | q, dir | |
| tab | action: select/close/new, id | |
| paste, suggest, star, history, bookmarks, downloads… | | **read `internal/browser/features.go` + `app.js` for exact names/fields before implementing** — do not guess from this table |

### Client → server (new, native-only)

| t | fields | meaning |
|---|---|---|
| video | on: bool | enter/leave video mode. Server: stop JPEG cast fan-out to this client, ensure ffmpeg lane running (docs/03 §2), send `video-config`, stream type-3. Off = reverse |
| audio | on: bool | phase 3b; requires video mode |
| perf | decodeMs, fps | phase 4; client-measured, ~every 5s; server adapts (docs/05 §6) |

### Server → client (existing)

`hello {vw,vh}`, `tabs {tabs:[TabInfo]}`, `url {url, starred}`, `histstate {back,fwd}`, `editable {on}`, `copytext {text}`, plus suggest/find/downloads/history replies — again, exact shapes from `app.js`/`features.go`. Unknown-field tolerance is required in the native parser (the web client already ignores unknowns; native must too).

### Server → client (new)

| t | fields | meaning |
|---|---|---|
| video-config | fps, w, h, ok | sent on entering video mode; `ok:false` = lane unavailable (ffmpeg died, budget exceeded) → client stays on JPEG lane. w/h = coded size. SPS/PPS travel in-band (every IDR, `repeat-headers=1`) |
| audio-config | rate, channels, ok | phase 3b; e.g. `{rate:24000, channels:1}` |
| editable **extension** | on, kind, rect:[x,y,w,h] | `kind`: text/password/email/number/url/search (maps to `UIKeyboardType`/`secureTextEntry`); `rect` = focused element bounding box, **viewport fractions**. Additive fields — web client ignores them (docs/03 §4) |

## 4. Versioning discipline

- Any change visible to the web client → bump `config.ClientVersion`.
- Any change visible to the native client (header semantics, new/changed messages it consumes) → bump `config.NativeVersion` **and** the version compiled into the app (`RBNativeVersion` in `RBConfig.h` — keep them literally identical strings; the Phase-1 nativeprobe asserts the match).
- Never reuse a frame-type byte for different semantics. Next free: 5.

## 5. Wire examples

Type-1 frame during motion (half-res): `RBR1 | 01 | 00 | 0020 | seq | 0000 0000 | w=0200 (512) | h=0180 (384) | len | scrollX | scrollY | <jpeg>` — client scales 512×384 to the full view.

Type-3 IDR AU: `RBR1 | 03 | 01 | 0020 | seq | 0000 0000 | w=0400 | h=0300 | len | 00000000 | 00000000 | 00 00 00 01 09 … 00 00 00 01 67(SPS) … 68(PPS) … 65(IDR) …`
