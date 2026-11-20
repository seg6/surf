# Capabilities

Optional features are advertised as `caps` in `/api/v1/config`. Clients should
gate features on these names instead of parsing app version strings. Adding a
capability is compatible. Removing one or changing what it means isn't, and it
may require bumping the compatibility generation.

They're defined in `backend/internal/config/config.go` (`Caps`).

| Capability | Meaning |
| --- | --- |
| `browser-setup` | Paused browser lifetime; `browser-mode` and resume |
| `dialog` | JS dialog forwarding and `dialogreply` |
| `filechooser` | Upload intercept and `POST /api/v1/uploads` |
| `history2` | Paginated history query; `histdel` and clear |
| `reader` | Reader-mode extraction |
| `security` | TLS state on URL messages |
| `pageerror` | Native error surface messages |
| `clock` | NTP-style monotonic clock synchronization |
| `dlprogress` | Download progress events |
| `dldel` | Delete a download on the server |
| `reqkeyframe` | On-demand IDR request |
| `video-retry` | Explicit retry after unavailable encoder |
| `video-pause` | Pause and resume only this client's video lane with `video` |
| `media-stats` | Client decode and presentation health for adaptive profiles |
| `live-logs` | Authenticated native log records mirrored to the host |
| `clipboard-sync` | Owner-controlled bidirectional clipboard bridge |
| `pointer-input` | Desktop pointer, hover, buttons, and wheel |
| `natural-orientation` | `size` may carry `naturalOrientation` (`portrait` or `landscape`) |
| `hidpi` | `size` may carry `scale`, with `w` and `h` in physical pixels |

## Client behavior

If a capability is absent, leave out the related commands and UI. Don't send
command types the server doesn't know, because it rejects them. UI that only
turns on when a capability is present doesn't need a generation bump.

## Related

- [Protocol](protocol.md)
- [Versioning](versioning.md)
