#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The image the browser tests of CI run in (.github/images/playwright/Dockerfile): its name, and whether the registry
# has it. The name holds what the image is made of, the Playwright of web/e2e/package.json, the Bun of .tool-versions,
# and the Dockerfile, so a change of one of them is another image and nothing has to be kept in step by hand.
#
#   scripts/browser-image.sh ref     print the image's name
#   scripts/browser-image.sh find    print the name when the registry has the image, and nothing when it has not
#   scripts/browser-image.sh ensure  as find; when the registry has none, build the image and push it, and print the
#                                    name. Print nothing when the push is refused (a pull request from a fork or
#                                    from Dependabot may only read)
#
# A job that gets no name installs the browser on the runner instead, as before the image: the image is a cache, and
# a miss is slower, not a failure. find and ensure need `docker login ghcr.io` first.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dockerfile="$root/.github/images/playwright/Dockerfile"
image="${BROWSER_IMAGE:-ghcr.io/mycroblog/my-playwright}"

playwright="$(jq -er '.devDependencies["@playwright/test"] // .dependencies["@playwright/test"]' "$root/web/e2e/package.json")"
bun="$(awk '$1 == "bun" { print $2 }' "$root/.tool-versions")"
if ! [[ "$playwright" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || [ -z "$bun" ]; then
  echo "browser-image.sh: web/e2e/package.json must pin @playwright/test to an exact version (it has '$playwright'), and .tool-versions must have bun (it has '$bun')" >&2
  exit 1
fi
ref="$image:$playwright-bun$bun-$(sha256sum "$dockerfile" | cut -c1-8)"

found() { docker manifest inspect "$ref" > /dev/null 2>&1; }

case "${1:-}" in
  ref)
    echo "$ref"
    ;;
  find)
    if found; then echo "$ref"; fi
    ;;
  ensure)
    if found; then
      echo "$ref"
      exit 0
    fi
    echo "browser-image.sh: the registry has no $ref: building it" >&2
    docker build --build-arg "PLAYWRIGHT_VERSION=$playwright" --build-arg "BUN_VERSION=$bun" \
      -t "$ref" -f "$dockerfile" "$(dirname "$dockerfile")" >&2
    if docker push "$ref" >&2; then
      echo "$ref"
    else
      echo "browser-image.sh: the registry refused the push of $ref (a token that may only read): the jobs install the browser on the runner instead" >&2
    fi
    ;;
  *)
    sed -n '8,12p' "$0" | sed 's/^# \{0,1\}//' >&2
    exit 2
    ;;
esac
