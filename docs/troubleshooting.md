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
- Use **Retry Video** after a transient encoder failure.
- Check the desktop logs for Chromium capture or WebCodecs errors.
- Make sure the client and backend have the same Surf/protocol release.

Surf's active-tab capture handles audio and video directly. Installing FFmpeg,
PulseAudio, or a virtual audio device will not help this path.

## Widevine or streaming-site sign-in

Open `chrome://components` in the remote browser and confirm the host browser
provides Widevine. Surf does not ship a CDM. A site may still reject playback
because of account, DRM, output-protection, or browser-policy requirements.

## Native package will not install

Run the package verifier from the repository root:

```sh
docker run --rm -v "$PWD:/src" surf-buildenv \
  bash /src/native/client/verify-package.sh
```

The package must contain armv7 and arm64 slices, declare device families 1 and
2, and retain the privileged updater's root ownership/setuid mode. After a
manual install, run `uicache` and respring if the icon does not appear.

## Collect logs

Desktop logs are available from the Surf tray Settings page. The native client
writes `/var/mobile/Library/Surf/surf.log`; avoid publishing complete logs until
you have checked them for visited URLs and device names.
