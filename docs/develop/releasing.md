# Releasing

Follow [Versioning](../reference/versioning.md) for when to bump app version
vs compatibility generation.

## Checklist

1. Set `VERSION` (and `COMPATIBILITY_VERSION` only if required).
2. Update `CHANGELOG.md`.
3. Build and verify the universal iOS package
   ([native-ios](native-ios.md)).
4. Build the host release with that package embedded (`CLIENT_DEB=...`).
5. Build the Linux desktop client from the same revision.
6. Run C sanitizers, Go tests, Rust tests, Clippy, session integration, package
   verification, and hardware checks as applicable.
7. Create a signed annotated tag `vMAJOR.MINOR.PATCH`.
8. Publish GitHub release assets via the release workflow.
9. Publish the Cydia/Sileo repository via the repository workflow (points at
   that tag), which builds the Astro site and apt metadata together.

## Prior release rebuild

```sh
git worktree add ../surf-v0.15.5 v0.15.5
make -C ../surf-v0.15.5 native-package
```

## Rollback

Replace the host program in place. Do **not** delete `SURF_HOME`; it holds
identity and pairings.

## Site and apt

`site/` -> static docs. `packaging/repository/build.sh` adds `Packages`,
`Release`, `debs/`, and icons beside it so `https://seg6.space/surf/` stays a
valid apt root.
