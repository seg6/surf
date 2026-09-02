# Surf client core

`surf_client_core` is the C99 library shared by Surf clients. It has no UI,
network, TLS, decoder, renderer, audio, filesystem, thread, or Rust dependency.

The core handles control events and commands, browser state, connection state,
input ordering, media admission, clock synchronization, and pipeline metrics.
Its JSON decoder uses a workspace supplied by the host. Decoded strings and
collections remain valid until that workspace is reused.

Build and test the library with:

```sh
cmake -S client/core -B .local/build/client-core \
  -DCMAKE_BUILD_TYPE=Debug -DSURF_CORE_WARNINGS_AS_ERRORS=ON
cmake --build .local/build/client-core
ctest --test-dir .local/build/client-core --output-on-failure
```

The root make targets run the same tests and the sanitizer build.

```sh
make client-core-test
make client-core-sanitize
```

Public headers live under `include/surf`. Media parsers return borrowed views,
so the host retains ownership of the source buffer.

See [Porting Surf Clients](../../docs/porting-clients.md) for the adapter and
threading contract.
