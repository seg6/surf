# Pairing

Closed until you open an invitation. One device per invitation; also ends on
cancel, restart, or five wrong codes.

```sh
surf pair                 # same SURF_HOME as the running server
./surf serve --pair       # invite at startup
```

Tray: **Paired Devices** -> **Pair device**. Host shows QR + six-digit code.

**QR**: address, fingerprint fragment, 128-bit token. Client checks the TLS
cert before saving the full fingerprint.

**Manual**: enter `host:port` and the code, then compare the six words on both
ends. Cancel if they differ. Set `SURF_PUBLIC_ADDRESS` when codes must show a
different address than the bind address.

After pairing the client stores address, cert SHA-256, and a per-server RSA key
(iOS: Keychain). Host keeps the pubkey in `devices.json`. Later connects use a
signed challenge and a one-shot WebSocket ticket ([security](../reference/security.md)).

```sh
surf devices list
surf devices revoke DEVICE_ID
```

Revoke drops sessions and tickets; that device must pair again.

| Problem | Check |
| --- | --- |
| Pairing closed | Open an invitation |
| SSL / identity | Clocks, real host address (not `127.0.0.1` from another machine) |
| Words differ | Wrong host; cancel |
| Server Identity Changed | Cert/`SURF_HOME` changed; forget and re-pair |

More: [Troubleshooting](../reference/troubleshooting.md).
