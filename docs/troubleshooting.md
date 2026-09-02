# Troubleshooting

## The client cannot find the server

1. Confirm that Surf is running.
2. Allow TCP port `18080` through the host firewall.
3. Test the local endpoint.

```sh
curl -k https://127.0.0.1:18080/api/v1/health
```

Bonjour only discovers hosts. Enter the host name or LAN address when multicast
discovery is unavailable. The address must belong to the computer running Surf,
not the iOS device.

## Pairing is closed

Open **Paired Devices** and choose **Pair device**, or run `surf pair`. An
invitation closes after use, cancellation, server restart, or five wrong
manual codes.

Manual pairing must show the same six words on both ends. Different words mean
that another server identity was presented, so the attempt must be cancelled.

CLI commands must use the same `SURF_HOME` as the running backend. `surf
status` shows the selected server and control connection.

## Pairing shows an SSL error

Surf uses a self signed certificate and pins its fingerprint. Public CA
validation is not involved.

Check both system clocks and use the host LAN address instead of `127.0.0.1`.
Never bypass a changed identity warning for a saved server.

## Server Identity Changed

The certificate no longer matches the saved fingerprint. The address may point
to another server, `SURF_HOME` may have been replaced, or the connection may
be intercepted.

Confirm the host. A reinstalled server must be forgotten and paired again.

## A paired device is rejected

Run `surf devices list`. A revoked device must pair again. Loss of the private
Keychain key also requires **Pair Again**. App preferences do not contain that
key.

## QR scanning fails on iOS 6

Surf includes a software QR decoder. Devices without a working camera can use
the address, six digit code, and six word comparison.

## Video, audio, or input fails

Open **More > Surf Settings > Performance Overlay** and check the host logs for
capture or WebCodecs errors.

Rotation and fullscreen should recover without manual action. **Retry Video**
is for a recovery that ended with video unavailable.

The compatibility values in Settings and `surf status` must match. Surf app
versions may differ.

FFmpeg, PulseAudio, and virtual audio devices are not part of the Chromium tab
capture path.

## Rotation or fullscreen has the wrong size

Surf fullscreen and page fullscreen are synchronized. The native Exit control
leaves both.

After a size change, the backend log should show the client surface and one
settled encoder resize. The WebSocket should remain open. If the client
reconnects, confirm matching compatibility generations and collect both logs
around the transition.

## Widevine or site sign in fails

Check the authenticated runtime statistics for the Widevine EME probe. Surf
does not ship a CDM. A browser supplied CDM can work without appearing in
`chrome://components`.

Sites may still reject playback or sign in because of account, IP, DRM, output
protection, or browser policy.

Desktop mode changes the `HeadlessChrome/` product token to `Chrome/`.
**Mobile Websites** requests a mobile identity. Disable it when checking a
site's desktop support.

## The iOS package will not install

Run the package verifier from the repository root.

```sh
docker run --rm -v "$PWD:/src" surf-buildenv \
  bash /src/client/ios/verify-package.sh
```

The package must contain armv7 and arm64 builds, both device families, and the
update helper with root ownership and its setuid mode.

After manual installation, run `uicache` and respring if the icon is missing.
Older systems may require:

```sh
su mobile -c uicache
```

## The desktop opens but the backend does not

Read **Settings > Logs** or
`$SURF_HOME/logs/desktop.log` before changing state.

A forced desktop exit should not leave the backend, Chromium, listener, or a
live lock. Empty `.lock` files do not hold locks. Surf repairs replaceable
state and can move a failed Chromium profile aside while keeping the server
identity and pairings.

Deleting all of `SURF_HOME` also deletes the server identity and forces every
client to pair again. An invalid TLS identity should be repaired or replaced
under `identity/` only after its old contents have been preserved.

## A Windows update does not reopen Surf

Install Surf 0.10.3 or later. Those installers stop the existing process before
replacement and start the new version afterward. `SURF_HOME` remains in place.

## Clipboard sync has no host integration

Run `surf clipboard status`.

Windows and macOS use their system clipboard APIs. Wayland needs `wl-copy`
and `wl-paste`. X11 needs `xclip` or `xsel`. The display environment must
be available to the Surf process.

A headless service can still sync text between Surf clients and use
`surf clipboard get` and `surf clipboard set`.

## Collect logs

Desktop Settings can follow server, desktop, or client logs. The terminal
equivalent is:

```sh
surf logs --follow
```

Host copies are bounded under `$SURF_HOME/logs/`.

On iOS, logs are under **Settings > Diagnostics > Logs** and at
`/var/mobile/Library/Surf/surf.log`. Surf omits credentials, clipboard text,
tickets, query strings, and full URLs. Exported logs still need review before
publication.
