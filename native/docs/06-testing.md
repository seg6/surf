# 06 — Testing, probes, deploys, device checklists

The split that makes agent work viable: **everything up to the device boundary is agent-verifiable** (Go tests, container builds, protocol probes against the real deployed server, desktop ffmpeg decode of captured streams, ASan-tested C helpers). **Everything past it is a human checklist** the user runs on the iPad. An agent claiming device behavior as verified is wrong by definition.

## 1. Standing verification (every server-touching work package)

```sh
go build ./... && go vet ./... && go test ./...
node --check public/app.js                      # if the web client was touched at all
# local smoke: docker compose up --build with a published port, run probes against localhost
```

Deploy (PLAN.md rule 8 — absolute paths, always):

```sh
rsync -a --exclude .theos --exclude packages --exclude buildenv/sdk \
  /Users/null/workspace/personal/rbrowser/ hetzner:workspace/personal/vm/apps/rbrowser/
ssh hetzner "cd workspace/personal/vm/apps/rbrowser && docker compose up -d --build"
ssh hetzner "docker logs --tail 50 rbrowser-rbrowser-1"
```

Post-deploy, both probes must pass before a package is called done:
1. the existing web-protocol probe (scratchpad `probe/` — connects with `k=<wstoken>&v=<ClientVersion>`, asserts frames/nav/editable), and
2. `tools/nativeprobe` (below).

## 2. `tools/nativeprobe` — the native client's desk double

A Go CLI (checked into the repo, unlike the scratchpad probes) that exercises the exact native flow so client bugs and server bugs can be told apart before any Objective-C exists:

```
nativeprobe -host http://wrp.seg6.space -pass <password> [flags]
  (no flag)   login → /native-config → /ws?nv= → assert hello/tabs/url/histstate,
              receive N type-1 frames, ack each, assert RBR1 header sanity (magic,
              hdrlen, w/h>0, payloadLen==len), decode JPEG dims, exit PROBE-OK
  -input      send click/wheel/key sequences recorded from a real web-client session;
              assert editable{} arrives with kind+rect after focusing a text field
  -video      send video:on → assert video-config{ok:true} → capture 10s of type-3 AUs
              to out.h264 → shell out to ffmpeg -i out.h264 -f null - (zero errors),
              assert IDR cadence + SPS/PPS on every IDR + flags bit0 correctness
  -video-stall  same, but stop reading 5s mid-stream → assert resync lands on an IDR
  -audio      audio:on → capture type-4 → write WAV header → aplay/afplay-able artifact
  -soak       30 min connected, acking, logging frame gaps > 2s; for leak hunts
```

nativeprobe is also the **protocol conformance record**: when Phase 2 says "the app emits exactly what app.js emits", the comparison fixture is a JSONL of messages captured by nativeprobe's `-record` mode from a scripted web session, checked into `tools/nativeprobe/testdata/`.

## 3. Client-side agent verification

- .deb builds green in `native/buildenv` (PLAN.md rule 6) — attach the §1 transcript from docs/01 to the work package.
- `rb_h264.c` desktop tests under ASan (docs/05 §8).
- RBProtocol's header parser: mirror the Go `protocol_test` vectors — encode fixtures with the Go code, commit the bytes, parse them in a small ObjC test target compiled *in the container* (host-runnable is not required; compile-and-static-assert beats nothing, and the vectors also feed nativeprobe which runs them live).
- WS framing: nativeprobe doubles as the reference — but additionally, a table-driven set of frame fixtures (masked short/16-bit/64-bit lengths, fragmented pairs, ping) lives in `tools/nativeprobe/testdata/ws/` and both implementations (Go probe + ObjC RBSocket) must be written against it.

## 4. Device debugging loop (user + agent together)

1. User: install .deb (docs/01 §5), reproduce, then `ssh ipad cat /var/mobile/Library/WRP/wrp.log` (or scp) and paste the tail into the session.
2. Agent: correlate with `docker logs rbrowser-rbrowser-1` timestamps (both sides log connects, mode switches, resyncs).
3. Crashes: user scps `/var/mobile/Library/Logs/CrashReporter/WRP*` + agent symbolizes against the archived unstripped binary for that version (docs/01 §6).
4. The debug overlay is the first thing to check before any log: fps, decode ms, RTT, WS state, RSS, lane (JPEG/video).

## 5. Device checklists (user runs; keep answers terse — pass/fail + notes)

**Phase 0**
- [ ] Icon on SpringBoard after `uicache`; correct artwork, no gloss
- [ ] App launches to the hello screen; rotating between the two landscapes doesn't crash
- [ ] ARC smoke button: 60s run, no crash; RSS in label stays flat-ish
- [ ] wrp.log retrievable over ssh

**Phase 1**
- [ ] Login (bare prompt), stream appears; DuckDuckGo renders
- [ ] Tap accuracy: hit the search field dead-center and near screen edges/corners
- [ ] Pan: page follows finger direction; flick coasts and stops naturally; touching mid-coast stops it instantly
- [ ] Sharp text appears ≤ ~0.5s after a scroll stops (settle frame)
- [ ] Overlay: fps during scroll (expect ≥ web client's ~15–25), decode ms, RTT
- [ ] Kill Wi-Fi 10s / lock+unlock / app-switch away 1 min: silent reconnect each time
- [ ] 15-minute browse: no crash, RSS stable, iPad back not worryingly hot

**Phase 2**
- [ ] Tap a text field → keyboard up fast; correct type (URL field → URL keyboard; password → dots)
- [ ] Type a query incl. autocorrect-bait; Return submits
- [ ] Long-press a word → menu; Copy → paste into Notes ✓; copy in Notes → Paste-to-page into a form ✓
- [ ] Long-press-drag: a slider (e.g. a volume control) and map panning both work
- [ ] Tabs: open/close/switch incl. favicons; omnibox progress fill; star round-trips
- [ ] Find, History, Bookmarks, Downloads sheets; download → "Open in…" hands off
- [ ] Fullscreen toggle + dot restore
- [ ] Pinch zoom: local preview smooth, committed frame sharp, taps still land correctly while zoomed

**Phase 3**
- [ ] Video mode on: scrolling reads as *video* — no slideshow feel; note overlay fps
- [ ] Stop scrolling → text goes crisp (sharp overlay); start again → no flash/jump
- [ ] 15 min video-mode browse: heat/battery acceptable vs Phase-2 JPEG (subjective note)
- [ ] Wi-Fi blip mid-scroll → picture freezes then recovers clean (no green/gray smear persisting past ~2s)
- [ ] Toggle video off in Settings → seamless fallback to JPEG lane
- [ ] (3b) YouTube: audio plays, stays roughly in sync, survives pause/resume; audio stops when app backgrounds
- [ ] While iPad in video mode: open the web client on the Mac simultaneously — both live, neither degraded

**Phase 4**
- [ ] Overlay `perf` numbers match subjective feel; adaptive changes (if enabled) are not visible as glitches
- [ ] Jetsam torture: open several fat native apps, return — app either resumed or cold-starts straight back to the stream

## 6. Definition of done, per work package

1. Code + tests merged; `go vet`/`go test`/container build green as applicable.
2. Deployed; both probes PROBE-OK against production; web client unaffected (probe 1).
3. wrp.log/overlay instrumentation added for anything newly failable.
4. Device checklist delta written for the user (only the items this package can affect).
5. Docs updated if the wire or the plan changed (PLAN.md decision log for decisions, 02 for protocol).
