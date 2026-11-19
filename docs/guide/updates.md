# Updates

Updates replace the programs in place. You don't have to delete `SURF_HOME` or
pair again, as long as the server identity files are still there.

## Host

Desktop releases use a signed manifest and assets checked against SHA-256
hashes. On Windows, the installer stops Surf before replacing it and starts the
new version afterward.

```sh
surf update
```

Keep `SURF_HOME` when you move between versions.

## iOS client

A release build of the host may embed a matching iOS `.deb`. A paired client
can download it over its authenticated, pinned connection. The client checks the
length and SHA-256, and then the privileged helper checks the hash, package ID,
version, architecture and path before installing.

You can also refresh [https://seg6.space/surf/](https://seg6.space/surf/) in
Cydia or Sileo, or install a `.deb` by hand.

This prevents the package from being swapped on the network. It doesn't protect
against a compromised host or a stolen server identity. See
[security](../reference/security.md#updates).

## Compatibility

App versions can differ. A client and host connect as long as their
compatibility generation and wire markers agree. If the client is older, it can
be offered the embedded package. If it's newer, the host needs an update.

See [versioning](../reference/versioning.md).

## After updating

```sh
surf status
surf version
```

Check that your clients reconnect. If you didn't touch the identity files,
pairing still works.
