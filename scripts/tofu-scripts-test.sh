#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests deploy/opentofu/upgrade.sh, deploy/opentofu/console.sh, scripts/tofu-examples.sh, and scripts/tofu-env-check.sh against a gcloud
# that records its calls: the order of the steps, stopping at a failure, and passing on an exit code. It shows nothing
# about Google Cloud itself.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
failures=0

fail() {
  echo "FAIL: $1" >&2
  failures=$((failures + 1))
}

mkdir "$tmp/bin"
cat > "$tmp/bin/gcloud" << 'EOF'
#!/usr/bin/env bash
# Records the call, fails when it contains $FAIL_ON, and answers the few reads the scripts make.
echo "$*" >> "$CALLS"
if [ -n "${FAIL_ON:-}" ]; then
  case "$*" in *"$FAIL_ON"*) exit "${FAIL_CODE:-1}" ;; esac
fi
case "$*" in
  "run services describe"*) echo "${SERVICE_IMAGE:-image:new}" ;;
  "run jobs describe"*) echo "${JOB_IMAGE:-image:new}" ;;
  "run jobs executions list"*) echo "${EXECUTION-exec-1}" ;;
  "run jobs executions tasks list"*) echo "${TASK_EXIT_CODE:-}" ;;
  "logging read"*) echo "${LOG_ENTRIES:-[]}" ;;
esac
exit 0
EOF
chmod +x "$tmp/bin/gcloud"
export PATH="$tmp/bin:$PATH" CALLS="$tmp/calls" CONSOLE_LOG_WAIT_SECONDS=0

upgrade() {
  : > "$CALLS"
  "$root/deploy/opentofu/upgrade.sh" --project p --region r --image image:new "$@" > "$tmp/out" 2>&1
}

# The verbs of the recorded calls, in order, without the flags that every call has.
steps() {
  sed -E 's/ --project=p//; s/ --region=r//; s/ --quiet//; s/ --wait//; s/run jobs (execute|update) my-migrate/job \1/; s/run services update my/service update/' "$CALLS"
}

# upgrade: the order, and the service last.
upgrade
want='job update --image=image:new
job execute --args=config,check
job execute --args=db,upgrade,-check
job execute --args=schema,apply
job execute --args=db,upgrade
service update --image=image:new'
[ "$(steps)" = "$want" ] || fail "upgrade ran: $(steps)"

# upgrade: each step that fails stops everything after it, and the service never changes.
for failing in 'args=config' 'args=db,upgrade,-check' 'args=schema,apply' 'args=db,upgrade --wait'; do
  if FAIL_ON="$failing" upgrade; then
    fail "upgrade succeeded although '$failing' failed"
  fi
  if grep -q 'services update' "$CALLS"; then
    fail "the service changed although '$failing' failed"
  fi
done

# upgrade: the options.
upgrade --migrate-only
if grep -q 'services update' "$CALLS"; then fail "--migrate-only changed the service"; fi
upgrade --plan
grep -q -- '--args=schema,apply,-dry-run' "$CALLS" || fail "--plan did not run the dry run"
if grep -qE 'args=schema,apply --|args=db,upgrade --|services update' "$CALLS"; then fail "--plan changed something"; fi
upgrade --allow-destructive
grep -q -- '--args=schema,apply,-allow-destructive' "$CALLS" || fail "--allow-destructive was not passed"
upgrade --maintenance
[ "$(head -n 1 "$CALLS" | grep -c 'MY_MAINTENANCE=true')" = 1 ] || fail "--maintenance did not come first"
tail -n 1 "$CALLS" | grep -q -- '--remove-env-vars=MY_MAINTENANCE' || fail "--maintenance did not end with the new revision"
if "$root/deploy/opentofu/upgrade.sh" --project p > /dev/null 2>&1; then fail "upgrade.sh without --image succeeded"; fi

CONSOLE='{"platform":"gcp-cloud-run","project":"p","region":"r","job":"my-console","service":"my"}'

console() {
  : > "$CALLS"
  local code=0
  "$root/deploy/opentofu/console.sh" "$@" <<< "$CONSOLE" > "$tmp/out" 2> "$tmp/err" || code=$?
  return "$code"
}

# console: the arguments survive commas, the output comes back, and so does success.
LOG_ENTRIES='[{"textPayload":"a link"},{"jsonPayload":{"msg":"x"}}]' console admin create -email 'a,b@example.com' || fail "console failed"
grep -qF -- '--args=^@@@^admin@@@create@@@-email@@@a,b@example.com' "$CALLS" || fail "console args: $(cat "$CALLS")"
grep -qx 'a link' "$tmp/out" || fail "console did not print the text output: $(cat "$tmp/out")"
grep -qxF '{"msg":"x"}' "$tmp/out" || fail "console did not print the JSON output: $(cat "$tmp/out")"
grep -q 'execution_name\\*"="exec-1"\|execution_name"="exec-1"' "$CALLS" || fail "console read other logs: $(cat "$CALLS")"
if grep -q 'run jobs update' "$CALLS"; then fail "console updated a job that has the service's image"; fi

# console: the job follows the service's image.
JOB_IMAGE=image:old console version || fail "console failed"
grep -q 'run jobs update my-console .*--image=image:new' "$CALLS" || fail "console left the job on another image"

# console: the command's exit code.
code=0
FAIL_ON='run jobs execute' TASK_EXIT_CODE=3 LOG_ENTRIES='[{"textPayload":"no"}]' console version || code=$?
[ "$code" = 3 ] || fail "console exited $code, want the command's 3"
code=0
FAIL_ON='run jobs execute' console version || code=$?
[ "$code" = 1 ] || fail "console exited $code without an exit code of the task, want 1"
code=0
FAIL_ON='run jobs execute' EXECUTION='' console version || code=$?
[ "$code" = 1 ] || fail "console exited $code with no execution, want 1"

# console: refusals.
code=0
"$root/deploy/opentofu/console.sh" version <<< 'null' > /dev/null 2>&1 || code=$?
[ "$code" = 1 ] || fail "console with the console off exited $code"
code=0
"$root/deploy/opentofu/console.sh" version <<< '{"platform":"elsewhere"}' > /dev/null 2>&1 || code=$?
[ "$code" = 1 ] || fail "console on an unknown platform exited $code"
code=0
"$root/deploy/opentofu/console.sh" <<< "$CONSOLE" > /dev/null 2>&1 || code=$?
[ "$code" = 2 ] || fail "console without a command exited $code"

# tofu-env-check: a setting of the reference passes, its _FILE form only where the reference has it.
cat > "$tmp/reference.md" << 'EOF'
- Environment variable: `MY_HOST`
- Environment variable: `MY_KEK` or `MY_KEK_FILE`
EOF
mkdir "$tmp/ok" "$tmp/bad"
echo 'MY_HOST = 1, MY_KEK_FILE = 2, MY_CONFIG = 3' > "$tmp/ok/main.tf"
echo 'MY_HOST_FILE = 1' > "$tmp/bad/main.tf"
"$root/scripts/tofu-env-check.sh" -r "$tmp/reference.md" "$tmp/ok" || fail "env check refused known names"
if "$root/scripts/tofu-env-check.sh" -r "$tmp/reference.md" "$tmp/bad" 2> /dev/null; then fail "env check passed an unknown name"; fi

# tofu-examples: the sources and the release gate, on a small tree.
tree="$tmp/tree"
mkdir -p "$tree/deploy/opentofu/examples/e" "$tree/deploy/opentofu/modules/m" "$tree/docs/operations"
echo v0.1.0 > "$tree/deploy/opentofu/MODULE_REF"
good='git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/m?ref=v0.1.0&depth=1'
write_example() {
  printf 'terraform {\n  required_providers {\n    google = {\n      source = "hashicorp/google"\n    }\n  }\n}\nmodule "m" {\n  source = "%s"\n}\n' "$1" > "$tree/deploy/opentofu/examples/e/main.tf"
}
examples() {
  TOFU_EXAMPLES_ROOT="$tree" "$root/scripts/tofu-examples.sh" "$@" > /dev/null 2>&1
}
write_example "$good"
examples check || fail "a good source was refused"
for bad in '../../modules/m' 'git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/m' \
  'git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/m?ref=v0.0.9&depth=1' \
  'git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/none?ref=v0.1.0&depth=1' \
  'git::https://github.com/someone/else.git//deploy/opentofu/modules/m?ref=v0.1.0&depth=1'; do
  write_example "$bad"
  if examples check; then fail "check passed: $bad"; fi
done
write_example "$good"
printf '#   source = "../../modules/m"\n' >> "$tree/deploy/opentofu/examples/e/main.tf"
if examples check; then fail "check passed a relative source in a comment"; fi
write_example "$good"
examples check --release v0.1.0 || fail "the release of MODULE_REF was refused"
examples check --release v0.2.0-rc.1 || fail "a prerelease was checked"
if examples check --release v0.2.0; then fail "a stable release passed with another ref"; fi
examples bump v0.2.0 || fail "bump failed"
examples check --release v0.2.0 || fail "bump left something behind"
grep -qF 'ref=v0.2.0&depth=1' "$tree/deploy/opentofu/examples/e/main.tf" || fail "bump did not change the source"

if [ "$failures" -gt 0 ]; then
  echo "tofu-scripts-test: $failures failed" >&2
  exit 1
fi
echo "tofu-scripts-test: ok"
