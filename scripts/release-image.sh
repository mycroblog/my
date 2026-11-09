#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The steps of publishing the container image (docs/agents/releasing.md, "The container image"): release.yml pushes
# the image by digest for the draft, and release-image.yml tags that digest when the release is published.
#
#   scripts/release-image.sh tags VERSION [--roll-back]     the image tags of VERSION, one per line, given the image tags
#                                                           that exist on stdin: X.Y.Z, and X.Y when VERSION is the
#                                                           latest patch of X.Y (or with --roll-back, which moves X.Y
#                                                           back to it); a pre-release has its own tag alone
#   scripts/release-image.sh series                         the X.Y tags to scan, given the image tags on stdin: those of
#                                                           the two latest minor versions, which operators upgrade through
#   scripts/release-image.sh metadata VERSION SHA CREATED [REF]
#                                                           the OCI labels and annotations of the image, as key=value lines;
#                                                           its documentation is at REF (default VERSION, a tag)
#   scripts/release-image.sh nightly-version LATEST SHA TIME
#                                                           the version of a nightly image of commit SHA made at TIME
#                                                           (YYYYMMDDhhmmss, UTC), after the release tag LATEST (empty when
#                                                           none): a pseudo-version, as Go's, that sorts after LATEST and
#                                                           before the next release
#   scripts/release-image.sh list-tags                      the tags of the image on the registry (REGISTRY_TOKEN when it
#                                                           is not public); nothing when anonymous and not public
#   scripts/release-image.sh digest REF                     the digest of the index that REF names
#   scripts/release-image.sh tag DIGEST TAG                 gives the image DIGEST the tag TAG and checks the digest;
#                                                           fails when X.Y.Z exists on another digest
#   scripts/release-image.sh check DIGEST PLATFORM VERSION  runs the image's binary (on the runner's platform) and checks
#                                                           it with scripts/check-release-build.sh; needs go
#   scripts/release-image.sh scan [--fail] REF              reports the vulnerabilities of both platforms of REF with
#                                                           Grype, which gets no credentials; --fail: fixable HIGH or above fail
#
# IMAGE overrides the image's name, to try the commands on another registry. scripts/release-image-test.sh tests them.
set -euo pipefail

image="${IMAGE:-ghcr.io/mycroblog/my}"
root="$(cd "$(dirname "$0")/.." && pwd)"
# Pinned by digest and raised by hand; Dependabot reads only the images of Dockerfiles.
grype_image="anchore/grype:v0.121.0@sha256:621d56d1d68106b787022c81546f7e0a36202a25d1239f0c20195fab8c16f7ea"
platforms=(linux/amd64 linux/arm64)
semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$'

die() {
  echo "release-image: $*" >&2
  exit 1
}

usage() {
  sed -n '/^#   scripts/p' "$0" >&2
  exit 2
}

# check_version VERSION sets major, minor, patch, and pre.
check_version() {
  [[ "$1" =~ $semver ]] || die "the version must be vX.Y.Z or vX.Y.Z-<pre-release>: $1"
  major="${BASH_REMATCH[1]}" minor="${BASH_REMATCH[2]}" patch="${BASH_REMATCH[3]}" pre="${BASH_REMATCH[4]}"
}

check_digest() {
  [[ "$1" =~ ^sha256:[0-9a-f]{64}$ ]] || die "not a digest: $1"
}

tags() {
  local version="${1:-}" roll_back="${2:-}"
  [[ -n "$version" && ($# -eq 1 || ($# -eq 2 && "$roll_back" == --roll-back)) ]] || usage
  check_version "$version"
  if [[ -n "$pre" && -n "$roll_back" ]]; then
    die "$version is a pre-release, which has no X.Y tag to roll back"
  fi
  echo "${version#v}"
  if [[ -n "$pre" ]]; then
    return
  fi
  local tag latest=1
  while IFS= read -r tag; do
    [[ "$tag" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || continue
    if ((BASH_REMATCH[1] == major && BASH_REMATCH[2] == minor && BASH_REMATCH[3] > patch)); then
      latest=0
    fi
  done
  if ((latest == 1)) || [[ -n "$roll_back" ]]; then
    echo "$major.$minor"
  fi
}

series() {
  { grep -E '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || true; } | sort -t. -k1,1nr -k2,2nr | head -n 2
}

metadata() {
  (($# == 3 || $# == 4)) || usage
  local version="$1" sha="$2" created="$3" ref="${4:-$1}" base
  check_version "$version"
  base="$(sed -n 's/^FROM \([^ ]*\)@\(sha256:[0-9a-f]*\) AS app$/\1 \2/p' "$root/Dockerfile")"
  [[ "$base" == *" sha256:"* ]] || die "the Dockerfile's app stage has no base image pinned by digest"
  cat <<META
org.opencontainers.image.title=my
org.opencontainers.image.description=An ActivityPub-compatible microblog server.
org.opencontainers.image.url=https://github.com/mycroblog/my
org.opencontainers.image.source=https://github.com/mycroblog/my
org.opencontainers.image.documentation=https://github.com/mycroblog/my/blob/$ref/README.md
org.opencontainers.image.version=${version#v}
org.opencontainers.image.revision=$sha
org.opencontainers.image.created=$created
org.opencontainers.image.licenses=AGPL-3.0-only
org.opencontainers.image.base.name=${base% *}
org.opencontainers.image.base.digest=${base#* }
META
}

nightly_version() {
  (($# == 3)) || usage
  local latest="$1" sha="$2" time="$3"
  [[ "$sha" =~ ^[0-9a-f]{12,40}$ ]] || die "not a commit: $sha"
  [[ "$time" =~ ^[0-9]{14}$ ]] || die "the time must be YYYYMMDDhhmmss: $time"
  local tail="0.$time.g${sha:0:12}"
  if [[ -z "$latest" ]]; then
    echo "v0.0.0-$tail"
    return
  fi
  check_version "$latest"
  # After a pre-release, a pre-release of the same version that sorts after it; after a release, a pre-release of the
  # next patch.
  if [[ -n "$pre" ]]; then
    echo "$latest.$tail"
  else
    echo "v$major.$minor.$((patch + 1))-$tail"
  fi
}

list_tags() {
  (($# == 0)) || usage
  local host="${image%%/*}" path="${image#*/}" auth=() token body status
  if [[ -n "${REGISTRY_TOKEN:-}" ]]; then auth=(-u "token:$REGISTRY_TOKEN"); fi
  # The registry refuses a package that does not exist or is not public; with a token, that is an error.
  if ! token="$(curl -fsS ${auth[@]+"${auth[@]}"} "https://$host/token?service=$host&scope=repository:$path:pull" 2>/dev/null | jq -er .token)"; then
    [[ -z "${REGISTRY_TOKEN:-}" ]] || die "the registry refused a token for $image"
    return 0
  fi
  body="$(mktemp)"
  status="$(curl -sS -o "$body" -w '%{http_code}' -H "Authorization: Bearer $token" "https://$host/v2/$path/tags/list?n=10000")"
  case "$status" in
    200) jq -r '.tags // [] | .[]' "$body" ;;
    401 | 403 | 404) ;;
    *)
      rm -f "$body"
      die "listing the tags of $image answered $status"
      ;;
  esac
  rm -f "$body"
}

digest() {
  (($# == 1)) || usage
  docker buildx imagetools inspect "$1" --format '{{json .Manifest}}' | jq -er .digest
}

tag() {
  (($# == 2)) || usage
  local want="$1" name="$2" have
  check_digest "$want"
  have="$(digest "$image:$name" 2>/dev/null || true)"
  if [[ "$have" == "$want" ]]; then
    echo "$image:$name is already at $want."
    return
  fi
  # A version's tag is never moved; only X.Y is.
  if [[ -n "$have" && "$name" == *.*.* ]]; then
    die "$image:$name exists at $have, not $want: a version's tag is never moved"
  fi
  docker buildx imagetools create --tag "$image:$name" "$image@$want"
  have="$(digest "$image:$name")"
  [[ "$have" == "$want" ]] || die "$image:$name is at $have after tagging, not $want"
  echo "$image:$name is at $want."
}

check() {
  (($# == 3)) || usage
  local want="$1" platform="$2" version="$3" ref="$image@$1" dir id out
  check_digest "$want"
  check_version "$version"
  docker pull -q --platform "$platform" "$ref" >/dev/null
  dir="$(mktemp -d)"
  id="$(docker create --platform "$platform" "$ref")"
  docker cp -q "$id:/usr/local/bin/my" "$dir/my"
  docker rm "$id" >/dev/null
  "$root/scripts/check-release-build.sh" "$dir/my"
  rm -rf "$dir"
  out="$(docker run --rm --platform "$platform" "$ref" version)"
  [[ "$out" == "my $version" ]] || die "$ref ($platform) reports '$out', not 'my $version'"
  echo "$ref ($platform): $out, not a test build."
}

scan() {
  local fail=() status=0 dir platform result
  if [[ "${1:-}" == --fail ]]; then
    fail=(--only-fixed --fail-on high)
    shift
  fi
  (($# == 1)) || usage
  local ref="$1" summary="${GITHUB_STEP_SUMMARY:-/dev/null}"
  dir="$(mktemp -d)"
  chmod 0755 "$dir"
  for platform in "${platforms[@]}"; do
    # Grype reads a file: the registry's credentials stay with this script's docker, and the container has only the image.
    docker pull -q --platform "$platform" "$ref" >/dev/null
    docker save -o "$dir/image.tar" "$ref"
    chmod 0644 "$dir/image.tar"
    docker rmi "$ref" >/dev/null
    result=0
    docker run --rm --security-opt no-new-privileges -e GRYPE_CHECK_FOR_APP_UPDATE=false \
      -v "$dir/image.tar:/image.tar:ro" "$grype_image" ${fail[@]+"${fail[@]}"} docker-archive:/image.tar >"$dir/report.txt" || result=$?
    rm -f "$dir/image.tar"
    {
      echo "### $ref ($platform)"
      echo '```'
      cat "$dir/report.txt"
      echo '```'
    } | tee -a "$summary"
    if ((result != 0)); then status=1; fi
  done
  rm -rf "$dir"
  return "$status"
}

cmd="${1:-}"
if (($# > 0)); then shift; fi
case "$cmd" in
  tags) tags "$@" ;;
  series) series "$@" ;;
  metadata) metadata "$@" ;;
  nightly-version) nightly_version "$@" ;;
  list-tags) list_tags "$@" ;;
  digest) digest "$@" ;;
  tag) tag "$@" ;;
  check) check "$@" ;;
  scan) scan "$@" ;;
  *) usage ;;
esac
