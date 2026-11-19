# Daily use

Once paired, open the client, select the saved server and browse.
The host has to be running, either the `surf` tray app or `surf serve`.

## Browser chrome

The client gives you a native browser interface:

- Tabs you can open, close and switch between
- An address bar with suggestions from your history and bookmarks
- Back, forward, reload and stop
- A library for bookmarks, history and downloads
- Find in page, and reader mode when the page supports it
- Share and open actions on platforms that have them
- Settings for appearance, browsing preferences, servers and diagnostics

The layout differs between phone and tablet sizes, or between a narrow and a
wide window. The [iOS client notes](../clients/ios.md) and
[desktop client notes](../clients/desktop.md) go into the details.

## Page surface

The video stream fills whatever area the native controls leave for the page.
Rotating the device, going fullscreen in Surf, or going fullscreen on a page
all update that area without reconnecting the browser session.

Mobile Websites changes Chromium's metrics and browser identity, but it doesn't
turn touch into mouse input.

## Downloads and uploads

Downloads finish on the host, in `SURF_HOME/downloads` (or wherever `DOWNLOADS`
points). Clients list the finished files in the library and fetch them over the
authenticated API, which supports byte ranges. Incomplete transfers stay in
`.incomplete/` on the host.

When a page asks for a file, the client opens its own picker, as long as the
`filechooser` capability is present. The upload goes to a temporary directory
on the host and is then handed to Chromium.

## Input

Desktop clients send normalized pointer and wheel input when the server
advertises `pointer-input`. Moves and wheel samples can be merged together, but
button changes, keyboard input, paste and IME composition always stay in order.
Older servers get touch input instead.

The iOS client sends the full set of active touches each time, with stable
contact IDs. The backend rejects stale generations and sequence numbers, maps
the coordinates through the current Chromium viewport, and dispatches Chromium
touch events. Navigating, changing tabs, changing the viewport or website mode,
and disconnecting all cancel any active input.

Chromium's focus events decide when the native keyboard shows up. Plain text is
inserted through Chromium directly, and marked text uses IME composition
events.

The client and backend decide whether they're compatible using
`COMPATIBILITY_VERSION`. If the generations match, they connect. An older
client can be offered the matching embedded package, and a newer client needs
the backend updated. See [versioning](../reference/versioning.md) and the
[protocol](../reference/protocol.md#pointer-input).

## Clipboard

Clipboard sync between the host and your devices is optional, and you can only
control it from the host's admin UI or the command line:

```sh
surf clipboard status
surf clipboard sync on
surf clipboard get
surf clipboard set
surf clipboard set --device DEVICE_ID
surf clipboard sync off
```

With sync on, copied text moves between the host and connected iOS clients.
Windows uses its native clipboard API, macOS uses `pbcopy` and `pbpaste`, and
Linux uses `wl-clipboard`, `xclip` or `xsel`. A headless host with no system
clipboard can still sync connected Surf clients, and you can read the value it
holds in memory with `surf clipboard get`.

With sync off, desktop Settings has a Send once field. The prompt for
`surf clipboard set` hides what you type and keeps redirected input exactly as
it is. Clipboard text can't be passed as a command argument, and it's never
stored on disk or written to logs. A one-time value sent to a device expires
after two minutes if it hasn't been used. See
[security](../reference/security.md#clipboard).

## Diagnostics

Clients can show a performance overlay with health, latency, video and the
adaptive profile. The host rotates its logs under `SURF_HOME`:

```text
$SURF_HOME/logs/server.log
$SURF_HOME/logs/desktop.log
$SURF_HOME/logs/devices/<device-id>.ndjson
```

Clients send structured records during authenticated sessions, and they upload
size-limited snapshots after the connection or the app's background state
changes.

```sh
surf logs
surf logs --follow
surf logs --source device --device DEVICE_ID
```

On iOS you can open Settings, then Diagnostics, then Logs. The log file is at
`/var/mobile/Library/Surf/surf.log`.

## Pausing for computer use

If you need to sign in to a site or change Chromium settings with the
computer's keyboard, use [Browser setup](browser-setup.md). Connected clients
pause until you're done.
