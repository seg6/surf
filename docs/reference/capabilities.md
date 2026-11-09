# Capabilities

Optional features are advertised in `/api/v1/config` as `caps`. Clients
feature-gate on these names instead of parsing app version strings. Adding a
capability is compatible. Removing one or changing its meaning is not, and may
require a compatibility generation bump.

Defined in `backend/internal/config/config.go` (`Caps`).

| Capability | Meaning |
| --- | --- |
| `browser-setup` | Paused browser lifetime; `browser-mode` / resume |
| `dialog` | JS dialog forwarding and `dialogreply` |
| `filechooser` | Upload intercept and `POST /api/v1/uploads` |
| `history2` | Paginated history query; `histdel` / clear |
| `reader` | Reader-mode extraction |
| `security` | TLS state on URL messages |
| `pageerror` | Native error surface messages |
| `clock` | NTP-style monotonic clock synchronization |
| `dlprogress` | Download progress events |
| `dldel` | Delete a download on the server |
| `reqkeyframe` | On-demand IDR request |
| `video-retry` | Explicit retry after unavailable encoder |
| `media-stats` | Client decode/presentation health for adaptive profiles |
| `live-logs` | Authenticated native log records mirrored to the host |
| `clipboard-sync` | Owner-controlled bidirectional clipboard bridge |
| `pointer-input` | Desktop pointer, hover, buttons, and wheel |

## Client behavior

- If a capability is absent, omit the related commands and UI.
- Do not send unknown command types; the server rejects them.
- Additive UI that only activates when a cap is present does not need a
  generation bump.

## Related

- [Protocol](protocol.md)
- [Versioning](versioning.md)
