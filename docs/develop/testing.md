# Testing

## The default suite

```sh
make test
```

This runs the `client/core` CMake tests, `go test ./...` under `backend/`, and
the desktop Rust workspace tests.

## Backend and Chromium

Some of the download and browser tests do a real round trip through Chromium,
using a temporary profile and local HTTP fixtures. They use whichever Chrome or
Chromium is installed, and you can point them at a specific one with
`SURF_TEST_BROWSER`. If there's no Chromium, they skip when you run them
locally, but they fail in CI.

## Client core

```sh
make client-core-test
make client-core-sanitize
```

## Desktop client

```sh
make desktop-test
client/desktop/test-secure-session.sh
bash client/desktop/test-ui-input.sh
```

The session tests need a working display, GL and audio output. CI gets those
from Xvfb and a PulseAudio null sink. `client/desktop/README.md` has the
details.

## Protocol fixtures

The fixtures in `protocol/fixtures/` run valid and invalid commands and events
through both languages. If you change the protocol, keep the Go and C codecs in
sync.

## Hardware

The checks to run on a real iOS device before a release are listed in
[Clients: iOS](../clients/ios.md), covering phone and tablet layouts, media,
pairing and appearance. CI can't stand in for that pass.
