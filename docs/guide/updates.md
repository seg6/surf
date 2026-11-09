# Updates

Updates replace programs in place. They do not require deleting `SURF_HOME` or
pairing again, as long as the server identity files remain.

## Host

Desktop releases use a signed manifest and SHA-256 checked assets. The Windows
installer stops Surf before replacing it and starts the new version afterward.

```sh
surf update
```

Keep `SURF_HOME` when moving between versions.

## iOS client

The host may embed a matching iOS `.deb` in a release build. A paired client
can download it over the authenticated, pinned connection. The client checks
length and SHA-256; the privileged helper checks hash, package ID, version,
architecture, and path before install.

You can also refresh [https://seg6.space/surf/](https://seg6.space/surf/) in
Cydia/Sileo or install a `.deb` manually.

This blocks network substitution of the package. It does not protect against a
compromised host or stolen server identity. See
[Security](../reference/security.md#updates).

## Compatibility

App versions may differ. Peers connect when **compatibility generation** and
wire markers agree. An older client against a newer generation can be offered
the embedded package. A newer client requires a host update.

See [Versioning](../reference/versioning.md).

## After update

```sh
surf status
surf version
```

Confirm clients reconnect. If identity files were not touched, pairing remains
valid.
