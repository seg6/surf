# 05 — Video (H.264/VideoToolbox) and audio lanes

This is the payoff phase. Server side is docs/03 §2–3; this doc covers the client decode/render path and the tuning/budget work. **Fallback posture everywhere:** any failure in this lane degrades to the JPEG lane automatically — the app must never be *worse* than Phase 2 because video broke.

## 1. VideoToolbox on iOS 6 — what's true

- VideoToolbox.framework exists on iOS 6 but is **private** (public API arrived in iOS 8). On a jailbroken device there's no entitlement barrier; we link `PrivateFrameworks/VideoToolbox.framework` from the 6.1 SDK.
- Headers: the 6.1 SDK does not ship usable VT headers. Vendor a minimal header (`native/client/Vendor/VTDecompressionSession.h`) declaring only what we call: `VTDecompressionSessionCreate`, `VTDecompressionSessionDecodeFrame`, `VTDecompressionSessionInvalidate`, `VTDecompressionSessionWaitForAsynchronousFrames`, the callback record struct, and the handful of option keys — copied from the known jailbreak-era community headers (iOS-6 era `VideoToolbox` dumps; also cross-check against the public iOS 8 header, whose ABI matches for these entry points). Every declaration gets a comment naming its source.
- CoreMedia/CoreVideo parts we need (`CMVideoFormatDescriptionCreate`, `CMBlockBuffer*`, `CMSampleBufferCreate`, `CVOpenGLESTextureCache*`) are **public since iOS 4–5** — no vendoring needed.
- Runtime posture: `dlopen`/weak-link and feature-test at startup; if session creation fails, log loudly, tell the server `{"t":"video","on":false}`, stay on JPEG.

## 2. Bitstream handling (RBVideoDecoder, net thread → VT)

Server sends type-3 frames: one complete **Annex-B access unit** each, SPS/PPS repeated at every IDR, flags bit0 = IDR (docs/02).

Per AU:
1. Scan NALs (start-code split). On SPS (type 7) + PPS (type 8): if different from current, (re)create the format description — build an **avcC** box from them and call
   `CMVideoFormatDescriptionCreate(alloc, kCMVideoCodecType_H264, w, h, extensions)` with extensions `{ kCMFormatDescriptionExtension_SampleDescriptionExtensionAtoms: { "avcC": <data> } }`.
   (Do **not** use `CMVideoFormatDescriptionCreateFromH264ParameterSets` — that's iOS 7+. The avcC-extensions route is public API since iOS 4 and is the known-working iOS 6 pattern.)
   avcC layout: `01 | profile | compat | level | FF (4-byte lengths) | E1 | spsLen16 | sps | 01 | ppsLen16 | pps`.
2. Format description changed → invalidate + recreate the VT session:
   `VTDecompressionSessionCreate(alloc, fmtDesc, NULL /*decoderSpec*/, destAttrs, &callbackRecord, &session)` with `destAttrs = { kCVPixelBufferPixelFormatTypeKey: kCVPixelFormatType_32BGRA }` — asking VT for BGRA pushes the YUV→RGB conversion into the decoder path and keeps our render trivial (v1; see §4 for the GL upgrade).
3. Convert the AU's VCL/non-VCL NALs (drop the AUD; SPS/PPS may stay in-band or be dropped once in the fmtDesc — dropping is cleaner) from start-code to **4-byte length prefixes** in one contiguous buffer → `CMBlockBufferCreateWithMemoryBlock` (transfer ownership, no copy) → `CMSampleBufferCreate` with the fmtDesc.
4. `VTDecompressionSessionDecodeFrame(session, sampleBuffer, 0 /*sync is fine at 15fps baseline*/, NULL, &flagsOut)`.
5. Errors: single-frame decode error → count it, wait for next IDR (decode nothing until flags bit0 set), resync. Session-level error (`kVTInvalidSessionErr`, e.g. after app resume) → tear down, recreate at next IDR. >3 resyncs/30s → drop to JPEG lane, report via debug overlay + log.

Decode-callback (VT thread): take the CVPixelBuffer, retain, hand to main.

## 3. Render (v1: BGRA → CALayer)

Main thread: lock the pixel buffer's base address only if copying; better — keep it simple and correct first:
`CVPixelBufferLockBaseAddress` → `CGDataProviderCreateWithData` wrapping the base address (with a release callback that unlocks+releases the pixel buffer) → `CGImageCreate` (BGRA little-endian, skip-first) → `baseLayer.contents`. One image live at a time; a new frame releases the old chain.

Cost check: 1024×768 BGRA is 3MB; creating a CGImage wrapper is cheap (no copy). A5 compositing one opaque full-screen layer at 15fps is nothing. **Only** if the overlay shows main-thread render cost >8ms/frame do we do §4.

## 4. Render (v2, only if needed): OpenGL ES 2 + CVOpenGLESTextureCache

`CVOpenGLESTextureCacheCreateTextureFromImage` (public, iOS 5+) gives zero-copy textures from the decoder's pixel buffers. Ask VT for `kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange` instead of BGRA, bind luma (R/luminance) + chroma (RG/luminance-alpha) planes, BT.601 video-range YUV→RGB in a tiny fragment shader, draw one quad in a CAEAGLLayer-backed view. This is the classic pipeline every 2012-era video app used; it's well-trodden but it's ~400 lines of GL boilerplate — hence v2, gated on measurement.

## 5. Mode orchestration & the sharp-settle overlay

- Enter video mode automatically when the connection reports itself capable (VT session smoke test at launch) — user toggle in Settings ("Video streaming: on/off") for A/B and battery testing. Send `{"t":"video","on":true}`; on `video-config{ok:true}`, base layer switches source video↔JPEG atomically (whichever lane owns the base layer, the other is fully quiesced).
- **Sharp overlay:** in video mode the server still sends type-1 sharp JPEGs on settle (docs/03 §3). Client decodes them on the JPEG queue and shows them on `overlayLayer` (above the video layer). Hide the overlay the moment the user next interacts (touch-down) — not on next AU (at 15fps with zerolatency, static page = tiny identical P-frames that would immediately replace crisp text with codec smear). This gives codec-smooth motion + JPEG-crisp reading, each where it's strong.
- Audio: `{"t":"audio","on":true}` only while in video mode and app foregrounded; off on background.

## 6. Tuning, budgets, adaptive quality

Initial targets (measure, then adjust — record measurements in this doc):

| Knob | Start | Notes |
|---|---|---|
| fps | 15 | A5 decode headroom is huge (hw decoder does 1080p30); 15 is a *server CPU* choice. Try 20–24 after measuring |
| bitrate | 1500k / maxrate 2000k | at 1024×768: fine for UI motion; text crispness comes from the sharp overlay, not bitrate |
| x264 preset | superfast | ultrafast if VPS CPU tight (worse rate-distortion, ~same speed class) |
| coded size | 1024×768 | 960×720 fallback knob only if VPS CPU forces it |

**Server CPU budget (risk R3):** measure `docker stats` on the VPS: idle, web-client scroll (Chromium+screencast), video-mode scroll (Chromium+x264). If Chromium+x264 sustained >85% of the box, drop fps first, then preset, then size. The encoder only runs while a native video client is connected, so worst case is bounded to active use.

**Network:** 2Mbps peak over the iPad's 802.11n — trivial. LAN-quality Wi-Fi assumed; the IDR-resync backpressure (docs/03 §2) covers blips.

**Phase 4 adaptive loop:** client sends `{"t":"perf", decodeMs, fps}` every 5s; server nudges ffmpeg config (requires encoder restart — do it only on sustained signal, ≥30s, and only between IDR periods) or, cheaper, adjusts nothing and just surfaces the numbers first. Instrument before automating.

## 7. Audio playback (RBAudioPlayer, phase 3b)

Type-4 frames: PCM s16le mono @ rate from `audio-config` (24000). AudioQueue with 6 × 100ms buffers:
- Enqueue incoming chunks into a ring; the AudioQueue output callback pulls from the ring; underrun → fill with silence (never stall the queue).
- Start playback only after ~200ms buffered (jitter buffer); if the ring exceeds 400ms, drop oldest down to 200ms (drift control — no timestamps needed at this quality bar).
- `AVAudioSession`-equivalent on iOS 6: `AudioSessionInitialize` + category `MediaPlayback`, handle interruptions (calls don't exist on an iPad but timers/alarms do).
- A/V sync: none by design. Browsing audio (music players, video voice) tolerates ±150ms; revisit only if it grates on device.

## 8. Verification without the device (agent-runnable)

- `tools/nativeprobe -video`: subscribes, writes received AUs to `out.h264`, then `ffmpeg -i out.h264 -f null -` must decode with zero errors and report the expected fps/resolution; assert IDR cadence (every 2s) and that every IDR AU carries SPS/PPS.
- Backpressure test: probe artificially stalls reads for 5s → server log shows resync; probe verifies the first AU after the gap is an IDR and that ffmpeg still decodes the file cleanly end-to-end.
- avcC builder + Annex-B→AVCC converter: write them as pure, platform-independent C functions in their own file (`Classes/rb_h264.c` + header). That file compiles into the app unchanged **and** into a desktop test binary (`clang -fsanitize=address rb_h264.c rb_h264_test.c`) run against a captured-AU fixture — so the trickiest byte-shuffling is fully agent-verifiable on the Mac.
- Client decode path itself: device-only (§9 checklist in docs/06). Everything up to the VT call is desk-testable; keep that boundary thin.
