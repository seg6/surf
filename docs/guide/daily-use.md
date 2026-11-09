# Daily use

Once paired, open the client, select the saved server, and browse. The host
must be running (`surf` tray app or `surf serve`).

## Browser chrome

Clients provide a native browser UI:

- Tabs (open, close, switch)
- Address bar with suggestions from history and bookmarks
- Back, forward, reload, stop
- Library: bookmarks, history, downloads
- Find in page, reader mode (when the page supports extraction)
- Share / open actions on platforms that have them
- Settings: appearance, browsing preferences, servers, diagnostics

Phone and tablet (or narrow and wide window) layouts differ. See
[Clients: iOS](../clients/ios.md) and [Clients: desktop](../clients/desktop.md).

## Page surface

The video stream matches the page area left by the native controls. Rotation,
Surf fullscreen, and page fullscreen update that area without reconnecting the
browser session.

**Mobile Websites** changes Chromium metrics and browser identity. It does not
turn touch into mouse input.

## Downloads and uploads

Downloads complete on the host under `SURF_HOME/downloads` (or `DOWNLOADS`).
Clients list finished files in Library and fetch them over the authenticated
API (byte ranges supported). Incomplete transfers stay under `.incomplete/` on
the host.

File chooser prompts on the page open a native picker on the client when the
`filechooser` capability is present. Uploads go to a temporary host directory
and into Chromium.

## Input

Desktop clients use normalized pointer and wheel input when the server
advertises `pointer-input`. Moves and wheel samples may coalesce. Button
changes, keyboard input, paste, and IME composition stay ordered. Older servers
receive touch input instead.

The iOS client sends complete active touch snapshots with stable contact IDs.
The backend rejects stale generations and sequence numbers, maps coordinates
through the current Chromium viewport, and dispatches Chromium touch events.
Navigation, tab changes, viewport changes, website mode changes, and disconnects
cancel active input.

Focus events from Chromium control the native keyboard. Plain text uses
Chromium insertion. Marked text uses IME composition events.

Client and backend compatibility uses `COMPATIBILITY_VERSION`. Matching
generations connect. An older client can receive the embedded matching package;
a newer client needs a backend update. See [Versioning](../reference/versioning.md)
and [Protocol](../reference/protocol.md#pointer-input).

## Clipboard

Optional bidirectional clipboard sync is controlled only from the host admin
UI or CLI:

```sh
surf clipboard status
surf clipboard sync on
surf clipboard get
surf clipboard set
surf clipboard set --device DEVICE_ID
surf clipboard sync off
```

With sync enabled, copied text moves between the host and connected iOS
clients. Windows uses its native clipboard API. macOS uses `pbcopy` and
`pbpaste`. Linux uses `wl-clipboard`, `xclip`, or `xsel`. A headless host
without a system clipboard can still sync connected Surf clients and expose the
in-memory value through `surf clipboard get`.

With sync disabled, desktop Settings has a **Send once** field. The terminal
prompt for `surf clipboard set` hides typed input and preserves redirected
input exactly. Clipboard text is not accepted as a command argument, stored on
disk, or written to logs. A one-time device value expires after two minutes if
unchanged. See [Security](../reference/security.md#clipboard).

## Diagnostics

Clients can show a performance overlay (health, latency, video, adaptive
profile). Host logs rotate under `SURF_HOME`:

```text
$SURF_HOME/logs/server.log
$SURF_HOME/logs/desktop.log
$SURF_HOME/logs/devices/<device-id>.ndjson
```

Clients send structured records during authenticated sessions and upload
bounded snapshots after connection and background changes.

```sh
surf logs
surf logs --follow
surf logs --source device --device DEVICE_ID
```

On iOS: **Settings -> Diagnostics -> Logs**, and
`/var/mobile/Library/Surf/surf.log`.

## Pausing for computer use

To sign into a site or change Chromium settings with the computer keyboard,
use [Browser setup](browser-setup.md). Connected clients pause until setup
ends.
