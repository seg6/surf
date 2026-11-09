# Troubleshooting

## Client cannot find the server

1. Confirm Surf is running (`surf status`).
2. Allow TCP port `18080` (or your `PORT`) through the host firewall.
3. Test the local endpoint:

```sh
curl -k https://127.0.0.1:18080/api/v1/health
```

Bonjour only discovers hosts. Enter the host name or LAN address when
multicast discovery is unavailable. The address must belong to the computer
running Surf, not the client device.

## Pairing is closed

Open **Paired Devices** -> **Pair device**, or run `surf pair`. An invitation
closes after use, cancellation, server restart, or five wrong manual codes.

Manual pairing must show the same six words on both ends. Different words mean
another server identity was presented. Cancel.

CLI commands must use the same `SURF_HOME` as the running backend.

## Pairing shows an SSL error

Surf uses a self-signed certificate and pins its fingerprint. Public CA
validation is not involved.

Check both system clocks and use the host LAN address instead of `127.0.0.1`
from another machine. Never bypass a changed identity warning for a saved
server.

## Server Identity Changed

The certificate no longer matches the saved fingerprint. Causes: wrong host,
replaced `SURF_HOME`, or interception. Confirm the host. A reinstalled server
must be forgotten and paired again.

## Paired device is rejected

```sh
surf devices list
```

A revoked device must pair again. Loss of the client private key (iOS Keychain
item) also requires pairing again. App preferences do not contain that key.

## QR scanning fails on iOS 6

Surf includes a software QR decoder. Devices without a working camera can use
the address, six-digit code, and six-word comparison.

## Video, audio, or input fails

Open the client performance overlay and check host logs for capture or
WebCodecs errors.

```sh
surf logs --follow
```

Rotation and fullscreen should recover without manual action. **Retry Video**
is for a recovery that ended with video unavailable.

Compatibility values in client Settings and `surf status` must match. App
versions may differ.

FFmpeg, PulseAudio, and virtual audio devices are not part of the Chromium tab
capture path on the host.

## Stuck in browser setup / paused

Clients pause while the host has setup open. Close setup windows, choose
**Resume on devices**, or confirm **Resume here** on a client. If close fails,
use the separate force-close confirmation (`surf browser --resume --force`
only after a failed close). See [Browser setup](../guide/browser-setup.md).

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

**Mobile Websites** requests a mobile identity. Disable it when checking a
site's desktop support. Use [Browser setup](../guide/browser-setup.md) for
awkward sign-in flows.

## iOS package will not install

```sh
docker run --rm -v "$PWD:/src" surf-buildenv \
  bash /src/client/ios/verify-package.sh
```

The package must contain armv7 and arm64 builds, both device families, and the
update helper with root ownership and setuid mode.

After manual installation, run `uicache` as `mobile` if needed, then respring.

## Desktop opens but the backend does not

Read **Settings -> Logs** or `$SURF_HOME/logs/desktop.log` before changing
state.

A forced desktop exit should not leave the backend, Chromium, listener, or a
live lock. Empty `.lock` files do not hold locks. Surf can move a failed
Chromium profile aside while keeping server identity and pairings.

Deleting all of `SURF_HOME` also deletes the server identity and forces every
client to pair again. Repair invalid TLS material under `identity/` only after
preserving the old contents.

## Windows update does not reopen Surf

Install Surf 0.10.3 or later. Those installers stop the existing process before
replacement and start the new version afterward. `SURF_HOME` remains in place.

## Clipboard sync has no host integration

```sh
surf clipboard status
```

Windows and macOS use system clipboard APIs. Wayland needs `wl-copy` /
`wl-paste`. X11 needs `xclip` or `xsel`. The display environment must be
available to the Surf process.

A headless service can still sync text between Surf clients via
`surf clipboard get` / `set`.

## Collect logs

```sh
surf logs --follow
surf logs --source device --device DEVICE_ID
```

Host copies are bounded under `$SURF_HOME/logs/`.

| Client | Location |
| --- | --- |
| iOS | Settings -> Diagnostics -> Logs; `/var/mobile/Library/Surf/surf.log` |
| Desktop | In-app diagnostics; preferences directory logs as documented in the client |

Surf omits credentials, clipboard text, tickets, query strings, and full URLs
from logs where possible. Review exports before publication.
