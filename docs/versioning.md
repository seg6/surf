# Versioning and Compatibility

Surf has several version dimensions because a backend, an installed client,
an embedded client package, and the portable core can evolve independently.
Treating all of them as one number caused unnecessary update prompts in the
past. The rules below are the release contract.

## The five dimensions

| Dimension | Source of truth | Purpose |
| --- | --- | --- |
| App version | `VERSION` | The user-facing Surf release, shared by the backend and packaged clients |
| Compatibility generation | `COMPATIBILITY_VERSION` | An ordered safety gate for client/backend combinations that cannot interoperate |
| Wire marker/range | `WireCompatibilityVersion` helpers and the `RBR1` header | The control and binary envelopes a component understands |
| Capability set | `backend/internal/config/config.go` | Independently negotiated, additive features |
| Core ABI | `SURF_CORE_ABI_VERSION` in `client/core/include/surf/core.h` | The native host-to-C binary interface |

A future deterministic trace format must have its own schema number inside the
trace. Logs are not protocol traces, and no trace version currently participates
in connection compatibility.

## App versions

`VERSION` uses `MAJOR.MINOR.PATCH`. One release revision supplies this value to
the Go backend, iOS package metadata and UI, Rust workspace, desktop archives,
embedded client metadata, and updater manifests.

An app-version difference is not a compatibility failure. A `0.15.4` client
and `0.15.5` server may connect when their compatibility generation and wire
marker agree. The server may still offer the newer client as a normal update.

The suffix after an iOS Debian version, such as `0.15.5-4`, is a package build
revision. It distinguishes repeated local package builds; it does not change
Surf's app version or protocol compatibility.

## Compatibility generations

`COMPATIBILITY_VERSION` is a positive, ordered integer. Increment it only when
continuing with the older peer would be impossible or unsafe—for example, a
mandatory authentication change or a breaking interpretation that cannot be
negotiated.

- Equal generations connect.
- An older client receives `client-update-required` only when the server has a
  verified embedded package for the server's exact app version and generation.
- A newer client receives `server-update-required`.
- Missing or invalid generation metadata fails closed; it is never inferred
  from the app version.

Do not increment the generation for an optional command, UI change, new
diagnostic, performance improvement, or additive field guarded by a capability.

## Wire markers and capabilities

The current deployment exposes one wire marker for each compatibility
generation. Generation 1 keeps the published legacy token `20260831-1`; its
effective supported range is therefore that single marker. Binary media uses
the independently validated `RBR1` header and its fixed header length.

New optional behavior belongs in `caps`. A client uses a feature only when it
understands it and the server advertises it. `pointer-input`, `clock`, and
`media-stats` are examples. Removing or redefining an existing capability is a
breaking change; adding one is not.

If Surf later supports multiple breaking wire generations at once, the config
response should expose explicit minimum and maximum wire generations. That
range must not be derived from `VERSION`.

## Portable core ABI

The C99 core ABI starts at 1. Rust checks the sizes of every shared structure
against functions compiled by the C compiler, while Objective-C builds the same
headers directly. Additive implementation changes do not bump the ABI. An
incompatible host-visible layout or calling-contract change must bump
`SURF_CORE_ABI_VERSION` and update every binding in the same commit.

The core ABI is an in-process build boundary, not a client/backend network
version. Released hosts statically build the matching core, so an ABI bump alone
must never trigger a device update prompt.

## Embedded iOS package

A release backend may embed one signed iOS `.deb`. The build rejects the bundle
unless all of these agree:

1. root `VERSION` and the Surf-version prefix of the Debian package version;
2. root `COMPATIBILITY_VERSION` and `X-Surf-Compatibility`;
3. generated embedded metadata and the package SHA-256/size;
4. the release backend's injected version and compatibility generation.

This makes the backend's update offer evidence-based: the displayed version,
downloaded bytes, and compatibility decision describe the same artifact.

## Release and rollback procedure

For a normal release:

1. Choose the app version; change compatibility only for a real breaking gate.
2. Build and verify the universal iOS package.
3. Build the backend release set with that exact `.deb` embedded.
4. Build the Linux desktop preview from the same Git revision.
5. Run C sanitizers, Go tests, Rust tests/Clippy, secure-session integration,
   package verification, and physical-device acceptance.
6. Tag only that verified revision.

To reproduce or roll back without disturbing current work, use a separate Git
worktree:

```sh
git worktree add ../surf-v0.15.5 v0.15.5
make -C ../surf-v0.15.5 native-package
```

Do not reset a working tree or delete `SURF_HOME` to roll back. Server profiles,
paired-device records, browser state, and client keys are data, not release
artifacts. Installing a compatible older binary in place preserves them.
