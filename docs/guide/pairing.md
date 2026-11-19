# Pairing

Pairing is closed until you open an invitation. Each invitation works for one
device, and it also ends if you cancel it, restart Surf, or enter the wrong
code five times.

To start one, open Settings from the tray and choose Pair device under Paired
Devices, or use the command line:

```sh
surf pair                 # same SURF_HOME as the running server
./surf serve --pair       # invite at startup
```

The host then shows a QR code and a six-digit code. The QR code contains the
address, part of the fingerprint and a 128-bit token, and the client checks the
TLS certificate before saving the full fingerprint.

If you pair manually, enter `host:port` and the code, then compare the six words
shown on both screens. Cancel if they differ. If the codes need to show a
different address than the one Surf binds to, set `SURF_PUBLIC_ADDRESS`.

After pairing, the client stores the address, the certificate's SHA-256 and a
separate RSA key for that server (on iOS it goes in the Keychain). The host
keeps the public key in `devices.json`. Later connections use a signed
challenge and a one-time WebSocket ticket, described in
[security](../reference/security.md).

You can list paired devices and remove one from the command line:

```sh
surf devices list
surf devices revoke DEVICE_ID
```

Revoking a device drops its sessions and tickets, and it has to pair again.

If pairing fails, these are the usual causes:

- Pairing closed: you haven't opened an invitation yet.
- SSL or identity errors: check the clocks on both devices, and make sure
  you're using the host's real address, not `127.0.0.1` from another machine.
- The words differ: you've probably reached the wrong host, so cancel.
- Server Identity Changed: the certificate or `SURF_HOME` changed. Forget the
  server on the client and pair again.

[Troubleshooting](../reference/troubleshooting.md) has more.
