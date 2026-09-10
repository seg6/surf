# Repository demo recording

This records a genuine backend/Linux-client session for a short README demo.
It does not run the gallery, impersonate iOS footage or modify your Surf profile.

Dependencies: modern Node.js with built-in WebSocket/fetch, FFmpeg with X11 and
PulseAudio capture/libx264/AAC/drawtext, Xvfb, xsetroot, a supported Chromium
browser, and the usual Go/Rust desktop build dependencies. Mini takes also need
`pactl` and PulseAudio or PipeWire's PulseAudio compatibility server.

The selected mini proof uses a real 768×1024 portrait window, the existing light
theme, bottom controls and the normal client viewport path. The browser page is
768×982 while browsing and 768×1024 in fullscreen. Capture is 1:1: higher DPI
would currently change the requested browser dimensions, not just video quality.

```sh
node client/desktop/demo/record.mjs --mini --portrait
node client/desktop/demo/record.mjs --mini --portrait --youtube
node client/desktop/demo/edit-mini.mjs READING_TAKE VIDEO_TAKE
# Same footage at its native dimensions, without the caption composition:
node client/desktop/demo/edit-mini.mjs --client-only READING_TAKE VIDEO_TAKE
```

The first command records a real touch swipe on Wikipedia. The second prepares
YouTube off camera (rejecting optional cookies and selecting a 1080p source),
then records playback and entering fullscreen. Read-only CDP snapshots locate
player controls and check readiness and geometry. Recorded input still goes
through the actual Linux client, shared core, session and backend; CDP does not
inject input or take over capture. Site UI changes may require script updates.
Omit `--portrait` for the earlier 1024×768 landscape variant. Both editor inputs
must have the same orientation; the editor never rotates or stretches a take.

The editor cuts between normal-speed takes, preserves client aspect ratio and
retains captured audio. The captioned portrait edit fits its content at 1280×1080
rather than forcing a 16:9 canvas; the client stays 720×960 within that edit.
It writes `surf-demo-preview.mp4` and `poster-demo.png`, with plain captions and
source credit. The client-only export is a framing review, not a publication-ready
credited video. Check film/music permissions and attribution before publishing.

The earlier dark, wide Wikipedia/Aquarium proof is still available:

```sh
node client/desktop/demo/record.mjs
node client/desktop/demo/edit.mjs .local/demo/take-XXXXXX
```

The recorder creates a unique private take directory, builds the current source,
starts a loopback-only backend and private X display, pairs a separate client,
and drives the real UI through XTest. Only its own processes are stopped.
Public Wikipedia and WebGL Aquarium pages are visited; Internet access is needed.
Page availability and content can change, so inspect each take before publishing.

Raw footage, timing markers, logs, the edited MP4 and poster remain in the take
directory. Logs/profile data may contain session credentials: do not publish
them. Only the MP4/poster are intended for distribution.

## Full cut

Cursor capture is disabled for new takes (`-draw_mouse 0`); no pointer is painted
out afterward. Record individual, replaceable scenes with:

```sh
node client/desktop/demo/record.mjs --mini --portrait --scene=browsing
# Other scenes: navigation, map, webgl, library, downloads
node client/desktop/demo/edit-full.mjs .local/demo/full-cut.json
```

The full editor reads a private manifest with a `scenes` array. Each entry has
`take` (a repository-relative directory), `title`, `lines`, optional `credit`,
and optional `start` / `end` seconds within that take. Missing trim points use
its `reading` / `end` markers. Optional `gainDB` changes captured audio level,
not its timing. The editor validates portrait capture geometry, trims paired
audio/video at normal speed, and writes a new ignored `full-*` directory with
`surf-demo.mp4`, `poster.png`, `chapters.json`, credits, and a private manifest.

The page scenes begin interacting promptly and use short pauses between inputs,
while still waiting for actual navigation, tab, and download state. The opening
explanation can sit over scrolling footage; it does not need a static title hold.
Keep footage at normal speed when tightening the edit. Netflix review/rights
notes belong in the accompanying credits, not as a draft label in the picture.

For GitHub publication, a repository MP4 link opens the file viewer; use an
uploaded media attachment URL on its own line for the README's inline player.
Keep a copy below 10 MB for GitHub's free-plan upload limit. The published demo
uses two-pass H.264 at 1250 kbit/s, preserving dimensions and frame rate and
copying the original AAC audio. See [GitHub's attachment documentation](https://docs.github.com/en/github-cli/github-cli/attaching-files-with-github-cli).
Uploading the attachment is a separate publication action; the recorder and
editors do not upload files or create issues, comments, or releases.

Inspect the entire result before sharing. The scene scripts use public sites
whose controls and content may change; readiness failures stop the take rather
than silently including a broken interaction. The download scene can also cause
the real client to save an EPUB in its working directory; keep that generated
file with the private take, not in the repository source.

The full draft includes a verified Netflix playback excerpt for review. Its
publication rights are not established; review permission or remove its scene
entry before publishing. See [CREDITS.md](CREDITS.md) for sources, attribution,
recording provenance, and private-file exclusions. Nothing is uploaded.

The proof uses footage at normal speed, without added music or voiceover.
Recording at 60 fps is an encoding choice, not an iOS performance benchmark.
There is no on-video Linux label. For accompanying descriptions, these recordings
use the Linux client with an iPad mini-sized portrait viewport, not filmed iOS
hardware. Surf's iOS app has its own native interface. Product copy covers legacy
iPhone, iPad and iPod touch devices, not just the device preset used to record.

## Audio and sync

Each mini take creates its own null sink. The recorder routes only its spawned
client's audio streams, resolved by process/client IDs, and records that sink's
monitor. It does not change the default sink or capture the microphone or other
applications. The backend's ordinary audio transport and client playback are
both exercised; audio is not substituted from the source film. PipeWire routing
is explicitly checked because its ALSA path can ignore `PULSE_SINK`.

```sh
node client/desktop/demo/record.mjs --mini --portrait --sync
node client/desktop/demo/check-sync.mjs SYNC_TAKE
```

The fixture produces five flashes/beeps; the analyzer reports onset differences
and drift. It fails beyond 120ms offset or 50ms drift. This checks the recording
path, not a physical iPad. Do not shift the soundtrack to conceal a Surf issue.

## Netflix preflight (private)

Use only the dedicated `.local/demo/netflix/profile` Chrome profile. Create it
with mode 0700, sign in manually with Chrome's `--user-data-dir` pointed there,
`--password-store=basic`, `--disable-sync` and `--disable-gpu`. Do not save a
password there or copy another browser's cookies. Close that Chrome completely.

```sh
node client/desktop/demo/record.mjs --mini --portrait --netflix --inspect
```

Only this option selects `chrome-netflix.sh`, which removes `--enable-gpu` and
adds `--disable-gpu` to the dedicated launch. It defaults to
`/opt/google/chrome/chrome`; `SURF_DEMO_CHROME` can select the executable used
for the manual login. Normal Surf launches and the other takes keep their GPU
configuration. The recorder refuses an existing Chrome profile lock rather
than deleting it. The signed-in profile is retained between takes.

Inspection records privately for up to 180 seconds; an `inspect.done` file in
the reported take directory ends it early. Homepage preview playback is not
proof that a complete protected title plays and captures through Surf. Verify
actual playback, audio and capture separately; do not bypass DRM restrictions.
Keep account identifiers out of public footage. Netflix is not yet an automated
publication scene, and all footage needs review before inclusion. The current
full draft uses a manually verified title excerpt, excluding profile selection;
an attached media key, advancing playback, visible recorded picture, and actual
client sound were checked. This does not establish publication rights.
