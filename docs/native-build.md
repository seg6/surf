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

Set `SDK_URL` to use a different archive.

## Build Environment

```sh
docker build -t surf-buildenv native/buildenv
docker run --rm -v "$PWD:/src" surf-buildenv make -C /src/native/client package DEBUG=0
```

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
