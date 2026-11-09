#!/usr/bin/env bash
# Verify a GitHub annotated tag is signed and points at a commit.
# Usage: require-signed-tag.sh TAG
# Prints the target commit SHA on success.
set -euo pipefail

tag="${1:-}"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || {
  echo "invalid release tag: ${tag:-<empty>}" >&2
  exit 1
}

: "${GH_REPO:?GH_REPO is required}"
command -v gh >/dev/null
command -v jq >/dev/null

tag_ref="$(gh api "repos/$GH_REPO/git/ref/tags/$tag")"
tag_object_type="$(jq -r '.object.type' <<< "$tag_ref")"
tag_object_sha="$(jq -r '.object.sha' <<< "$tag_ref")"
test "$tag_object_type" = "tag" || {
  echo "$tag is not an annotated tag" >&2
  exit 1
}

tag_json="$(gh api "repos/$GH_REPO/git/tags/$tag_object_sha")"
tag_name="$(jq -r '.tag' <<< "$tag_json")"
tag_target_type="$(jq -r '.object.type' <<< "$tag_json")"
tag_commit="$(jq -r '.object.sha' <<< "$tag_json")"
tag_verified="$(jq -r '.verification.verified' <<< "$tag_json")"
tag_reason="$(jq -r '.verification.reason' <<< "$tag_json")"
test "$tag_name" = "$tag"
test "$tag_target_type" = "commit" || {
  echo "$tag does not point directly to a commit" >&2
  exit 1
}
test "$tag_verified" = "true" || {
  echo "$tag signature is not verified: $tag_reason" >&2
  exit 1
}

printf '%s\n' "$tag_commit"
