# Releasing

[Versioning](../reference/versioning.md) explains when to bump the app version
and when to bump the compatibility generation.

## Checklist

1. Set `VERSION`, and `COMPATIBILITY_VERSION` only if it has to change.
2. Update `CHANGELOG.md`.
3. Land the release commit on `main` and wait for Continuous Integration to
   pass.
4. Create a signed, annotated tag named `vMAJOR.MINOR.PATCH` on that commit.
5. Dispatch the Release workflow:

```sh
gh workflow run release.yml -f tag=vMAJOR.MINOR.PATCH
```

Release verifies the tag's signature, builds every host and client package from
that tag, publishes the GitHub release, and then queues Package Repository on
its own.

6. Package Repository builds the Astro site from the current `main`, attaches
   the `.deb` from the signed release, and pushes `gh-pages`, which holds the
   apt root and the docs.

If you only want to republish the site and docs against a release that's
already out, without building anything again:

```sh
gh workflow run repository.yml --ref main -f tag=vMAJOR.MINOR.PATCH
```

## Trust model

- Release and Package Repository both require a signed, annotated tag.
- Package Repository also requires a published GitHub release that isn't a
  draft.
- The site can be built from a newer `main` commit than the tag, as long as the
  tag is an ancestor of that commit.
- Continuous Integration never publishes anything.

## Rebuilding an earlier release

```sh
git worktree add ../surf-v0.15.5 v0.15.5
make -C ../surf-v0.15.5 native-package
```

## Rolling back

Replace the host program in place. Don't delete `SURF_HOME`, because it holds
the host's identity and pairings.

## The site and the apt repository

The static docs come from `site/`. `packaging/repository/build.sh` adds
`Packages`, `Release`, `debs/` and the icons next to it, so
`https://seg6.space/surf/` is still a valid apt root.
