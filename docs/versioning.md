# Versioning

Surf tracks releases, network compatibility, optional features, and the client
core ABI separately.

## Version dimensions

| Dimension | Source | Purpose |
| --- | --- | --- |
| App version | `VERSION` | Surf release shown by the backend and clients |
| Compatibility generation | `COMPATIBILITY_VERSION` | Blocks client and backend pairs that cannot interoperate |
| Wire marker or range | `WireCompatibilityVersion` and the `RBR1` header | Identifies control and media envelopes |
| Capabilities | `backend/internal/config/config.go` | Negotiates optional features |
| Core ABI | `SURF_CORE_ABI_VERSION` | Identifies the C interface used by native hosts |

A future trace format needs its own schema number. Logs are not protocol traces.

## App version

`VERSION` uses `MAJOR.MINOR.PATCH`. One value is used by the Go backend, iOS
package, Rust workspace, desktop packages, embedded client metadata, and update
manifest.

Different app versions may connect when their compatibility generation and
wire marker agree. An iOS package suffix such as `0.15.5-4` is a package build
revision, not a Surf or protocol version.

## Compatibility generation

`COMPATIBILITY_VERSION` is a positive integer. It changes only when an older
peer cannot continue safely, such as after a mandatory authentication change or
an incompatible wire interpretation.

| Client and server | Result |
| --- | --- |
| Equal generations | Connect |
| Older client | Offer a verified embedded package for the server generation |
| Newer client | Require a server update |
| Missing or invalid generation | Reject the connection |

Optional commands, UI changes, diagnostics, performance changes, and additive
capabilities do not change the generation.

## Wire markers and capabilities

Generation 1 uses the published wire token `20260831-1`. Its supported range
contains that marker only. Binary media uses the separate `RBR1` header.

Optional behavior belongs in `caps`. Current examples include
`pointer-input`, `clock`, and `media-stats`. Adding a capability is
compatible. Removing one or changing its meaning is not.

If a later release supports several wire generations, the config response must
state an explicit minimum and maximum. App versions do not define that range.

## Core ABI

The C99 core ABI starts at 1. Rust verifies shared structure sizes against the
C build. Objective C uses the same headers.

Internal and additive changes leave the ABI unchanged. An incompatible layout
or calling convention change increments `SURF_CORE_ABI_VERSION` and updates
all bindings in the same revision.

The core ABI is a build boundary inside a client. It does not affect network
compatibility or trigger an iOS update.

## Embedded iOS package

A release backend may contain one iOS `.deb`. The build checks agreement
between:

1. `VERSION` and the version in the Debian package
2. `COMPATIBILITY_VERSION` and `X-Surf-Compatibility`
3. Embedded metadata, package length, and SHA 256
4. The version and generation compiled into the backend

The update offer and downloaded package therefore refer to the same artifact.

## Release

1. Set the app version.
2. Change compatibility only when older peers cannot continue safely.
3. Build and verify the universal iOS package.
4. Build the backend release with that package embedded.
5. Build the Linux development client from the same revision.
6. Run C sanitizers, Go tests, Rust tests, Clippy, session integration, package
   verification, and hardware checks.
7. Tag the tested revision.

A previous release can be built in another worktree.

```sh
git worktree add ../surf-v0.15.5 v0.15.5
make -C ../surf-v0.15.5 native-package
```

Rollback replaces the program in place. `SURF_HOME` contains data and must not
be deleted as part of a rollback.
