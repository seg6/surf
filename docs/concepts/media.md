# Media

Surf streams the active Chromium tab to clients using Chromium tab capture, not
desktop capture, FFmpeg host pipelines, or PulseAudio virtual devices.

## Capture path

Surf prefers a compatible installed Chrome, Edge, or Chromium. If none is
found, it uses a verified managed Chromium build. Extensions (including
`Surf Tab Capture`) load through the DevTools protocol.

1. The host loads the built-in capture extension into Chromium.
2. The extension opens loopback WebSockets to the Go process (tokenized URLs).
3. Tab capture produces H.264 video and 16 kHz mono PCM audio.
4. Go wraps payloads in an 84-byte `RBR1` header and fans them out on the
   authenticated client WebSocket.
5. Clients validate headers, apply admission policy, decode, and present.

GPU page rendering stays enabled. Surf uses Chromium's software AVC encoder and
requests variable rate encoding with `STREAM_BITRATE`, falling back to constant
quality with `STREAM_QUANTIZER`. The default stream is the client-reported page
surface at 60 FPS. Clients that support `media-stats` report renderer health,
throughput, queue depth, frame age, drops, and memory pressure about every two
seconds.

## Client surface

Clients report the even-sized page area left by native controls. Width and
height must be between 64 and 1600. Rotation and fullscreen changes are
coalesced before Chromium and the encoder resize. The WebSocket stays open
across those transitions.

## Adaptive video

`SURF_ADAPTIVE_VIDEO=1` enables four frame-rate profiles at the **same** render
size (it does not shrink the viewport):

| Profile | Frame rate |
| --- | ---: |
| Crisp | 60 FPS |
| Motion | 50 FPS |
| Balanced | 40 FPS |
| Recovery | 30 FPS |

Clients that support `media-stats` report decode/presentation health about
every two seconds so the host can pick a profile.

## Generations and recovery

Each encoder generation is unique across browser replacements (including setup
handoffs). Clients drop stale generations, request IDRs (`reqkeyframe`) on
decode loss, and may send `video-retry` after an unavailable encoder state.

Binary envelope layout: [Protocol, media envelope](../reference/protocol.md#media-envelope).

## Widevine

Surf does not ship Widevine. A CDM from the selected host browser may work,
subject to site policy. Check authenticated runtime statistics for the EME
probe when debugging protected playback.
