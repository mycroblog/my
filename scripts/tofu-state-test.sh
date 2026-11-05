#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Proves, with a real plan and apply of deploy/opentofu/tests/no-secret-in-state against a local stub of the Secret Manager
# API (scripts/tofu-state-test.py), what the generating modules promise:
#
#   - the values have the formats of kek and vapid_private_key;
#   - no value is in the state, the saved plan, or what tofu prints;
#   - another apply writes no new version, which would replace the KEK;
#   - destroying is refused, and so is turning off what generates the KEK.
#
# It reaches no cloud.
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

# A copy, so that the state and the plan stay out of the repository.
cp -r "$root/deploy/opentofu/modules" "$tmp/modules"
mkdir -p "$tmp/tests"
cp -r "$root/deploy/opentofu/tests/no-secret-in-state" "$tmp/tests/run"
rm -rf "$tmp/tests/run/.terraform" "$tmp"/modules/*/.terraform
dir="$tmp/tests/run"

python3 "$root/scripts/tofu-state-test.py" "$tmp/port" "$tmp/values" &
stub=$!
for _ in $(seq 1 50); do
  [ -s "$tmp/port" ] && break
  sleep 0.1
done
[ -s "$tmp/port" ] || {
  echo "tofu-state-test: the stub did not start" >&2
  exit 1
}
TF_VAR_endpoint="http://127.0.0.1:$(cat "$tmp/port")/"
export TF_VAR_endpoint
export TF_IN_AUTOMATION=1

fail() {
  echo "tofu-state-test: $1" >&2
  exit 1
}

tofu -chdir="$dir" init -backend=false -input=false > /dev/null
tofu -chdir="$dir" plan -input=false -out=plan.bin > "$tmp/plan.out"
tofu -chdir="$dir" show -json plan.bin > "$tmp/plan.json"
tofu -chdir="$dir" apply -input=false plan.bin > "$tmp/apply.out"

[ "$(wc -l < "$tmp/values")" = 2 ] || fail "want 2 values written, got: $(cut -d' ' -f1 "$tmp/values" | tr '\n' ' ')"

# The formats: checkKEK and webpush.ParsePrivateKey.
python3 -I - "$tmp/values" << 'EOF'
import base64, sys
order = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
values = dict(line.split() for line in open(sys.argv[1]))
kek = base64.b64decode(values["my-kek"], validate=True)
assert len(kek) == 32, "the KEK must be 32 bytes in standard base64"
vapid = values["my-vapid-key"]
assert "=" not in vapid and "+" not in vapid and "/" not in vapid, "the VAPID key must be base64url without padding"
raw = base64.urlsafe_b64decode(vapid + "=")
assert len(raw) == 32, "the VAPID key must be 32 bytes"
assert 0 < int.from_bytes(raw, "big") < order, "the VAPID key must be below the order of P-256"
EOF

# The files exist and the state holds the resources, so that the searches below look at the real thing.
for file in "$dir/terraform.tfstate" "$dir/plan.bin" "$tmp/plan.json" "$tmp/plan.out" "$tmp/apply.out"; do
  [ -s "$file" ] || fail "$(basename "$file") is empty or missing"
done
grep -q '"secret_data_wo_version": 1' "$dir/terraform.tfstate" || fail "the state does not hold the secret versions"

while read -r secret value; do
  for file in "$dir/terraform.tfstate" "$dir/plan.bin" "$tmp/plan.json" "$tmp/plan.out" "$tmp/apply.out"; do
    if grep -qaF -- "$value" "$file"; then
      fail "the value of $secret is in $(basename "$file")"
    fi
  done
  # The base64 that went over the wire, in case something stored that.
  encoded="$(printf '%s' "$value" | base64 -w0)"
  if grep -qaF -- "$encoded" "$dir/terraform.tfstate" "$dir/plan.bin"; then
    fail "the encoded value of $secret is in the state or the plan"
  fi
done < "$tmp/values"

# Another apply must change nothing, above all not write a second KEK.
tofu -chdir="$dir" apply -input=false -auto-approve > "$tmp/apply2.out"
[ "$(wc -l < "$tmp/values")" = 2 ] || fail "a second apply wrote a new version"
tofu -chdir="$dir" plan -input=false -detailed-exitcode > /dev/null || fail "a second plan is not empty"

if tofu -chdir="$dir" plan -destroy -input=false > "$tmp/destroy.out" 2>&1; then
  fail "a destroy was not refused"
fi
grep -q 'prevent_destroy' "$tmp/destroy.out" || fail "a destroy failed for another reason: $(tail -5 "$tmp/destroy.out")"

# Nor may turning a module's generation off take the KEK with it.
sed -i '/^variable "create"/,/^}/s/default     = true/default     = false/' "$tmp/modules/kek-gcp-secret-manager/main.tf"
if tofu -chdir="$dir" plan -input=false > "$tmp/off.out" 2>&1; then
  fail "turning off create was not refused"
fi
grep -q 'prevent_destroy' "$tmp/off.out" || fail "turning off create failed for another reason: $(tail -5 "$tmp/off.out")"

echo "tofu-state-test: ok"
