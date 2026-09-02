# Surf iOS client

This directory contains the Objective C and UIKit host for the shared C99
client core. The rootful package includes an armv7 build for iOS 6 and an arm64
build for iOS 7 and later.

UIKit handles presentation and lifecycle. Platform code handles Keychain
storage, pinned TLS, WebSocket transport, VideoToolbox and OpenGL video,
AudioQueue output, files, clipboard, and input. The C99 core handles the wire
formats and shared browser, connection, input, and media state.

Build and verify the package from the repository root.

```sh
make native-package
```

Packages are written to `client/ios/packages/`. The build generates
`client/ios/control` and `client/ios/Resources/Info.plist` from their
templates.

See [Native Build](../../docs/native-build.md) for the toolchain and device
checks.
