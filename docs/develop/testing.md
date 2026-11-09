# Testing

## Default suite

```sh
make test
```

`client/core` CMake tests, `go test ./...` under `backend/`, desktop Rust
workspace tests.

## Backend / Chromium

Download/browser tests can do a real Chromium round trip (temp profile, local
HTTP fixtures). They pick an installed Chrome/Chromium; override with
`SURF_TEST_BROWSER`. Without Chromium they skip locally and **fail in CI**.

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

Session tests need a working display (Xvfb in CI), GL, and audio output (CI
uses a PulseAudio null sink). See `client/desktop/README.md`.

## Protocol fixtures

Cross-language fixtures under `protocol/fixtures/` exercise valid and invalid
commands/events. Keep Go and C codecs in sync when changing the contract.

## Hardware

iOS release checks on device are listed in [Clients: iOS](../clients/ios.md)
(phone/tablet layouts, media, pairing, appearance). Automated CI does not
replace that pass.
