# Browser lifetime

The host owns one browser manager for the server's lifetime. Chromium isn't a
loose dependency that the tray can restart independently of Surf's mode
machine.

## Modes

The manager publishes its state on `browser-mode`, which comes with the
`browser-setup` capability.

| State | Meaning |
| --- | --- |
| `starting` | Launching streaming browser |
| `streaming` | Normal remote browsing; page input accepted |
| `opening` | Transitioning into setup |
| `setup` | Visible Chromium on the host; clients paused |
| `resuming` | Closing setup and restoring streaming |
| `failed` | Handoff or launch failure |

Only `streaming` accepts page input. While paused, clients clear prompts,
focus, the keyboard, fullscreen and any buffered media. When streaming returns
they send the viewport and site preferences again.

## Locks and ownership

There are three things that guard ownership:

- `server.lock` allows one Surf backend per `SURF_HOME`.
- A profile lock beside the profile directory covers both streaming and
  standalone setup.
- Chromium's own singleton files are left intact, and a live or uncertain owner
  blocks startup and profile recovery.

If Chromium exits unexpectedly while streaming, the server ends too, so the
supervisor can restart both. Setup is an intentional paused state and doesn't
count as a crash.

## Tab and profile restore

Tabs, the active tab, website mode, appearance, cookies and site storage all
come back after a restart. Tab state is written after changes and again on a
clean shutdown.

Malformed replaceable state is moved aside with an `.invalid-<timestamp>`
suffix. If Chromium fails to start twice within ten minutes, Surf keeps the old
profile as `profile.startup-failed-<timestamp>` and starts a clean profile with
Surf bookmarks and history. This recovery never replaces the TLS identity
files.

## Setup handoff

Operators can read about this in [Browser setup](../guide/browser-setup.md).
The manager serializes ownership transitions, so stale revisions and
concurrent requests fail without changing the current browser.

## Downloads while running

Chromium writes unfinished downloads into a private session directory under
`DOWNLOADS/.incomplete/`. When a download finishes, Surf publishes it into
`DOWNLOADS` without overwriting an existing file, and concurrent downloads with
the same filename get different names. Only completed, non-hidden regular files
show up in Library or can be fetched through the downloads API.

Setup and filesystem failures are reported rather than announced as success.
Files left behind by a crash or a failed publication stay in `.incomplete` for
manual recovery, and Surf doesn't resume them automatically. If a completed
file is kept there after a publication error, it's named with Chromium's
download ID, and the server log identifies its path.

The file endpoint supports authenticated GET, HEAD, byte ranges and ETags. The
desktop client saves to unique temporary files and never overwrites local
copies. The iOS client uses bounded ranges and a unique local directory for
each copy.
