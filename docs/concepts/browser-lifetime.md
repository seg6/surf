# Browser lifetime

The host owns one browser manager for the server's lifetime. Chromium is not a
loose dependency the tray restarts independently of Surf's mode machine.

## Modes

States published on `browser-mode` (capability `browser-setup`):

| State | Meaning |
| --- | --- |
| `starting` | Launching streaming browser |
| `streaming` | Normal remote browsing; page input accepted |
| `opening` | Transitioning into setup |
| `setup` | Visible Chromium on the host; clients paused |
| `resuming` | Closing setup and restoring streaming |
| `failed` | Handoff or launch failure |

Only `streaming` accepts page input. Clients clear prompts, focus, keyboard,
fullscreen, and buffered media while paused, then re-send viewport and site
preferences when streaming returns.

## Locks and ownership

- `server.lock`: one Surf backend per `SURF_HOME`
- Profile lock beside the profile directory: streaming and standalone setup
- Chromium's own singleton files are left intact; a live or uncertain owner
  blocks startup and profile recovery

Unexpected Chromium exit during streaming ends the server so the supervisor can
restart both. Setup is an intentional paused state, not a crash.

## Tab and profile restore

Tabs, active tab, website mode, appearance, cookies, and site storage restore
after restart. Tab state is written after changes and again on clean shutdown.

Malformed replaceable state is moved aside with an `.invalid-<timestamp>`
suffix. After two Chromium startup failures within ten minutes, Surf preserves
the old profile as `profile.startup-failed-<timestamp>` and starts a clean
profile with Surf bookmarks and history. **TLS identity files are never
replaced by this recovery.**

## Setup handoff

Documented for operators in [Browser setup](../guide/browser-setup.md). The
manager serializes ownership transitions; stale revisions and concurrent
requests fail without changing the current browser.

## Downloads while running

Chromium writes unfinished downloads into a private session directory under
`DOWNLOADS/.incomplete/`. When a download finishes, Surf publishes it into
`DOWNLOADS` without overwriting an existing file. Concurrent downloads with the
same filename receive different names. Only completed, non-hidden regular files
appear in Library or can be fetched through the downloads API.

Setup and filesystem failures are reported instead of announcing success. Files
left behind by a crash or failed publication remain in `.incomplete` for manual
recovery; Surf does not automatically resume them. A completed file kept there
after a publication error is named with Chromium's download ID; the server log
identifies its path.

The file endpoint supports authenticated GET, HEAD, byte ranges, and ETags.
Desktop saves use unique temporary files and never overwrite local copies. The
iOS client uses bounded ranges and a unique local directory for each copy.
