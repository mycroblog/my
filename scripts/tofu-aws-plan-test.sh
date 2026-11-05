#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Shows with a real plan of deploy/opentofu/tests/aws-no-secret-in-plan, whose provider has no credentials and calls nothing,
# what the AWS modules that generate a value promise of the plan:
#
#   - the value goes to the write-only attribute, which the plan does not hold, at the fixed version;
#   - nothing goes to the attributes that the state keeps;
#   - the value does not appear in what tofu prints or in the saved plan's JSON.
#
# It runs no apply, so unlike scripts/tofu-state-test.sh it says nothing of the state beyond what the provider's schema
# promises, and it never sees the value: it looks for the attributes that would hold one.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail() {
  echo "tofu-aws-plan-test: $1" >&2
  exit 1
}

# A copy, so that the plan stays out of the repository.
cp -r "$root/deploy/opentofu/modules" "$tmp/modules"
mkdir -p "$tmp/tests"
cp -r "$root/deploy/opentofu/tests/aws-no-secret-in-plan" "$tmp/tests/run"
rm -rf "$tmp/tests/run/.terraform" "$tmp"/modules/*/.terraform
dir="$tmp/tests/run"
export TF_IN_AUTOMATION=1

tofu -chdir="$dir" init -backend=false -input=false > /dev/null
tofu -chdir="$dir" plan -input=false -out=plan.bin > "$tmp/plan.out"
tofu -chdir="$dir" show -json plan.bin > "$tmp/plan.json"

# The provider's own word: the attribute is write-only and the version is not.
tofu -chdir="$dir" providers schema -json > "$tmp/schema.json"
attribute() {
  jq -e --arg name "$1" '[.provider_schemas[].resource_schemas.aws_secretsmanager_secret_version.block.attributes[$name]][0]' "$tmp/schema.json"
}
[ "$(attribute secret_string_wo | jq -r .write_only)" = true ] || fail "the provider no longer declares secret_string_wo as write-only"
[ "$(attribute secret_string_wo_version | jq -r .write_only)" != true ] || fail "the provider changed secret_string_wo_version"

versions="$(jq -c '[.resource_changes[] | select(.type == "aws_secretsmanager_secret_version")]' "$tmp/plan.json")"
[ "$(jq length <<< "$versions")" = 3 ] || fail "want 3 versions planned, got: $(jq -c 'map(.address)' <<< "$versions")"

for attr in secret_string secret_binary secret_string_wo; do
  held="$(jq -c --arg attr "$attr" '[.[].change.after[$attr] // empty]' <<< "$versions")"
  [ "$held" = '[]' ] || fail "the plan holds $attr"
done
[ "$(jq -c '[.[].change.after.secret_string_wo_version] | unique' <<< "$versions")" = '[1]' ] || fail "the version must stay fixed at 1"

# The database's password, on the instance: write-only, at a version that moves with the URL's, and held nowhere.
instance_attribute() {
  jq -e --arg name "$1" '[.provider_schemas[].resource_schemas.aws_db_instance.block.attributes[$name]][0]' "$tmp/schema.json"
}
[ "$(instance_attribute password_wo | jq -r .write_only)" = true ] || fail "the provider no longer declares password_wo as write-only"
instance="$(jq -c '[.resource_changes[] | select(.type == "aws_db_instance")][0].change.after' "$tmp/plan.json")"
[ "$(jq -r '.password // "none"' <<< "$instance")" = none ] || fail "the plan holds the password"
[ "$(jq -r '.password_wo // "none"' <<< "$instance")" = none ] || fail "the plan holds password_wo"
[ "$(jq -r '.password_wo_version' <<< "$instance")" = 1 ] || fail "the instance's password version must start at 1"

# Nothing that looks like 32 bytes of base64 or base64url, or a password of 40 letters and digits, in what a person or a log
# would see.
for file in "$tmp/plan.out" "$tmp/plan.json"; do
  if grep -qE '(^|[^A-Za-z0-9+/_-])[A-Za-z0-9+/_-]{43}=?($|[^A-Za-z0-9+/_=-])' "$file"; then
    fail "$(basename "$file") holds something the size of a generated value"
  fi
  if grep -qE '(^|[^A-Za-z0-9])[A-Za-z0-9]{40}($|[^A-Za-z0-9])' "$file"; then
    fail "$(basename "$file") holds something the size of a generated password"
  fi
done

echo "tofu-aws-plan-test: ok"
