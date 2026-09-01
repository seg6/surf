# ADR 0001: Portable C99 client core with platform-native hosts

- Status: Accepted
- Date: 2026-09-01

## Context

Surf's released client is a native Objective-C/UIKit application supporting
iOS 6 through iOS 14. It combines platform integration, protocol decoding,
browser state, input sequencing, media policy, and presentation coordination.
That combination delivered a responsive client on constrained hardware, but it
makes most client behavior testable only by building or running the iOS app.

Surf also needs a genuine desktop client for development and eventual client
hosts on Android, old Windows, and other legacy systems. Sharing a visual
toolkit would either exclude old iOS or force every platform into a lowest-
common-denominator interface. The reusable boundary must therefore be client
behavior rather than widgets.

## Decision

Surf will use a conservative C99 library at `client/core` as the portable
client core. It will expose a narrow C ABI and contain:

- typed control-command and event codecs;
- the binary media-frame parser;
- session, compatibility, and capability state;
- browser/tab state and deterministic transitions;
- interaction and input sequencing;
- media admission, generation, gap, and recovery policy; and
- portable diagnostics calculations.

The core will not import a platform UI, networking, TLS, storage, decoder,
renderer, audio, clipboard, or file API. It will not create threads. Platform
hosts serialize events into the core, drain requested effects, execute those
effects using platform APIs, and return results as new events.

The first desktop host will live at `client/desktop` and use Rust with Dear
ImGui, winit, and glutin.
Rust will provide asynchronous platform orchestration, pinned TLS, secure
pairing, FFmpeg integration, audio, and a custom OpenGL video surface. A raw
FFI crate and a safe ownership wrapper will isolate Rust from the C ABI.

The iOS host will ultimately live at `client/ios` and retain Objective-C,
UIKit, Keychain/Security, the existing socket/TLS compatibility work,
VideoToolbox, AudioQueue, and the stable presentation surfaces required by old
iOS. It will adopt the core incrementally before its directory is moved.

## Execution constraints

1. The released iOS client must remain buildable at every commit.
2. The armv7/iOS 6 and arm64/iOS 7 deployment targets remain verified.
3. High-rate media does not pass through the UI/control reducer.
4. H.264 access units are not copied merely to cross the core boundary.
5. A platform host owns all asynchronous operations and core-thread affinity.
6. Core snapshots are semantic; they contain no layout coordinates, colors,
   fonts, or widget types.
7. Production authentication and certificate pinning have no test bypass.
8. Wire generation, additive capabilities, app version, and core ABI are
   independent concepts.

## Consequences

Positive consequences:

- Client behavior can run under ordinary Linux unit tests, sanitizers, and
  fuzzers.
- UIKit and Dear ImGui can express interfaces appropriate to their roles while
  sharing the same behavioral source of truth.
- New hosts can consume a small C ABI without inheriting Rust, Qt, UIKit, or a
  language runtime.
- Protocol drift becomes detectable through Go/C contract fixtures.
- Old iOS retains its optimized decoder and renderer.

Costs and risks:

- C ownership rules require unusually strict tests and documentation.
- A temporary migration period will contain both old Objective-C state and the
  new core model.
- The Rust desktop host remains responsible for a substantial amount of real
  platform work; the portable core is not a universal application framework.
- Hardware decoders, TLS stores, IME behavior, and graphics drivers still need
  platform-specific verification.

## Rejected alternatives

### Put the core in Rust

Rejected because the core must compile with the old iOS SDK/toolchain and be
easy to embed on legacy platforms without introducing a Rust runtime or
toolchain requirement. Rust remains an excellent host language on desktop.

### Share one cross-platform UI

Rejected because UIKit is integral to the supported old-iOS experience and a
single widget toolkit would constrain either legacy compatibility or native
platform behavior. Surf shares semantics rather than pixels.

### Rewrite the iOS client at once

Rejected because it would combine architectural, protocol, media, lifecycle,
and directory-layout changes into one unreviewable transition. Surf will use a
strangler migration with verified checkpoints.
