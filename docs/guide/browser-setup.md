# Browser setup

Browser setup opens Surf's own Chromium profile as a normal window on the
host, so you can sign in or change browser settings with the computer's
keyboard and screen. It doesn't open your personal Chrome or Edge profile, and
it doesn't copy Surf's profile anywhere.

## While Surf is running

Choose Browser setup from the tray or dashboard, or run:

```sh
surf browser
surf browser --status
surf browser --resume
```

Opening setup pauses browsing on every connected client. Pairings and sockets
stay up. Clients that support `browser-setup` show a paused screen, and the
others get a toast.

To resume, close all the normal setup windows and it happens automatically. You
can also choose Resume on devices, or Resume here on a paired client, which asks
you to confirm. Resuming closes any setup windows that are still open on the
computer.

These carry over to the clients: cookies and Chromium settings in the Surf
profile, normal web tabs with their order and active selection, completed
downloads, and Surf bookmarks and history. These don't: unsaved form fields,
page JavaScript state, back and forward history, in-progress transfers, private
windows and `chrome://` settings pages.

If you have several windows open, they're merged into one tab list, and an empty
list becomes a new tab. Pages reload when control is handed over. Chromium's own
bookmarks aren't the same as the Surf library. Cookies come across through
Chromium's session-cookie restore, not a separate export.

## Standalone (Surf stopped)

When Surf isn't running, `surf browser` runs in the foreground with no
listener, pairing or streaming. Closing the setup windows exits without
starting Surf, and `surf quit` closes a standalone setup too. Both modes share
the same profile lock.

## Force close

If a normal resume can't close the browser, Surf stays paused and asks you to
confirm a force close separately. It never force-closes just because a handoff
timed out.

```sh
surf browser --resume --force
```

Force closing may lose recent profile changes, and it only terminates Surf's
own browser process group or job. Confirmation prompts accept `--yes`, and
stale revisions are rejected.

Browser setup needs a visible desktop session. On Linux, run Surf from the
logged-in desktop rather than a display-less SSH session. On Windows, a service
or session without an interactive desktop can't show the browser window.

For modes, locks, profile recovery and download staging, see
[browser lifetime](../concepts/browser-lifetime.md). The wire details are in the
[protocol](../reference/protocol.md#browser-setup).
