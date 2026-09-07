# Docker deployment

Mount `/data` as `SURF_HOME` so the server identity, paired devices, browser
profile, and downloads survive container replacement.

```env
SURF_HOME=/data
SURF_SERVER_NAME=Home Surf
SURF_PUBLIC_ADDRESS=192.168.1.50:18080
PORT=18080
```

Publish the same TCP port. The container runs `surf serve` in the foreground.

```sh
docker compose exec surf surf pair
docker compose exec surf surf status
```

Surf terminates TLS itself. It has no plaintext mode and does not require a
reverse proxy.

The unauthenticated health endpoint is
`https://HOST:18080/api/v1/health`. Configuration, media, files, and
diagnostics require a paired device.

See [Security](../../docs/security.md) for the pairing and host trust model.
