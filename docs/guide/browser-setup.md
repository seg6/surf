# Browser setup

Opens Surf's Chromium profile as a normal host window so you can sign in or
change browser settings with the computer's keyboard and screen. It does not
open your personal Chrome/Edge profile or copy Surf's profile elsewhere.

## While Surf is running

**Browser setup...** in the tray/dashboard, or:

```sh
surf browser
surf browser --status
surf browser --resume
```

Opening setup **pauses browsing on every connected client**. Pairings and
sockets stay up. Clients with `browser-setup` show paused; others get a toast.

Resume by closing all normal setup windows (automatic), **Resume on devices**,
or **Resume here** on a paired client (with confirmation). Resuming closes the
local setup windows.

| Carries over | Does not |
| --- | --- |
| Cookies and Chromium settings in the Surf profile | Unsaved form fields |
| Normal web tabs, order, and active selection | Page JavaScript state |
| Completed downloads; Surf bookmarks and history | Back/forward history; in-progress transfers |
| | Private windows and `chrome://` settings pages |

Multiple windows flatten into one tab list. Empty tab list -> new tab. Pages
reload on handoff. Chromium bookmarks != Surf Library. Handoffs use Chromium's
session-cookie restore, not a separate cookie export.

## Standalone (Surf stopped)

`surf browser` runs in the foreground: no listener, pairing, or streaming.
Closing setup windows exits without starting Surf. `surf quit` closes
standalone setup too. Streaming and standalone share the profile lock.

## Force close

If normal resume cannot close the browser, Surf stays paused and asks for a
separate force-close confirmation (never force-closes only because a handoff
timed out).

```sh
surf browser --resume --force
```

May lose recent profile changes. Only Surf's browser process group/job is
terminated. Confirmation prompts accept `--yes`. Stale revisions are rejected.

Needs a visible desktop session. On Linux, run Surf from the logged-in
desktop, not a display-less SSH session. On Windows, a service/session without
an interactive desktop cannot show the browser window.

Modes, locks, profile recovery, and download staging:
[Browser lifetime](../concepts/browser-lifetime.md). Protocol:
[Browser setup](../reference/protocol.md#browser-setup).
