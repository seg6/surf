# Troubleshooting

## Client cannot find the server

1. Confirm Surf is running with `surf status`.
2. Allow TCP port `18080` (or the configured `PORT`) through the host firewall.
3. Test the local endpoint:

```sh
curl -k https://127.0.0.1:18080/api/v1/health
```

Bonjour only discovers hosts. When multicast discovery is unavailable, enter
the host name or LAN address. The address must belong to the computer
running Surf, not the client device.

## Pairing is closed

Open Settings from the tray and choose Pair device under Paired Devices, or run
`surf pair`. An invitation closes after use, cancellation, server restart, or five wrong manual codes.

Manual pairing must show the same six words on both ends. If the words differ, another
server identity was presented, so cancel the pairing.

CLI commands must use the same `SURF_HOME` as the running backend.

## Pairing shows an SSL error

Surf uses a self-signed certificate and pins its fingerprint, so public CA
validation isn't involved.

Check both system clocks and use the host LAN address instead of `127.0.0.1`
from another machine. Don't bypass a changed identity warning for a saved
server.

## Server Identity Changed

The certificate no longer matches the saved fingerprint. This can be caused by
the wrong host, a replaced `SURF_HOME`, or interception. Confirm the host
first. A reinstalled server must be forgotten and paired again.

## Paired device is rejected

```sh
surf devices list
```

A revoked device must pair again. Losing the client private key (an iOS Keychain
item) also requires pairing again. App preferences do not contain that key.

## QR scanning fails on iOS 6

Surf includes a software QR decoder. Devices without a working camera can use
the address, six-digit code, and six-word comparison.

## Video, audio, or input fails

Open the client performance overlay and check the host logs for capture or
WebCodecs errors.

```sh
surf logs --follow
```

Rotation and fullscreen should recover without manual action. Retry Video
is for a recovery that ended with video unavailable.

The compatibility values in the client's Settings and in `surf status` must
match, although the app versions may differ.

FFmpeg, PulseAudio and virtual audio devices are not part of the Chromium tab
capture path on the host.

## Stuck in browser setup / paused

Clients pause while the host has setup open. Close the setup windows, choose
Resume on devices, or confirm Resume here on a client. If closing fails, use
the separate force-close confirmation. `surf browser --resume --force` should
only be used after a failed close. See [Browser setup](../guide/browser-setup.md).

## Rotation or fullscreen has the wrong size

Surf fullscreen and page fullscreen are synchronized. The native Exit control
leaves both.

After a size change, the backend log should show the client surface and one
settled encoder resize. The WebSocket should remain open. If the client
reconnects, confirm matching compatibility generations and collect both logs
around the transition.

## Widevine or site sign-in fails

Check authenticated runtime statistics for the Widevine EME probe. Surf does
not ship a CDM. A browser-supplied CDM can work without appearing in
`chrome://components`.

Sites may still reject playback or sign-in because of account, IP, DRM, output
protection, or browser policy.

The Mobile Websites setting requests a mobile identity, so disable it when
checking a site's desktop support. For sign-in flows that don't work through
the stream, use [Browser setup](../guide/browser-setup.md).

## iOS package will not install

```sh
docker run --rm -v "$PWD:/src" surf-buildenv \
  bash /src/client/ios/verify-package.sh
```

The package must contain armv7 and arm64 builds, both device families, and the
update helper with root ownership and setuid mode.

After manual installation, run `uicache` as `mobile` if needed, then respring.

## Desktop opens but the backend does not

Read Settings, then Logs, or `$SURF_HOME/logs/desktop.log` before changing
anything.

A forced desktop exit shouldn't leave the backend, Chromium, listener, or a
live lock. Empty `.lock` files do not hold locks. Surf can move a failed
Chromium profile aside while keeping server identity and pairings.

Deleting all of `SURF_HOME` also deletes the server identity and forces every
client to pair again. Repair invalid TLS material under `identity/` only after
preserving the old contents.

## macOS Surf closes immediately at launch

Surf 0.18.0 and earlier could abort at launch on macOS with
`Assertion failed: (CGAtomicGet(&is_initialized)), function CGSConnectionByID`,
with no window or menu bar icon. This was seen on Sequoia running through
OpenCore Legacy Patcher. Update Surf to fix it.

On those versions, run the backend without the tray. `surf serve` starts the
same backend from a terminal, and clients can pair with it as usual.

## Windows update does not reopen Surf

Install Surf 0.10.3 or later. Those installers stop the existing process before
replacement and start the new version afterward. `SURF_HOME` remains in place.

## Clipboard sync has no host integration

```sh
surf clipboard status
```

Windows and macOS use system clipboard APIs. Wayland needs `wl-copy` and
`wl-paste`. X11 needs `xclip` or `xsel`. The display environment must be
available to the Surf process.

A headless service can still sync text between Surf clients through
`surf clipboard get` and `surf clipboard set`.

## Collect logs

```sh
surf logs --follow
surf logs --source device --device DEVICE_ID
```

Host copies are bounded under `$SURF_HOME/logs/`.

| Client | Location |
| --- | --- |
| iOS | Settings, then Diagnostics, then Logs; `/var/mobile/Library/Surf/surf.log` |
| Desktop | In-app diagnostics, and the logs in the preferences directory as documented in the client |

Surf omits credentials, clipboard text, tickets, query strings, and full URLs
from logs where possible. Review exports before publishing them.
