# Surf Client Core

`surf_client_core` is Surf's platform-neutral C99 client library. It is built
without UIKit, Foundation, Qt, Rust, networking, TLS, decoder, renderer, or
audio dependencies.

The core owns the binary media envelope, H.264 Annex-B helpers, the complete
typed control-event decoder and command encoder, deterministic browser state,
immutable navigation and rich-semantic snapshots, modal completion, platform
effects, reconnect/media/input policy, NTP-style clock synchronization, and
rolling pipeline diagnostics. The JSON decoder is strict and allocation-free
after a host supplies one reusable workspace; decoded strings and collections
remain valid until that workspace is reused.

Build and test it directly:

```sh
cmake -S client/core -B .local/build/client-core \
  -DCMAKE_BUILD_TYPE=Debug -DSURF_CORE_WARNINGS_AS_ERRORS=ON
cmake --build .local/build/client-core
ctest --test-dir .local/build/client-core --output-on-failure
```

Or run:

```sh
make client-core-test
make client-core-sanitize
```

Public interfaces live under `include/surf`. Large media payloads remain owned
by the platform host; parsers return borrowed views and never copy access units.
See [Porting Surf Clients](../../docs/porting-clients.md) for the complete
adapter and threading contract.
