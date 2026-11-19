# Surf iOS client

This directory holds the Objective-C and UIKit host for the shared C99 client
core. The rootful package has an armv7 build for iOS 6 and an arm64 build for
iOS 7 and later.

UIKit takes care of presentation and the app lifecycle. The platform code
handles Keychain storage, pinned TLS, WebSocket transport, VideoToolbox and
OpenGL video, AudioQueue output, files, the clipboard and input. The C99 core
handles the wire formats and the browser, connection, input and media state
that the clients share.

To build and verify the package, run this from the repository root:

```sh
make native-package
```

Packages are written to `client/ios/packages/`. The build generates
`client/ios/control` and `client/ios/Resources/Info.plist` from their
templates.

The [native build](../../docs/develop/native-ios.md) doc covers the toolchain
and the device checks.
