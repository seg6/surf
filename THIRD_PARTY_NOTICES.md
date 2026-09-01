# Third-party notices

## Deta Surf application icon

The native application icon is sourced from
`deta/surf/app/build/resources/prod/icon.png` at commit
`07969419e3d9bd3825c8b79f429d8384a1cf451b`, retrieved on 2026-08-26. The
unmodified source asset retained in `client/ios/Artwork` has SHA-256
`a7fe7713e95bb66a476e9f65cbff4fae4441c52dfd12f284548f321382e9f594`.
The packaged icon and launch-screen PNGs are resized/composited derivatives.
The modern opaque icons also use
`packages/icons/src/lib/assets/plane.png` from the same commit. Its unmodified
source has SHA-256
`c2eecccb48ad6a929549c9411ec6506653831fec7405122474e5c1f7e6d5eb30`.

Copyright 2025 Deta GmbH

Copyright 2026 Negative Entropy L.L.C

Licensed under the Apache License, Version 2.0. The full license is retained in
`client/ios/Artwork/DETA-SURF-LICENSE.txt` and bundled with the app.

Surf is not affiliated with or endorsed by Deta GmbH or Negative Entropy
L.L.C. Their names and trademarks are not used to imply sponsorship.

## Lucide icons

Browser interface glyphs use the Lucide icon font from `lucide-static` 1.34.0.
The package was retrieved from
<https://registry.npmjs.org/lucide-static/-/lucide-static-1.34.0.tgz>.
The downloaded package archive has SHA-256
`a265596d6ec6f1eec640872086a157e9a04e36387506ffefc81d7f9f0626cba8`.

Copyright 2026 Lucide Icons and Contributors. Licensed under the ISC License.
Some Lucide glyphs are derived from Feather Icons, copyright 2013–present
Cole Bemis, licensed under the MIT License. The copyright and permission
notices are reproduced in `client/ios/Artwork/LUCIDE-LICENSE.txt`.

## FFmpeg and ffmpeg-the-third

The Linux desktop client uses the system FFmpeg libraries through
`ffmpeg-the-third` 6.0.0+ffmpeg-9.0. `ffmpeg-the-third` is copyright its
contributors and is distributed under the WTFPL. FFmpeg is copyright the
FFmpeg developers and is normally distributed under LGPL 2.1 or later; the
exact license of a system FFmpeg build can differ based on enabled components.
Surf does not vendor or statically link an FFmpeg build.

The desktop audio host uses CPAL 0.18.2, copyright the RustAudio contributors,
under the Apache License 2.0.
