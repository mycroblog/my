#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Shows with real plans of deploy/opentofu/examples/aws-ecs-neon, whose provider is given no credentials, what mycroblog-aws
# promises of the plan:
#
#   - the recommended setup plans with no call to AWS: the one read it makes, GetCallerIdentity, goes to a local stub of STS
#     (scripts/tofu-aws-plan-test.py), which records anything else it is asked;
#   - each generated value (the KEK, the VAPID key, and with database.kind = rds the database's URL and password) goes to a
#     write-only attribute, which the plan does not hold, at the fixed version, and to no attribute that the state keeps;
#   - no such value appears in what tofu prints or in the saved plan's JSON.
#
# It runs no apply, so unlike scripts/tofu-state-test.sh it says nothing of the state beyond what the provider's schema
# promises, and it never sees the value: it looks for the attributes that would hold one.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
stub=""
cleanup() {
  if [ -n "$stub" ]; then
    kill "$stub" 2> /dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT

fail() {
  echo "tofu-aws-plan-test: $1" >&2
  exit 1
}

# A copy of the example on the modules of this commit, so that the plans stay out of the repository and nothing is fetched
# from GitHub, with the values of its terraform.tfvars.example.
mkdir -p "$tmp/deploy"
tar -C "$root/deploy" --exclude=.terraform -cf - opentofu | tar -C "$tmp/deploy" -xf -
dir="$tmp/deploy/opentofu/examples/aws-ecs-neon"
sed -i -E 's#git::https://github\.com/mycroblog/my\.git//deploy/opentofu/([a-z0-9/-]+)\?ref=[^"]*#../../\1#' "$dir"/*.tf
cp "$dir/terraform.tfvars.example" "$dir/terraform.tfvars"

python3 "$root/scripts/tofu-aws-plan-test.py" "$tmp/port" "$tmp/calls" &
stub=$!
for _ in $(seq 1 50); do
  [ -s "$tmp/port" ] && break
  sleep 0.1
done
[ -s "$tmp/port" ] || fail "the stub did not start"
touch "$tmp/calls"

# Values that are no credentials, which the provider is told not to check, and STS on the stub.
cat > "$dir/provider_override.tf" << EOF
provider "aws" {
  access_key                  = "not-a-credential"
  secret_key                  = "not-a-credential"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_region_validation      = true
  skip_requesting_account_id  = true

  endpoints {
    sts = "http://127.0.0.1:$(cat "$tmp/port")"
  }
}
EOF
export TF_IN_AUTOMATION=1
# Nothing of the example uses Cloudflare, so its provider must plan without a token.
unset CLOUDFLARE_API_TOKEN

tofu -chdir="$dir" init -backend=false -input=false -lockfile=readonly > /dev/null

# The provider's own word: the attributes are write-only and the versions are not.
tofu -chdir="$dir" providers schema -json > "$tmp/schema.json"
attribute() {
  jq -e --arg type "$1" --arg name "$2" '[.provider_schemas[].resource_schemas[$type].block.attributes[$name] // empty][0]' "$tmp/schema.json"
}
[ "$(attribute aws_secretsmanager_secret_version secret_string_wo | jq -r .write_only)" = true ] || fail "the provider no longer declares secret_string_wo as write-only"
[ "$(attribute aws_secretsmanager_secret_version secret_string_wo_version | jq -r .write_only)" != true ] || fail "the provider changed secret_string_wo_version"
[ "$(attribute aws_db_instance password_wo | jq -r .write_only)" = true ] || fail "the provider no longer declares password_wo as write-only"

# plan NAME [OVERRIDE] plans the example, with OVERRIDE as a file that changes the block of mycroblog-aws, and checks the
# versions of the secrets that hold generated values. The saved plan is $tmp/NAME.json.
plan() {
  local name="$1" versions held
  rm -f "$dir/mycroblog_override.tf"
  if [ -n "${2:-}" ]; then
    printf '%s\n' "$2" > "$dir/mycroblog_override.tf"
  fi
  tofu -chdir="$dir" plan -input=false -out=plan.bin > "$tmp/$name.out" || fail "the plan of $name failed: $(cat "$tmp/$name.out")"
  tofu -chdir="$dir" show -json plan.bin > "$tmp/$name.json"

  versions="$(jq -c '[.resource_changes[] | select(.type == "aws_secretsmanager_secret_version")]' "$tmp/$name.json")"
  [ "$(jq length <<< "$versions")" = "$3" ] || fail "$name: want $3 versions planned, got: $(jq -c 'map(.address)' <<< "$versions")"
  for attr in secret_string secret_binary secret_string_wo; do
    held="$(jq -c --arg attr "$attr" '[.[].change.after[$attr] // empty]' <<< "$versions")"
    [ "$held" = '[]' ] || fail "$name: the plan holds $attr"
  done
  [ "$(jq -c '[.[].change.after.secret_string_wo_version] | unique' <<< "$versions")" = '[1]' ] || fail "$name: the version must stay fixed at 1"

  # Nothing that looks like 32 bytes of base64 or base64url, or a password of 40 letters and digits, in what a person or a
  # log would see. The hashes of the canary's archive have that shape, and are left out, and so are the provider's attribute
  # names, which are as long but hold no capital letter, as random base64 of that length all but always does.
  jq 'del(.prior_state, .. | .output_base64sha256?, .output_base64sha512?, .output_md5?, .output_sha?, .output_sha256?, .output_sha512?, .source_code_hash?)' \
    "$tmp/$name.json" > "$tmp/$name.checked.json"
  for file in "$tmp/$name.out" "$tmp/$name.checked.json"; do
    if grep -oE '(^|[^A-Za-z0-9+/_-])[A-Za-z0-9+/_-]{43}=?($|[^A-Za-z0-9+/_=-])' "$file" | grep -qE '[A-Z]'; then
      fail "$name: $(basename "$file") holds something the size of a generated value"
    fi
    # A SHA-1 in hex, such as the ID of the archive, has the size of a password too, and only hex digits.
    if grep -oE '(^|[^A-Za-z0-9])[A-Za-z0-9]{40}($|[^A-Za-z0-9])' "$file" | grep -qE '[g-zA-Z]'; then
      fail "$name: $(basename "$file") holds something the size of a generated password"
    fi
  done
}

# The recommended setup: the KEK and the VAPID key.
plan recommended "" 2
[ "$(jq '[.resource_changes[] | select(.type == "aws_db_instance")] | length' "$tmp/recommended.json")" = 0 ] || fail "the recommended setup creates no database"

# RDS: its URL too, and the password on the instance: write-only, at a version that moves with the URL's, and held nowhere.
plan rds 'module "mycroblog" {
  database = { kind = "rds" }
}' 3
instance="$(jq -c '[.resource_changes[] | select(.type == "aws_db_instance")][0].change.after' "$tmp/rds.json")"
[ "$(jq -r '.password // "none"' <<< "$instance")" = none ] || fail "the plan holds the password"
[ "$(jq -r '.password_wo // "none"' <<< "$instance")" = none ] || fail "the plan holds password_wo"
[ "$(jq -r '.password_wo_version' <<< "$instance")" = 1 ] || fail "the instance's password version must start at 1"

[ ! -s "$tmp/calls" ] || fail "the plans asked the stub for more than the caller's identity: $(cat "$tmp/calls")"

echo "tofu-aws-plan-test: ok"
