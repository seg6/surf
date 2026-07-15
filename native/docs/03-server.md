# 03 — Server work (Go)

All changes live in the existing module. Invariants from PLAN.md rule 4 apply everywhere: never hold `b.mu` across `cdp.Call`; never block the CDP dispatch goroutine; client writes only through the hub outbox.

## 1. Phase 1: native handshake (small, ship first)

- `internal/config`: add `const NativeVersion = "<date>-1"` next to `ClientVersion`.
- `internal/httpd`:
  - `GET /native-config` — behind the normal auth-cookie check (it's below the `s.auth.Valid` gate like other assets); returns `{"token": s.auth.Token, "vw", "vh", "nv": config.NativeVersion, "host"}`. `Cache-Control: no-store`.
  - `handleWS`: accept `q.Get("v") == config.ClientVersion || q.Get("nv") == config.NativeVersion`; pass `native := q.Get("nv") != ""` into `hub.Serve`.
- `internal/ws`: `Client.Native bool`. Frame fan-out unchanged for type-1. Add `SendRaw([]byte)` for pre-encoded type-3/4 messages that bypasses the type-1 ack window but **shares the same outbox** (one writer goroutine per client, always).
- Log lines: `ws connected (native nv=...)` — keep the reject log symmetric.

Agent gate: `tools/nativeprobe` (docs/06 §2) does login → native-config → ws and streams type-1 frames; web client still connects (bump nothing).

## 2. Phase 3: `internal/stream` — the H.264 lane

New package owning one ffmpeg process and its consumers. It reads the **X display directly** (`DISPLAY=:99`, see `start.sh`) — zero interaction with CDP or the screencast path.

```go
package stream

type Config struct {
    Display string  // ":99"
    W, H    int     // coded size; default VW,VH; optional STREAM_SCALE env (e.g. 960x720)
    FPS     int     // STREAM_FPS, default 15
    BitrateK, MaxrateK, BufsizeK int // defaults 1500/2000/500
    Preset  string  // STREAM_PRESET, default "superfast"
}

type Streamer struct { /* mu, cmd, subscribers map[*Sub]struct{}, lastConfig */ }

func (s *Streamer) Subscribe() *Sub    // starts ffmpeg on 0→1 subscribers
func (sub *Sub) Close()                // stops ffmpeg on 1→0 (after 5s linger)
type AU struct { Data []byte; IDR bool; Seq uint32 }
```

### ffmpeg invocation

The Docker image (see §5) gains ffmpeg. Command (adjust only with measurements in hand):

```
ffmpeg -loglevel warning -f x11grab -framerate {FPS} -video_size {W}x{H} -i {Display} \
  -c:v libx264 -profile:v baseline -level 3.1 -preset {Preset} -tune zerolatency \
  -x264-params keyint={2*FPS}:min-keyint={2*FPS}:scenecut=0:repeat-headers=1:aud=1 \
  -b:v {B}k -maxrate {M}k -bufsize {Buf}k -pix_fmt yuv420p -f h264 pipe:1
```

Why each flag matters:
- `baseline` — A5 hardware decoder sweet spot; no B-frames (zero reorder latency).
- `zerolatency` — no lookahead, frame out per frame in.
- `keyint=2*FPS, scenecut=0` — IDR every 2s exactly; predictable recovery points for the drop-to-IDR backpressure below.
- `repeat-headers=1` — SPS/PPS in front of every IDR, so a client can join/recover mid-stream without out-of-band parameter sets.
- `aud=1` — Access Unit Delimiter NALs, making AU splitting a trivial scan instead of slice-header parsing.
- `bufsize` small — caps how far instantaneous quality can overshoot the rate (latency control).

### AU splitter (readLoop)

Scan the pipe for start codes (`00 00 01` / `00 00 00 01`); an AU runs from one AUD NAL (type 9) to the next. Tag `IDR` if any NAL in the AU has type 5. Keep the raw Annex-B bytes intact (client wants start codes). Stamp `Seq`. Hand the AU to every subscriber.

### Backpressure (per subscriber — this is the part agents get wrong)

Video frames are not independently droppable: P-frames reference their predecessors. Per-sub state machine:

- Buffered channel (cap ~30 AUs). Normal: enqueue.
- Channel full → **drop this and every subsequent AU until the next IDR**, then resume from that IDR. Set a `dropped` flag; on resume, log `stream: sub resynced at seq=N`.
- Never block the splitter on a slow subscriber.

### Process management

- Start: `exec.CommandContext`, stderr to a prefixed logger. Health = first AU within 3s, else kill + `video-config{ok:false}` to waiting subscribers.
- Crash mid-run: notify subs (`video-config{ok:false}`), auto-restart once with 2s backoff if subscribers remain; second crash within 60s → give up until next Subscribe.
- Idle stop: last unsubscribe arms a 5s timer (fast video-off/on toggles shouldn't cycle the process).
- Shutdown: tie into the browser's existing shutdown path so ffmpeg never outlives the server.

## 3. Phase 3: browser/hub wiring for video mode

`internal/browser` handles `{"t":"video","on":true}`:
1. Subscribe the client to the Streamer; on `ok`: reply `video-config{fps,w,h,ok:true}`.
2. **Stop sending this client type-1 cast frames** (hub per-client flag `videoMode`). The screencast itself keeps running if web clients are attached; if the native client is the only one, stop the cast (CPU back for x264).
3. Sharp-settle overlay: the existing settle path (`sendFreshFrame` after `SettleMS`) **still sends this client a type-1 sharp frame** — the one JPEG exception in video mode. Client overlays it and drops the overlay on next user input (docs/05 §5).
4. `video off` (or disconnect — hook the hub's client-removal path): unsubscribe, restore normal cast fan-out, `ensureCast`.

Tab switches, navigation, zoom need **no** stream plumbing — ffmpeg films the X display, which always shows the active tab; whatever CDP does to the page shows up on screen. (Zoom via `Emulation.setDeviceMetricsOverride` changes the rendered content, not the display size — verify visually on device; if the emulated viewport doesn't fill the Xvfb screen at zoom, that's a bug to chase in the emulation params, not the stream.)

## 4. Phase 2: `editable` extension (focus rect + kind)

Extend `checkEditable` (`internal/browser/input.go`) — the evaluated expression additionally returns, when editable: tag/type mapped to `kind` (text/password/email/number/url/search) and `getBoundingClientRect()` normalized to **viewport fractions** (divide by `innerWidth/innerHeight` in-page — the server shouldn't re-derive zoom math). Reply becomes `{"t":"editable","on":true,"kind":"text","rect":[x,y,w,h]}`. Web client reads only `on` — additive, no ClientVersion bump strictly required, but bump anyway per discipline (comment why).

## 5. Phase 3b: audio

- **Dockerfile**: add `pulseaudio` + ffmpeg (ffmpeg already added for video). `start.sh`: start `pulseaudio --system=false --daemonize --exit-idle-time=-1` with a null sink (`load-module module-null-sink sink_name=rb`), `export PULSE_SINK=rb` before Chromium launch. Verify Chromium actually outputs (it must NOT be launched with `--mute-audio`; check `internal/cdp/cdp.go` launch flags and remove it if present).
- `internal/stream`: second, much simpler pipeline — `ffmpeg -f pulse -i rb.monitor -f s16le -ar 24000 -ac 1 pipe:1`, chunked at 4800-byte reads (100ms), wrapped as type-4 frames. Same subscribe/idle-stop pattern; audio drops are just… dropped (client jitter buffer rides over it).
- `audio-config{rate:24000, channels:1, ok}` on subscribe.

## 6. Config additions (env, all optional)

```
STREAM_FPS=15  STREAM_SCALE=      # empty = VWxVH; "960x720" to shrink
STREAM_BITRATE=1500  STREAM_MAXRATE=2000  STREAM_BUFSIZE=500  STREAM_PRESET=superfast
AUDIO=0|1 (default 1 once phase 3b lands)
```

## 7. Order of implementation

1. §1 handshake (Phase 1 blocker; tiny).
2. §4 editable extension (Phase 2; tiny).
3. §2+§3 video lane (Phase 3; testable end-to-end with nativeprobe + desktop ffmpeg before any client code exists — do it in parallel with client Phase 2 if staffing allows).
4. §5 audio (after video proves out on device).

Every step: `go build ./... && go vet ./... && go test ./...`, deploy per PLAN.md rule 8, run both probes (docs/06), confirm the **web** client path with the existing protocol probe before calling it done.
