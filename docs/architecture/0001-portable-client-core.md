# ADR 0001: Portable C99 client core

Status: Accepted

Date: 2026-09-01

## Context

The iOS client supports iOS 6 through iOS 14. It previously combined platform
integration, protocol parsing, browser state, input ordering, media policy, and
presentation coordination. Most behavior could only be tested through the iOS
build.

A desktop client and possible Android or old Windows clients need the same
behavior. A shared widget toolkit would either drop old iOS or constrain every
platform. The common boundary is browser behavior, not UI.

## Decision

`client/core` is a C99 library with a narrow C ABI. It contains:

1. Control event and command codecs
2. The media frame parser
3. Session, compatibility, and capability state
4. Browser and tab state
5. Input ordering
6. Media admission and recovery policy
7. Diagnostics calculations

The library has no UI, network, TLS, storage, decoder, renderer, audio,
clipboard, filesystem, or thread dependency. Hosts dispatch events, read
snapshots and effects, perform platform work, and return results as new events.

The desktop host uses Rust, Dear ImGui, winit, glutin, FFmpeg, CPAL, and OpenGL.
Rust bindings isolate the host from the raw C ABI.

The iOS host keeps Objective C, UIKit, Security, Keychain, its transport code,
VideoToolbox, OpenGL, and AudioQueue.

## Constraints

1. The iOS client remains buildable during migration.
2. armv7 on iOS 6 and arm64 on iOS 7 remain build targets.
3. Media does not pass through the UI reducer.
4. Crossing the core boundary does not require copying H.264 access units.
5. Hosts perform asynchronous work and keep core calls on one owner.
6. Snapshots contain meaning, not layout, colors, fonts, or widget types.
7. Authentication and certificate pinning have no production bypass.
8. App versions, wire generations, capabilities, and the core ABI remain
   separate.

## Results

Client behavior can run in Linux tests, sanitizers, and fuzzers. UIKit and Dear
ImGui can use different interfaces over the same state rules. New hosts need
only the C ABI. Go and C fixture tests catch protocol drift. Old iOS keeps its
existing decoder and renderer.

The cost is another API boundary with strict ownership rules. Platform
differences in TLS, decoding, graphics, input methods, and lifecycle still need
platform tests.

## Alternatives

### Rust core

Rejected because the core must build with the old iOS toolchain without adding
a Rust toolchain to the client build. Rust remains in the desktop host.

### Shared UI toolkit

Rejected because old iOS depends on UIKit and each platform needs native input,
lifecycle, and accessibility behavior.

### Immediate iOS rewrite

Rejected because it would combine protocol, state, media, lifecycle, and source
layout changes in one migration.
