# Surf iOS Client

This directory is the Objective-C/UIKit host for Surf's portable C99 client
core. It deliberately supports both armv7/iOS 6 and arm64/iOS 7 deployment
targets from one rootful package.

UIKit owns presentation, lifecycle, Keychain/Security integration, pinned TLS,
the hand-rolled old-iOS WebSocket adapter, VideoToolbox/OpenGL rendering,
AudioQueue output, files, clipboard, and device-specific input. `client/core`
owns the bounded wire formats, deterministic navigation/editable state,
connection epochs, input/media admission policy, and shared diagnostics.

Build and verify the universal package from the repository root:

```sh
make native-package
```

Artifacts are written under `client/ios/packages/`; `.theos/`, generated
`control`, and generated `Resources/Info.plist` are intentionally ignored or
verified against their templates. See `docs/native-build.md` for the pinned
SDK/toolchain setup and physical-device acceptance matrix.
