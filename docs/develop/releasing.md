# Releasing

Follow [Versioning](../reference/versioning.md) for when to bump app version
vs compatibility generation.

## Checklist

1. Set `VERSION` (and `COMPATIBILITY_VERSION` only if required).
2. Update `CHANGELOG.md`.
3. Land the release commit on `main` and wait for Continuous Integration.
4. Create a signed annotated tag `vMAJOR.MINOR.PATCH` on that commit.
5. Dispatch Release:

```sh
gh workflow run release.yml -f tag=vMAJOR.MINOR.PATCH
```

Release verifies the annotated tag signature, builds every host and client
package from that tag, publishes the GitHub release, then queues Package
Repository automatically.

6. Package Repository builds the Astro site from current `main`, attaches the
   signed release's `.deb`, and pushes `gh-pages` (apt root + docs).

To republish the site/docs against an already-published release without a new
build:

```sh
gh workflow run repository.yml --ref main -f tag=vMAJOR.MINOR.PATCH
```

## Trust model

- Release and Package Repository require a signed annotated tag.
- Package Repository also requires a published, non-draft GitHub release.
- Site content may come from a newer `main` commit than the tag, as long as
  the tag is an ancestor of that commit.
- Continuous Integration never publishes.

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
