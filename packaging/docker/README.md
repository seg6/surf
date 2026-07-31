# Docker deployment

Persist `/data` as `SURF_HOME` so the TLS identity, paired devices, Chromium
profile, and downloads survive container replacement:

```env
SURF_HOME=/data
SURF_SERVER_NAME=Home Surf
SURF_PUBLIC_ADDRESS=192.168.1.50:18080
PORT=18080
```

Publish the same TCP port; the container runs `surf daemon` in the foreground.
Review a client request with `docker compose exec surf surf pair`, and inspect
the running instance with `docker compose exec surf surf status`. Surf
terminates TLS itself; there is no password or required reverse proxy.

Basic health is `https://HOST:18080/api/v1/health`. Clients authenticate with
paired device keys before configuration, media, files, or diagnostics are
available.

For the pairing and host-compromise trust boundaries, see
[`docs/security.md`](../../docs/security.md).
