#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# The examples of deploy/opentofu name their modules by Git source at a release, so that they work when copied out of the
# repository and no module changes under an operator. deploy/opentofu/MODULE_REF is that release, the one place it is kept.
#
#   scripts/tofu-examples.sh check [--release VERSION]
#       Fails when a source in an example or a document is relative, is not pinned, or has another ref. With --release, also
#       when a stable VERSION is not MODULE_REF; a prerelease is not checked.
#   scripts/tofu-examples.sh bump VERSION
#       Sets MODULE_REF and every source to VERSION.
#   scripts/tofu-examples.sh test
#       Runs validate and the tests of each example on a temporary copy whose sources are the local modules of this commit.
#   scripts/tofu-examples.sh lock
#       Updates each example's lock file, the same way.
#   scripts/tofu-examples.sh git-source-test
#       Proves the Git form: copies an example to an empty directory, as an operator does, and runs tofu init and validate
#       against a local bare repository with the tag, with no network to GitHub. The scripts are then under .terraform/modules.
#
# TOFU_EXAMPLES_ROOT overrides the repository root, for the tests of this script.
set -euo pipefail

root="${TOFU_EXAMPLES_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
tofu_dir="$root/deploy/opentofu"
base='git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/'

usage() {
  sed -n '/^#   scripts/,/^#$/p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

# documents lists the files whose sources must follow MODULE_REF.
documents() {
  find "$tofu_dir/examples" "$tofu_dir/modules" -type f \( -name '*.tf' -o -name '*.md' \) -not -path '*/.terraform/*' \
    -not -path "$tofu_dir/modules/*/*.tf"
  find "$tofu_dir" -maxdepth 1 -name '*.md'
  if [ -f "$root/docs/operations/deploy-cloud-run.md" ]; then
    echo "$root/docs/operations/deploy-cloud-run.md"
  fi
}

check() {
  local ref status=0 file line value name found
  ref="$(cat "$tofu_dir/MODULE_REF")"
  if ! [[ "$ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
    echo "$tofu_dir/MODULE_REF: not a version: $ref" >&2
    return 1
  fi
  while IFS= read -r file; do
    # Comments count: a swap shown in a comment is what an operator copies.
    while IFS= read -r line; do
      value="${line#*\"}"
      value="${value%%\"*}"
      case "$value" in
        # The source of a provider, in required_providers.
        [a-z]*/[a-z]*) if [[ "$value" =~ ^[a-z0-9-]+/[a-z0-9-]+$ ]]; then continue; fi ;;
      esac
      found=1
      case "$value" in
        "$base"*'?ref='"$ref"'&depth=1')
          name="${value#"$base"}"
          name="${name%%\?*}"
          if [ ! -d "$tofu_dir/modules/$name" ]; then
            echo "$file: no module named $name: $value" >&2
            status=1
          fi
          ;;
        *)
          echo "$file: a source must be ${base}<name>?ref=$ref&depth=1, not: $value" >&2
          status=1
          ;;
      esac
    done < <(grep -E '^[[:space:]#]*source[[:space:]]*=[[:space:]]*"' "$file" || true)
  done < <(documents)
  if [ -z "${found:-}" ]; then
    echo "tofu-examples: no module source found" >&2
    return 1
  fi
  return "$status"
}

release_check() {
  local version="$1" ref
  case "$version" in
    *-*) return 0 ;;
  esac
  ref="$(cat "$tofu_dir/MODULE_REF")"
  if [ "$ref" != "$version" ]; then
    echo "deploy/opentofu/MODULE_REF is $ref, but a stable release must name itself: run scripts/tofu-examples.sh bump $version" >&2
    return 1
  fi
}

bump() {
  local version="$1" old file
  old="$(cat "$tofu_dir/MODULE_REF")"
  while IFS= read -r file; do
    sed -i "s|?ref=$old&depth=1|?ref=$version\&depth=1|g" "$file"
  done < <(documents)
  echo "$version" > "$tofu_dir/MODULE_REF"
}

# copy DIR copies deploy/opentofu to DIR/deploy/opentofu with the examples on local modules and their tests beside them.
copy() {
  local dir="$1" example name
  mkdir -p "$dir/deploy"
  tar -C "$root/deploy" --exclude=.terraform -cf - opentofu | tar -C "$dir/deploy" -xf -
  for example in "$dir"/deploy/opentofu/examples/*/; do
    name="$(basename "$example")"
    sed -i -E 's#git::https://github\.com/mycroblog/my\.git//deploy/opentofu/modules/([a-z0-9-]+)\?ref=[^"]*#../../modules/\1#' "$example"*.tf
    if [ -d "$dir/deploy/opentofu/tests/examples/$name" ]; then
      mkdir -p "${example}tests"
      cp "$dir/deploy/opentofu/tests/examples/$name/"*.tftest.hcl "${example}tests/"
    fi
  done
}

work_dir() {
  local dir="$root/.tmp/tofu-examples"
  rm -rf "$dir"
  mkdir -p "$dir"
  echo "$dir"
}

run_tests() {
  local dir example
  dir="$(work_dir)"
  copy "$dir"
  for example in "$dir"/deploy/opentofu/examples/*; do
    tofu -chdir="$example" init -backend=false -input=false -lockfile=readonly > /dev/null
    tofu -chdir="$example" validate
    tofu -chdir="$example" test
  done
}

lock() {
  local dir example name
  dir="$(work_dir)"
  copy "$dir"
  for example in "$dir"/deploy/opentofu/examples/*; do
    name="$(basename "$example")"
    tofu -chdir="$example" init -backend=false -input=false -upgrade > /dev/null
    tofu -chdir="$example" providers lock -platform=linux_amd64 -platform=linux_arm64 -platform=darwin_amd64 -platform=darwin_arm64
    cp "$example/.terraform.lock.hcl" "$tofu_dir/examples/$name/.terraform.lock.hcl"
  done
}

git_source_test() {
  local tmp ref example name scripts code
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064 # tmp is set here and must be expanded now.
  trap "rm -rf '$tmp'" RETURN
  ref="$(cat "$tofu_dir/MODULE_REF")"

  # A repository of this tree with the tag, reachable the way GitHub would be.
  mkdir -p "$tmp/src/deploy"
  tar -C "$root/deploy" --exclude=.terraform -cf - opentofu | tar -C "$tmp/src/deploy" -xf -
  git -C "$tmp/src" init -q
  git -C "$tmp/src" add -A
  git -C "$tmp/src" -c user.name=test -c user.email=test@example.org -c commit.gpgSign=false commit -q -m test
  git -C "$tmp/src" -c tag.gpgSign=false tag "$ref"
  git clone -q --bare "$tmp/src" "$tmp/my.git"

  for example in "$tofu_dir"/examples/*; do
    name="$(basename "$example")"
    # An operator's copy in an empty directory: the files of the example and its lock file, and nothing else.
    mkdir "$tmp/$name"
    cp -r "$example/." "$tmp/$name/"
    rm -rf "$tmp/$name/.terraform" "$tmp/$name/tests"
    sed -i "s|git::https://github.com/mycroblog/my.git|git::file://$tmp/my.git|" "$tmp/$name/"*.tf
    tofu -chdir="$tmp/$name" init -backend=false -input=false -lockfile=readonly > /dev/null
    tofu -chdir="$tmp/$name" validate

    # What the guide tells the operator to run.
    scripts="$tmp/$name/.terraform/modules/app/deploy/opentofu"
    code=0
    "$scripts/upgrade.sh" > /dev/null 2>&1 || code=$?
    if [ "$code" != 2 ]; then
      echo "tofu-examples: $scripts/upgrade.sh without arguments exited $code, want its usage" >&2
      return 1
    fi
    code=0
    "$scripts/console.sh" version <<< 'null' > /dev/null 2>&1 || code=$?
    if [ "$code" != 1 ]; then
      echo "tofu-examples: $scripts/console.sh with the console off exited $code" >&2
      return 1
    fi

    # The pin is what is used: another tag fails.
    rm -rf "$tmp/$name/.terraform"
    sed -i "s|?ref=$ref&|?ref=no-such-tag\&|" "$tmp/$name/"*.tf
    if tofu -chdir="$tmp/$name" init -backend=false -input=false > /dev/null 2>&1; then
      echo "tofu-examples: a ref that does not exist was accepted" >&2
      return 1
    fi
  done
  echo "tofu-examples: the Git source works from a copy"
}

[ "$#" -ge 1 ] || usage
command="$1"
shift
case "$command" in
  check)
    check
    if [ "${1:-}" = "--release" ]; then
      [ "$#" -eq 2 ] || usage
      release_check "$2"
    fi
    ;;
  bump)
    [ "$#" -eq 1 ] || usage
    bump "$1"
    ;;
  test) run_tests ;;
  lock) lock ;;
  git-source-test) git_source_test ;;
  *) usage ;;
esac
