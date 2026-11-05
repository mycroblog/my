#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Fails when the type of `backends` of an app module and the backend contract disagree. OpenTofu drops an unknown field
# without a word when it converts a module's outputs to that type, so a setting or a security group that a backend returns
# would vanish, with a plan that passes. So:
#
#   - every field of the contract tables of README.md is in the `backends` of every app-* module, and nothing else is;
#   - every such field is read or refused by that module: `b.<field>` is in its configuration;
#   - every output of the other modules is either in the contract or listed here as being for the operator;
#   - every other module returns platform, which is what its name says: gcp or aws when the name has it, neutral otherwise.
#
# Usage: scripts/tofu-backend-fields-check.sh [DIR]   (default: deploy/opentofu)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
dir="${1:-$root/deploy/opentofu}"

# Outputs that the operator reads, or hands to another block by name, and that never go through backends. A new output
# that the app should take goes in the contract instead.
for_the_operator="
  alarm_metrics alerts budget_topic canary_name connection_name connection_uri connection_uri_pooler
  dns_note dns_records domain_name hosted_zone_id ids ip_address notification_channels project_id role_arn rules
  topic_arn uptime_check_id url_without_password
"

# The first column of the first table of a section, without a marker such as "(optional)".
# shellcheck disable=SC2016 # the backticks are the table's
table_fields() {
  awk -v section="## $2" '
    $0 == section { in_section = 1; next }
    in_section && /^## / { exit }
    in_section && /^\| / { in_table = 1; print; next }
    in_section && in_table { exit }
  ' "$1" | grep -oE '^\| `[a-z_]+`' | tr -d '|` '
}

# The fields of the object of the backends variable: at the object's own indent, not the deeper ones of a nested object.
backends_fields() {
  awk '
    /^variable "backends"/ { in_var = 1; next }
    in_var && /^  default/ { exit }
    in_var && /^    [a-z_]+ +=/ { print $1 }
  ' "$1"
}

contract="$(mktemp)"
trap 'rm -f "$contract"' EXIT
{
  table_fields "$dir/README.md" "The backend contract"
  table_fields "$dir/README.md" "The ingress contract"
} | grep -vxF dns_records | sort -u > "$contract"
[ "$(wc -l < "$contract")" -gt 3 ] || {
  echo "tofu-backend-fields-check: no fields found in the contract tables of $dir/README.md" >&2
  exit 1
}

status=0
apps=0
for app in "$dir"/modules/app-*/; do
  apps=$((apps + 1))
  name="$(basename "$app")"
  fields="$(backends_fields "$app/variables.tf" | sort -u)"
  while IFS= read -r field; do
    echo "$name: backends has no field $field, so what a backend returns there is dropped without a word" >&2
    status=1
  done < <(comm -23 "$contract" <(echo "$fields") | grep . || true)
  while IFS= read -r field; do
    echo "$name: backends has the field $field, which the contract in README.md does not name" >&2
    status=1
  done < <(comm -13 "$contract" <(echo "$fields") | grep . || true)
  while IFS= read -r field; do
    if ! grep -qE "\bb\.$field\b" "$app"/*.tf; then
      echo "$name: nothing reads or refuses b.$field: a backend's value would do nothing" >&2
      status=1
    fi
  done <<< "$fields"
done
[ "$apps" -gt 0 ] || {
  echo "tofu-backend-fields-check: no app-* module in $dir/modules" >&2
  exit 1
}

for module in "$dir"/modules/*/; do
  name="$(basename "$module")"
  case "$name" in app-*) continue ;; esac
  case "$name" in
    *-gcp | *-gcp-*) expected=gcp ;;
    *-aws | *-aws-*) expected=aws ;;
    *) expected=neutral ;;
  esac
  platform="$(cat "$module"/*.tf | awk '/^output "platform"/ { in_output = 1; next } in_output && /^}/ { exit } in_output && /^  value +=/ { print $3 }' | tr -d '"')"
  if [ "$platform" != "$expected" ]; then
    echo "$name: its platform is \"$platform\", and its name says $expected: an app refuses a backend of another platform by this" >&2
    status=1
  fi
  while IFS= read -r output; do
    grep -qxF "$output" "$contract" && continue
    # shellcheck disable=SC2086 # the list is words on purpose
    printf '%s\n' $for_the_operator | grep -qxF "$output" && continue
    echo "$name: the output $output is neither in the contract in README.md nor listed for the operator in $(basename "$0")" >&2
    status=1
  done < <(cat "$module"/*.tf | grep -oE '^output "[a-z_0-9]+"' | cut -d'"' -f2)
done
exit "$status"
