# 04 — The Objective-C client (iOS 6.0+, iPad, landscape)

Style: plain UIKit, no storyboards/xibs (theos-friendly, reviewable diffs), no third-party deps. ARC per docs/01 §3. Every class prefixed `RB`. Target look: the web client's graphite/gold design language translated to native (flat, hairline borders, Avenir Next — it's a native iOS 6 font).

## 1. Class map

```
RBAppDelegate            window, root VC, log/crash handlers, memory-warning fanout
RBConfig.h               RBNativeVersion, defaults keys, tunables (one header, no magic numbers elsewhere)
RBSession                login (POST /login via NSURLConnection), cookie storage,
                         /native-config fetch, owns RBSocket, reconnect w/ backoff 1.5s→15s
RBSocket                 hand-rolled RFC6455 client over NSStream (§3), dedicated thread
RBProtocol               RBR1 header parse (hdrlen-driven!), frame structs, JSON encode/decode
                         (NSJSONSerialization), tolerant of unknown fields/types
RBFrameQueue             latest-wins slot per lane + ack bookkeeping (§4)
RBJPEGDecoder            GCD serial queue, ImageIO decode + forced decompression (§4)
RBVideoDecoder           phase 3, VideoToolbox (docs/05)
RBAudioPlayer            phase 3b, AudioQueue (docs/05 §7)
RBStreamView             UIView showing the remote page: base CALayer (video or JPEG),
                         overlay CALayer (sharp settle frame), pinch-preview transform
RBInputController        gesture recognizers → protocol messages (§5); owns inertia timer
RBKeyboardShim           hidden UITextField, editable{} handling, per-char send (§6)
RBChromeController       top bar: back/fwd, omnibox+progress+star, reload/stop, menu (§7)
RBTabStrip               horizontal UIScrollView of tab cells (favicon via /tabicon with cookie)
RBSheet*                 RBSheetFind, RBSheetHistory, RBSheetBookmarks, RBSheetDownloads,
                         RBSheetSettings — one shared slide-up container + UITableView each
RBDebugOverlay           fps / decode ms / RTT / WS state / RSS; triple-tap toggle
RBLog                    file+NSLog logger (docs/01 §6)
```

## 2. Threading model (three lanes, strict)

- **Net thread** (one NSThread + runloop, owned by RBSocket): stream I/O, WS framing, RBR1 header parse. Hands complete messages off; never decodes images, never touches UIKit.
- **Decode queue** (GCD serial): JPEG decode + decompress. Video decode happens on VT's own callback thread (docs/05).
- **Main thread**: all UIKit/CALayer, gesture handling, and *sending* decisions (sends are marshaled to the net thread via `performSelector:onThread:`).

Rule: data crosses threads by handoff of immutable NSData/structs; no shared mutable state without a queue. Any UIKit call off-main is a bug, full stop.

## 3. RBSocket — hand-rolled WebSocket (client side, plain ws:// only)

We deliberately skip SocketRocket (2013 dependency archaeology; we need no TLS — the transport is plain HTTP by design for iOS 6). ~400 lines, testable against the real gorilla server via nativeprobe parity.

**Connect:** `CFStreamCreatePairWithSocketToHost(host, 80)`, schedule both streams on the net thread's runloop, open. Send the upgrade:

```
GET /ws?k=<token>&nv=<ver> HTTP/1.1
Host: <host>
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: <16 random bytes, base64>
Sec-WebSocket-Version: 13
```

Read until `\r\n\r\n`; require `101` and `Sec-WebSocket-Accept == base64(SHA1(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))` (CommonCrypto `CC_SHA1`). Any non-101 (esp. 403) → error path in RBSession (docs/02 §1).

**Framing:**
- Receive: parse FIN/opcode/len (7-bit, 126→uint16, 127→uint64); server frames are unmasked. Handle opcodes: 1 text, 2 binary, 8 close (echo close, then teardown), 9 ping (reply pong with same payload), 0 continuation — **gorilla can fragment; accumulate until FIN** (cap the assembly buffer at 4MB; overflow = protocol error → reconnect).
- Send: always FIN, always **masked** (4 random bytes; XOR payload) — RFC requires it and gorilla enforces it. Text for JSON, binary never sent by us.
- Own liveness: if no frame of any kind for 20s, send `{"t":"poke"}`; nothing for 35s → declare dead, reconnect. (Server pings periodically; responding to ping is handled above.)

**Teardown/reconnect (RBSession):** exponential backoff 1.5s ×1.6 cap 15s, reset on a successful `hello`. On app foreground (`applicationDidBecomeActive`), if socket not open, reconnect immediately. On reconnect, re-run `/native-config` only after a 403.

## 4. JPEG lane: frame queue, decode, render, ack

Mirrors the web client's semantics exactly — the server's 3-deep inflight window doesn't know or care which client it's talking to.

1. Net thread parses a type-1 frame → `RBFrameQueue.offer(frame)`: a **single pending slot**; if occupied, the old frame is replaced (coalesce) and — critical — **an ack is sent for the replaced frame too**. One `{"t":"ready"}` per received frame, no exceptions, or the pipeline stalls at 3.
2. Decode queue (if idle) takes the slot: `CGImageSourceCreateWithData` → `CGImageSourceCreateImageAtIndex` → force decompression *on this queue* by drawing into a `CGBitmapContext` (device RGB, `kCGImageAlphaNoneSkipFirst|kCGBitmapByteOrder32Little`) sized to the image. iOS 6 has no `kCGImageSourceShouldCacheImmediately`; drawing once is the reliable decompress-off-main trick. Keep exactly one scratch context per size (frames alternate 1024×768 / 512×384 during motion — cache both).
3. Main thread: `streamView.baseLayer.contents = (id)cgImage` (CATransaction with actions disabled — no implicit fades), aspect-scale via `contentsGravity` = resize (the layer is always viewport-shaped; half-res frames stretch, matching web behavior). Send `ready` *after* handing off to the layer (paced acks = server-side pacing, same as web).
4. `poke` watchdog & `hello` resync: on `hello`, send `{"t":"size", w:1024, h:768}` once, reset seq expectations.

Budget check (device gate): decode+draw of a 1024×768 q60 JPEG on A5 should land ~20–35ms on the decode queue; overlay shows it. If >60ms sustained, don't optimize blindly — that's the video lane's job.

## 5. Input (RBInputController)

All coordinates sent as **fractions** of RBStreamView's bounds. Recognizers on the stream view:

- **Tap** → `{"t":"click", x, y}` + small gold fleck animation (design parity). Server may reply `editable`.
- **Pan** (1 finger) → stream of `{"t":"wheel", x:anchorX, y:anchorY, dx, dy}` where anchor = touch-down point (fixed for the whole gesture — the web client learned this the hard way; a moving anchor crosses scrollable regions) and `dx = -translationDelta.x / viewW`, etc. (**negated** finger delta, matching `app.js` `sendScroll`). Send at recognizer callback rate; the server's motion/half-res machinery handles the load.
- **Inertia**: on pan end, take `velocityInView`, run a CADisplayLink-driven decay `v *= pow(0.998, dtMs)` emitting wheel messages until `|v| < 40 pt/s`; cancel instantly on any new touch. Identical constants to app.js — feel parity, and NO local content movement (PLAN.md rule 10).
- **Pinch** (2 finger): live local preview — `streamView` transform scaled about the pinch centroid (pure visual); on end, send `{"t":"zoom", scale, cx, cy}` (centroid as fractions), keep the preview transform until the next frame arrives, then reset it in the same transaction the frame lands (web client's trick — avoids the double-jump).
- **Long-press** (0.5s): `lpdown` at point; subsequent moves → `lpmove` (drag: sliders, maps, text selection); on release → `lpup` with `sel: !moved`. Server replies `copytext` when a selection was made → §6 clipboard. Show the web client's drag-ring affordance equivalent (a subtle ring under the finger).
- **Two-finger tap** → back (nice-to-have, phase 4).

## 6. Keyboard, clipboard, text (RBKeyboardShim)

**Focus-in:** on `editable{on:true, kind, rect}` → configure the hidden UITextField (`keyboardType` from `kind`, `secureTextEntry` for password, autocorrection OFF, autocapitalization OFF) → `becomeFirstResponder`. Native keyboard rises at native speed. If `rect` provided and the keyboard would cover it (rect bottom fraction > ~0.45 in landscape), slide the whole stream view up by the covered amount (visual-only transform; input math unaffected since coords are fractions of the *view*, which moved as a unit).

**Typing:** `shouldChangeCharactersInRange:` — single printable char → `{"t":"key", text:ch}`. Empty replacement → Backspace `{"t":"key", down:true, keyCode:8, key:"Backspace"}` + up. Multi-char replacement (autocomplete/paste-into-field) → the burst-paste message (read `features.go`/`app.js` for the exact `t` — same one web uses for >2-char bursts). Return key → keyCode 13 down+up (server's `\r` special-case handles submit) then `resignFirstResponder` unless `kind` suggests multiline (textarea → keep keyboard).

**Focus-out:** `editable{on:false}` → resign. User "Done" on keyboard → resign locally only (no server message; matches web).

**Clipboard — the native superpower, get it right:**
- `copytext{text}` → `[UIPasteboard generalPasteboard].string = text` immediately + toast "Copied". *Also* offer `UIMenuController` with Copy/Define anchored at the long-press point when the text is short (Define = `UIReferenceLibraryViewController`, exists on iOS 6 iPad).
- Paste: menu action + omnibox button → read `generalPasteboard.string` → send the server's paste/insert message (exact shape from `features.go`). This closes the Notes→remote-form loop iOS 6 Safari never could.

## 7. Chrome (RBChromeController + RBTabStrip + sheets)

Layout (landscape 1024×768): top bar 44pt — [back][fwd] [omnibox: siteicon | url/progress | star | reload-or-stop] [menu]; tab strip 32pt below; stream view fills the rest (1024×692 visible). **Fullscreen**: menu action hides both bars (stream view gets 1024×768 — matching the server viewport 1:1, the ideal); a 24pt translucent dot bottom-right restores. Send `size` only if we choose per-mode viewport sizes later — v1 keeps 1024×768 always and letterboxes the chromed mode via aspect fit (simpler; revisit in Phase 4).

- **Omnibox**: UITextField styled flat graphite; while loading, a gold progress fill behind the text driven by the server's progress messages (same source app.js uses); editing state swaps to full-URL + Go keyboard button → `{"t":"nav", url}`. Suggestions: `{"t":"suggest", q}` per keystroke (250ms debounce) → dropdown table of results.
- **Star** toggles bookmark (server message per `features.go`); filled gold when `url{starred:true}`.
- **Tabs**: render from `tabs` broadcasts; cell = favicon (`GET /tabicon/<id>?v=..` with the auth cookie via NSURLConnection, tiny in-memory cache) + title + close ×; [+] at the end. Optimistic active-tab highlight on tap (web parity), corrected by the next `tabs` broadcast.
- **Menu** (UIPopoverController-style, custom): Find, History, Bookmarks, Downloads, Paste to page, Zoom reset, Fullscreen, Settings, Debug overlay.
- **Sheets**: one `RBSheet` container (slide-up, dim behind, Done button) hosting a UITableView each. Downloads rows: name/size, tap → download via NSURLConnection to `tmp/` → `UIDocumentInteractionController` ("Open in…" — GoodReader etc.). History/bookmarks rows navigate on tap. Find: text field + prev/next chevrons + count label → `{"t":"find", q, dir}`.
- **Settings/login screen** (also first-launch): server URL (default `http://wrp.seg6.space`), password field → RBSession login flow; connection status line; version strings (app + server nv) for mismatch debugging.

## 8. Lifecycle & memory

- `applicationDidEnterBackground`: keep the socket (iOS 6 gives ~10min via beginBackgroundTask — take it, cheap); on expiry or kill, reconnect-on-foreground covers it.
- `didReceiveMemoryWarning` (fanned out from the app delegate): drop favicon cache, scratch bitmap contexts, any queued frame; log RSS before/after.
- Steady-state allocations: two scratch contexts (~3MB + 0.75MB), one displayed CGImage (~3MB), pending NSData (≤200KB), video decoder buffers (docs/05). Total well under 40MB — jetsam-safe territory on a 512MB device.

## 9. Phase deliverables

- **Phase 1**: RBAppDelegate, RBConfig, RBLog, RBSession, RBSocket, RBProtocol, RBFrameQueue, RBJPEGDecoder, RBStreamView, RBInputController (tap/pan/inertia only), RBDebugOverlay. No chrome — fullscreen stream only, server URL hardcoded to the deployed instance, password typed into a bare alert on first run.
- **Phase 2**: RBKeyboardShim, pinch+longpress+clipboard, RBChromeController, RBTabStrip, sheets, settings screen, icon.
- **Phase 3**: RBVideoDecoder + RBAudioPlayer wiring (docs/05).
