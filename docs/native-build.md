# Native Build

Most users should download the `.deb` from GitHub Releases. Build the native
client only if you are changing the app or packaging your own release.

## SDK

The iOS 6.1 SDK is downloaded into this ignored local path:

```text
native/buildenv/sdk/iPhoneOS6.1.sdk
```

Fetch it with:

```sh
native/buildenv/fetch-sdk.sh
```

The default source is:

```text
https://github.com/GrowtopiaJaw/iPhoneOS-SDK/releases/download/v1.0/iPhoneOS6.1.sdk.zip
```

The default archive must match this SHA-256:

```text
2696df17fc48e1b6ea3f7acd346b5f2356fb5c6cc60b0f3aaca0c24522d761de
```

Set `SDK_URL` and `SDK_SHA256` together to use a different archive.

## Build Environment

```sh
docker build -t surf-buildenv native/buildenv
docker run --rm -v "$PWD:/src" surf-buildenv make -C /src/native/client package DEBUG=0
```

The build environment pins Theos to commit
`16362d3aa83a0acd56df4493d575d34306d42478` and the iOS toolchain to release
`test-210562a`, with SHA-256 verification for each host-architecture archive.

Or use the root make target:

```sh
make native-package
```

`make native-package` renders generated native metadata from `VERSION` and
compiles the native protocol gate from `PROTOCOL_VERSION`. Do not edit generated
`native/client/control` or `native/client/Resources/Info.plist` by hand.

The package is written to:

```text
native/client/packages/
```

## Verify

```sh
docker run --rm -v "$PWD:/src" surf-buildenv lipo -info /src/native/client/.theos/obj/Surf
docker run --rm -v "$PWD:/src" surf-buildenv dpkg-deb -c /src/native/client/packages/*.deb
```
