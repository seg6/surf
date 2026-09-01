# Surf Client Core

`surf_client_core` is Surf's platform-neutral C99 client library. It is built
without UIKit, Foundation, Qt, Rust, networking, TLS, decoder, renderer, or
audio dependencies.

The current slice owns the binary media envelope, H.264 Annex-B helpers, typed
browser events, deterministic tab/navigation/editable state, immutable
snapshots, and platform effects. As the migration proceeds it will add the
remaining control messages, session state, interaction sequencing, media
policy, and portable diagnostics.

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
