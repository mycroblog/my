#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Proves, with a real plan and apply of deploy/opentofu/tests/db-no-secret-in-state against a local stub of the Cloud SQL Admin
# and Secret Manager APIs (scripts/tofu-db-state-test.py), what db-gcp-cloud-sql promises:
#
#   - the password the user gets is the one in the URL secret, in the URL the server must read;
#   - it is in neither the state, nor the saved plan, nor what tofu prints;
#   - another apply writes nothing, and raising password_version writes a new password to both, once;
#   - destroying is refused;
#   - a value of another module, unknown at plan time, reaches a secret through secrets-gcp-secret-manager's write_values, which
#     writes it once and keeps nothing of it beside what its own module keeps.
#
# It reaches no cloud, so it shows what OpenTofu and the provider do, not what Cloud SQL does.
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
  echo "tofu-db-state-test: $1" >&2
  exit 1
}

# A copy, so that the state and the plan stay out of the repository.
cp -r "$root/deploy/opentofu/modules" "$tmp/modules"
mkdir -p "$tmp/tests"
cp -r "$root/deploy/opentofu/tests/db-no-secret-in-state" "$tmp/tests/run"
rm -rf "$tmp/tests/run/.terraform" "$tmp"/modules/*/.terraform
dir="$tmp/tests/run"

python3 "$root/scripts/tofu-db-state-test.py" "$tmp/port" "$tmp/values" &
stub=$!
for _ in $(seq 1 50); do
  [ -s "$tmp/port" ] && break
  sleep 0.1
done
[ -s "$tmp/port" ] || fail "the stub did not start"
TF_VAR_endpoint="http://127.0.0.1:$(cat "$tmp/port")/"
export TF_VAR_endpoint
export TF_IN_AUTOMATION=1

value() { # kind name [nth]: the nth value of that kind and name
  awk -v k="$1" -v n="$2" -v i="${3:-1}" '$1 == k && $2 == n { if (++c == i) { print $3; exit } }' "$tmp/values"
}

tofu -chdir="$dir" init -backend=false -input=false > /dev/null
tofu -chdir="$dir" plan -input=false -out=plan.bin > "$tmp/plan.out"
tofu -chdir="$dir" show -json plan.bin > "$tmp/plan.json"
tofu -chdir="$dir" apply -input=false plan.bin > "$tmp/apply.out"

[ "$(wc -l < "$tmp/values")" = 3 ] || fail "want 3 values written, got: $(cut -d' ' -f1,2 "$tmp/values" | tr '\n' ';')"

password="$(value user my)"
[ "${#password}" = 40 ] || fail "the password is not 40 characters"
[[ "$password" =~ ^[A-Za-z0-9]+$ ]] || fail "the password needs escaping in a URL"
want="postgres://my:$password@/my?host=/cloudsql/p:r:my"
[ "$(value secret my-database-url)" = "$want" ] || fail "the URL secret does not hold the user's password in the URL the server reads"

written="$(value secret written-url)"
[ "${#written}" = 40 ] || fail "the value from elsewhere did not reach its secret"

# The files exist and the state holds the resources, so that the searches below look at the real thing.
for file in "$dir/terraform.tfstate" "$dir/plan.bin" "$tmp/plan.json" "$tmp/plan.out" "$tmp/apply.out"; do
  [ -s "$file" ] || fail "$(basename "$file") is empty or missing"
done
grep -q '"password_wo_version": 1' "$dir/terraform.tfstate" || fail "the state does not hold the user"
grep -q '"secret_data_wo_version": 1' "$dir/terraform.tfstate" || fail "the state does not hold the secret version"

no_password_anywhere() {
  for file in "$dir/terraform.tfstate" "$dir/plan.bin" "$tmp/plan.json" "$tmp/plan.out" "$tmp/apply.out"; do
    if grep -qaF -- "$1" "$file"; then
      fail "the password is in $(basename "$file")"
    fi
  done
  encoded="$(printf '%s' "postgres://my:$1@/my?host=/cloudsql/p:r:my" | base64 -w0)"
  if grep -qaF -- "$encoded" "$dir/terraform.tfstate" "$dir/plan.bin"; then
    fail "the encoded URL is in the state or the plan"
  fi
}
no_password_anywhere "$password"

# What the module of the value keeps is its own business. secrets-gcp-secret-manager keeps nothing: its version holds no value.
written_count="$(jq --arg v "$written" '[.resources[] | select(.module == "module.secrets") | .. | strings | select(contains($v))] | length' "$dir/terraform.tfstate")"
[ "$written_count" = 0 ] || fail "the secrets module keeps the value it writes in the state"

# Another apply must change nothing.
tofu -chdir="$dir" apply -input=false -auto-approve > "$tmp/apply2.out"
[ "$(wc -l < "$tmp/values")" = 3 ] || fail "a second apply wrote a new value"
tofu -chdir="$dir" plan -input=false -detailed-exitcode > /dev/null || fail "a second plan is not empty"

# A new password goes to both, and they agree.
tofu -chdir="$dir" apply -input=false -auto-approve -var password_version=2 > "$tmp/apply3.out"
[ "$(wc -l < "$tmp/values")" = 5 ] || fail "want 2 new values for a new password, got: $(cut -d' ' -f1,2 "$tmp/values" | tr '\n' ';')"
second="$(value user my 2)"
[ -n "$second" ] || fail "raising password_version wrote no new password"
[ "$second" != "$password" ] || fail "raising password_version did not make a new password"
[ "$(value secret my-database-url 2)" = "postgres://my:$second@/my?host=/cloudsql/p:r:my" ] || fail "after a new password, the URL and the user disagree"
if grep -qaF -- "$second" "$dir/terraform.tfstate" "$tmp/apply3.out"; then
  fail "the new password is in the state or the output"
fi

if tofu -chdir="$dir" plan -destroy -input=false -var password_version=2 > "$tmp/destroy.out" 2>&1; then
  fail "a destroy was not refused"
fi
grep -q 'prevent_destroy' "$tmp/destroy.out" || fail "a destroy failed for another reason: $(tail -5 "$tmp/destroy.out")"

echo "tofu-db-state-test: ok"
