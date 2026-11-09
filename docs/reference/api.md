# HTTP API

`/api/v1` on the Surf TLS listener. No plaintext HTTP.

| Class | Requirement |
| --- | --- |
| Public | Reachability / pairing bootstrap |
| Device session | Cookie/session after challenge |
| WebSocket | Single-use device-bound ticket |
| Admin | Loopback + control token from `daemon.json` |

`/api/v1/health` and `/api/v1/server` are unauthenticated (reachability /
metadata). Browser, media, files, stats, updates, and WebSockets need a paired
device (or admin where noted).

## Public and pairing

| Method | Path | Role |
| --- | --- | --- |
| GET | `/api/v1/health` | Liveness |
| GET | `/api/v1/server` | Server metadata for discovery/UI |
| GET | `/api/v1/tunnel` | Tunnel helper when `SURF_TUNNEL_HOST` is set |
| POST | `/api/v1/pairing/request` | Start/continue a pairing attempt |
| GET | `/api/v1/pairing/status/{id}` | Pairing status |
| POST | `/api/v1/pairing/confirm/{id}` | Client confirmation |
| POST | `/api/v1/pairing/cancel/{id}` | Cancel |
| POST | `/api/v1/pairing/ack/{id}` | Acknowledge completion |

Pairing is closed until the owner opens an invitation on the host.

## Authentication

| Method | Path | Role |
| --- | --- | --- |
| POST | `/api/v1/auth/challenge` | Fresh 30s challenge |
| POST | `/api/v1/auth/complete` | Signature -> HTTP session |
| POST | `/api/v1/auth/revoke` | Device self-revoke (gated) |

Challenges bind API v1, server ID, device ID, challenge ID, and nonce. Success
yields a session cookie (`surf_device`, 24h) used for subsequent device routes.

## Session device routes

| Method | Path | Role |
| --- | --- | --- |
| GET | `/api/v1/config` | Caps, compatibility, client configuration |
| GET | `/api/v1/ws` | WebSocket upgrade with single-use ticket |
| GET/POST | `/api/v1/browser-mode` | Setup status / resume (device) |
| POST | `/api/v1/client/logs` | Upload bounded client log snapshots |
| * | `/api/v1/tab-icons/*` | Favicon assets |
| POST | `/api/v1/uploads` | File chooser uploads |
| * | `/api/v1/downloads/*` | List/fetch downloads (GET/HEAD, ranges, ETags) |
| GET | `/api/v1/updates/client` | Embedded iOS package when present |

```sh
curl -k https://127.0.0.1:18080/api/v1/health
```

## Admin (loopback)

Under `/api/v1/admin/...` with the control token:

| Area | Examples |
| --- | --- |
| Pairing | Create/get/delete session; list/reject candidates |
| Devices | List; revoke |
| Logs | Sources, fetch, stream, refresh, clear |
| Clipboard | Get/set text; enable/disable sync |
| Browser | GET status; POST `open` / `resume` with revision |
| Config / shutdown | Local dashboard operations |

Admin is for the tray, dashboard, and `surf` CLI on the same machine. Not for
remote administration over the public port.

## WebSocket

After authentication, the client obtains a short-lived ticket and connects to
`/api/v1/ws`. The socket carries:

- Text frames: control JSON (commands in, events out)
- Binary frames: `RBR1` media

See [Protocol](protocol.md).
