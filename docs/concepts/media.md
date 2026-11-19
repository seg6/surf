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
controls. Rotation and fullscreen changes are coalesced before Chromium and the
encoder resize, and the WebSocket stays open through those transitions.

### Resolution

Hosts with the `hidpi` capability take the size in physical pixels, together
with a `scale`: the device pixels per CSS pixel, from 1 to 4. Chromium lays the
page out at the pixel size divided by the scale and renders with that device
scale factor, so text and images are sharp on a Retina or HiDPI display
instead of being drawn at point size and upscaled. The capture surface and the
encoder use the pixel size.

Clients lower the scale to keep the decoded surface within what they can
decode: iOS aims for about 1920x1080 (a Retina iPhone gets 2x, a Retina iPad
about 1.5x), and the desktop client allows up to 2560x1600. Scales are rounded
down in quarter steps and never go below 1. Hosts without `hidpi`, and clients
that send no `scale`, keep the old behavior: the size is in points and the
scale is 1.

`SURF_VIEWPORT_SCALE` still applies on top: it lays pages out wider at the
same pixel size, so the device scale factor is the client scale divided by it.
The host bounds a surface that sends a scale to 2560 pixels a side and 2560x1600
in area, and lowers the scale with it so the page layout does not change.
Sizes sent without a scale are bounded to between 64 and 1600 as before.

### Bitrate and H.264 level

`STREAM_BITRATE` is the rate for a 1024x768 surface. Larger surfaces get it
multiplied by their area relative to that, up to 1.5 times, so a higher
resolution is not just more compression. Smaller surfaces keep the configured
rate.

Surf declares the lowest constrained baseline level that covers the frame size
and macroblock rate: 3.1, 4.1, 4.2, or 5.1 for surfaces beyond 1920x1088. The
frame rate is lowered if it would exceed what the highest level for that size
allows.

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
