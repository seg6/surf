# Media

Surf streams the active Chromium tab to clients using Chromium's tab capture.
It doesn't use desktop capture, FFmpeg pipelines on the host, or PulseAudio
virtual devices.

## Capture path

Surf prefers a compatible installed Chrome, Edge or Chromium, and uses a
verified managed Chromium build if it can't find one. Extensions, including
`Surf Tab Capture`, are loaded through the DevTools protocol.

1. The host loads the built-in capture extension into Chromium.
2. The extension opens loopback WebSockets to the Go process, using tokenized
   URLs.
3. Tab capture produces H.264 video and PCM audio. Audio is 48 kHz stereo by default, or 16 kHz
   mono with `SURF_AUDIO_QUALITY=standard`.
4. Go wraps the payloads in an 84-byte `RBR1` header and fans them out on the
   authenticated client WebSocket.
5. Clients validate the headers, apply admission policy, decode and present.

GPU page rendering stays enabled. Surf uses Chromium's software AVC encoder and
asks for variable rate encoding with `STREAM_BITRATE`, falling back to constant
quality with `STREAM_QUANTIZER`. By default the stream is the page surface the
client reports, at 60 FPS. Clients that support `media-stats` report renderer
health, throughput, queue depth, frame age, drops and memory pressure about
every two seconds.

## Client surface

Clients report the even-sized page area that's left over after the native
controls. Width and height must be between 64 and 1600. Rotation and fullscreen
changes are coalesced before Chromium and the encoder resize, and the WebSocket
stays open through those transitions.

## Adaptive video

Setting `SURF_ADAPTIVE_VIDEO=1` enables four frame rate profiles. They all use
the same render size, so the viewport never shrinks.

| Profile | Frame rate |
| --- | ---: |
| Crisp | 60 FPS |
| Motion | 50 FPS |
| Balanced | 40 FPS |
| Recovery | 30 FPS |

Clients that support `media-stats` report decode and presentation health about
every two seconds, which is how the host picks a profile.

## Generations and recovery

Each encoder generation is unique across browser replacements, including setup
handoffs. Clients drop stale generations and request IDRs (`reqkeyframe`) when
decoding loses data. They may also send `video-retry` after the encoder was
unavailable.

The binary envelope layout is in [Protocol, media envelope](../reference/protocol.md#media-envelope).

## Widevine

Surf doesn't ship Widevine. A CDM from the selected host browser may work,
depending on site policy. When debugging protected playback, check the
authenticated runtime statistics for the EME probe.
