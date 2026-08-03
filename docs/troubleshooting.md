# Troubleshooting

## The client cannot find the server

- Confirm Surf is running and the computer firewall allows TCP port `18080`.
- Test locally with `curl -k https://127.0.0.1:18080/api/v1/health`.
- Bonjour is only discovery. Enter the LAN hostname or IP manually if it is
  filtered by the network.
- Use the computer's address, not the iOS device's address.

## Pairing is not started

Open **Paired Devices**, choose **Pair device**, then scan that QR code or
enter its six-digit code. For a headless daemon, run `surf pair`. Pairing is
closed before this step, even when Bonjour or manual address entry finds the
server.

The invitation does not expire on a timer. It closes when one client uses it,
the owner cancels it, the daemon restarts, or five incorrect codes are entered.
A client that has not presented the invitation credential has no access to
browser data or media.

Manual pairing must show the same six words on both endpoints. If the phrases
differ, cancel: something is presenting a different server identity.

If a CLI command says the daemon is not running, start `surf daemon` under your
service manager and run the command with the same `SURF_HOME`. `surf status`
shows the daemon and control connection selected by the CLI.

## Pairing shows an SSL error

The self-signed Surf certificate is expected; the client validates its exact
fingerprint instead of a public CA. Confirm the computer and iOS device clocks
are reasonably correct, then retry with the computer's LAN address rather than
`127.0.0.1`. If the endpoint belongs to a saved server, do not bypass a changed
identity warning—verify the host or explicitly forget and pair it again.

## Server Identity Changed

Surf refuses an endpoint whose certificate differs from the saved pin. This
can mean the address now reaches another server, `SURF_HOME` was replaced, or a
connection is being intercepted. Verify the host first. To accept a genuinely
reinstalled server, explicitly **Forget Server** and pair again.

## A paired device is rejected

Check `surf devices list`. A revoked device must pair again. If the client lost
its device-only Keychain key, use **Pair Again**; copying app preferences does
not copy the private key.

## QR scanning on iOS 6

Surf includes a software QR decoder for camera-equipped iOS 6 devices. If the
device has no usable camera, enter the address and six-digit code instead.
Manual pairing also uses the six-word identity comparison.

## Video, audio, or input trouble

- Open **More > Surf Settings > Performance Overlay** for decoder, latency,
  network, and audio health.
- Rotation and fullscreen reconfiguration should recover automatically. Use
  **Retry Video** only if that recovery ultimately reports video unavailable.
- Check the desktop logs for Chromium capture or WebCodecs errors.
- Make sure the client and backend have the same Surf/protocol release.

Surf's active-tab capture handles audio and video directly. Installing FFmpeg,
PulseAudio, or a virtual audio device will not help this path.

## Rotation or fullscreen has the wrong size

Surf fullscreen and the remote page Fullscreen API are synchronized. A page
player such as YouTube can enter native fullscreen, and the native Exit control
leaves both states; Back and Forward are intentionally hidden in fullscreen.

After rotation or a fullscreen change, the backend log should report the exact
even-sized client surface and one settled tab-encoder resize. Intermediate
orientation sizes are coalesced and must not close the WebSocket or require
**Retry Video**. If the client reconnects, confirm the client and backend use
the same protocol build, then collect both logs around the transition.

## Widevine or streaming-site sign-in

Check Surf's authenticated runtime statistics for the live Widevine EME probe.
`chrome://components` only lists browser-managed components; a host-supplied
CDM used by Chromium can work without appearing there. Surf does not ship a
CDM. A site may still reject sign-in or playback because of its account,
datacenter-IP, DRM, output-protection, or browser-policy requirements.

For desktop pages, Surf changes only the classic `HeadlessChrome/` product
token to `Chrome/`. Browser brands, full versions, platform details, and
`Sec-CH-UA-*` values are read from the selected Chrome/Chromium build and
returned unchanged. **Mobile Websites** is the deliberate exception: it
requests an Android/mobile device identity while retaining the browser's
native brand and version list. Keep that option off when testing a service's
desktop Linux support.

## Native package will not install

Run the package verifier from the repository root:

```sh
docker run --rm -v "$PWD:/src" surf-buildenv \
  bash /src/native/client/verify-package.sh
```

The package must contain armv7 and arm64 slices, declare device families 1 and
2, and retain the privileged updater's root ownership/setuid mode. After a
manual install, run `uicache` and respring if the icon does not appear.
On older systems, use `su mobile -c uicache`; running it as `root` may fail to
open SpringBoard's cache.

## The desktop opens but the backend does not start

Check **Settings > Logs** before removing state. Surf retries transient daemon
failures with bounded backoff and records the actual startup error in
`$SURF_HOME/desktop.log`.

A force-closed tray should not leave a daemon, Chromium tree, listener, or
backend lock behind. Empty `.lock` files are normal and do not hold a lock by
themselves. Dead runtime descriptors and malformed replaceable state are backed
up and repaired automatically. Repeated Chromium startup failures preserve the
old profile under `SURF_HOME` and retry with clean browser state while retaining
the server identity and pairings.

Do not delete all of `SURF_HOME` as a first repair step: that also destroys the
pinned server identity and forces every client to pair again. If the log reports
an invalid TLS identity, preserve the directory and repair or deliberately
replace only `identity/`; clients will correctly reject a replacement identity
until explicitly forgotten and paired again.

## A Windows update does not reopen Surf

Install Surf 0.10.3 or later. Those installers force-close the installed Surf
process tree before replacing it and launch the new version after a silent
update. `SURF_HOME` is not removed, so the server identity, pairings, and
browser profile remain in place.

## Collect logs

Desktop logs are available from the Surf tray Settings page. On iOS, open
**Settings > Diagnostics > Logs** for color-coded structured events, expandable
typed fields, live updates, copy, and clear controls. The bounded NDJSON store
is `/var/mobile/Library/Surf/surf.log`; Surf excludes credentials, tickets,
query strings, and full URLs, but review exported logs before publishing them.
