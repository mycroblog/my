#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Tests deploy/terraform/upgrade.sh, deploy/terraform/console.sh, scripts/terraform-examples.sh, and scripts/terraform-env-check.sh against a gcloud
# and an aws that record their calls: the order of the steps, stopping at a failure, and passing on an exit code. It shows
# nothing about Google Cloud or AWS themselves.
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
cat > "$tmp/bin/aws" << 'EOF'
#!/usr/bin/env bash
# Records the call, fails when it contains $FAIL_ON, and answers as Lambda would: $ANSWER is what the function returns.
echo "$*" >> "$CALLS"
if [ -n "${FAIL_ON:-}" ]; then
  case "$*" in *"$FAIL_ON"*) exit "${FAIL_CODE:-1}" ;; esac
fi
case "$*" in
  "lambda get-function --function-name my-console"*) echo "${JOB_IMAGE:-image:new}" ;;
  "lambda get-function "*) echo "${SERVICE_IMAGE:-image:new}" ;;
  "lambda get-function-configuration"*) echo '{"MY_HOST":"h"}' ;;
  "lambda invoke"*)
    echo "attempts=${AWS_MAX_ATTEMPTS:-}" >> "$CALLS"
    for last; do :; done
    default_answer='{"exit_code":0,"output":"ok"}' default_meta='{"StatusCode":200}'
    echo "${ANSWER:-$default_answer}" > "$last"
    echo "${META:-$default_meta}"
    ;;
esac
exit 0
EOF
chmod +x "$tmp/bin/aws"
export PATH="$tmp/bin:$PATH" CALLS="$tmp/calls" CONSOLE_LOG_WAIT_SECONDS=0

upgrade() {
  : > "$CALLS"
  "$root/deploy/terraform/upgrade.sh" --project p --region r --image image:new "$@" > "$tmp/out" 2>&1
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
if "$root/deploy/terraform/upgrade.sh" --project p > /dev/null 2>&1; then fail "upgrade.sh without --image succeeded"; fi

CONSOLE='{"platform":"gcp-cloud-run","project":"p","region":"r","job":"my-console","service":"my"}'

console() {
  : > "$CALLS"
  local code=0
  "$root/deploy/terraform/console.sh" "$@" <<< "$CONSOLE" > "$tmp/out" 2> "$tmp/err" || code=$?
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
"$root/deploy/terraform/console.sh" version <<< 'null' > /dev/null 2>&1 || code=$?
[ "$code" = 1 ] || fail "console with the console off exited $code"
code=0
"$root/deploy/terraform/console.sh" version <<< '{"platform":"elsewhere"}' > /dev/null 2>&1 || code=$?
[ "$code" = 1 ] || fail "console on an unknown platform exited $code"
code=0
"$root/deploy/terraform/console.sh" <<< "$CONSOLE" > /dev/null 2>&1 || code=$?
[ "$code" = 2 ] || fail "console without a command exited $code"

# console on Lambda: the arguments survive as they are, the output comes back, and so does the exit code.
LAMBDA_CONSOLE='{"platform":"aws-lambda","region":"r","function":"my-console","service":"my-web"}'

lambda_console() {
  : > "$CALLS"
  local code=0
  "$root/deploy/terraform/console.sh" "$@" <<< "$LAMBDA_CONSOLE" > "$tmp/out" 2> "$tmp/err" || code=$?
  return "$code"
}

lambda_console admin create -email 'a,b "c"@example.com' || fail "lambda console failed: $(cat "$tmp/err")"
grep -qF -- '--payload {"args":["admin","create","-email","a,b \"c\"@example.com"]}' "$CALLS" || fail "lambda console args: $(cat "$CALLS")"
grep -qx 'ok' "$tmp/out" || fail "lambda console did not print the output: $(cat "$tmp/out")"
grep -qx 'attempts=1' "$CALLS" || fail "lambda console may run a command twice: $(cat "$CALLS")"
if grep -q 'update-function-code' "$CALLS"; then fail "lambda console updated a function that has the service's image"; fi
JOB_IMAGE=image:old lambda_console version || fail "lambda console failed"
grep -q 'update-function-code --function-name my-console .*--image-uri image:new' "$CALLS" || fail "lambda console left the function on another image"
code=0
ANSWER='{"exit_code":3,"output":"no"}' lambda_console version || code=$?
[ "$code" = 3 ] || fail "lambda console exited $code, want the command's 3"
grep -qx 'no' "$tmp/out" || fail "lambda console hid the output of a failure"
code=0
ANSWER='{"errorMessage":"Task timed out after 900.00 seconds"}' META='{"StatusCode":200,"FunctionError":"Unhandled"}' lambda_console version || code=$?
[ "$code" = 1 ] || fail "lambda console exited $code on a function error, want 1"
grep -q 'may have stopped part way.*timed out' "$tmp/err" || fail "lambda console did not say what failed: $(cat "$tmp/err")"
code=0
ANSWER='{"statusCode":404}' lambda_console version || code=$?
[ "$code" = 1 ] || fail "lambda console exited $code on an answer with no exit code, want 1"

# upgrade on Lambda: the same order, the migration function first, and the service last.
lambda_upgrade() {
  : > "$CALLS"
  "$root/deploy/terraform/upgrade.sh" --platform aws-lambda --region r --image image:new "$@" > "$tmp/out" 2>&1
}

lambda_steps() {
  grep -v '^attempts=\|^lambda wait' "$CALLS" | sed -E 's/ --region r//; s/ --cli-binary-format raw-in-base64-out --cli-read-timeout 0//; s/ --image-uri image:new//; s/lambda invoke --function-name my-migrate --payload /invoke /; s/ \/.*$//; s/lambda update-function-code --function-name /code /'
}

lambda_upgrade || fail "lambda upgrade failed: $(cat "$tmp/out")"
want='code my-migrate
invoke {"args":["config","check"]}
invoke {"args":["db","upgrade","-check"]}
invoke {"args":["schema","apply"]}
invoke {"args":["db","upgrade"]}
code my-worker
code my-web'
[ "$(lambda_steps)" = "$want" ] || fail "lambda upgrade ran: $(lambda_steps)"
if ANSWER='{"exit_code":1,"output":"no"}' lambda_upgrade; then fail "lambda upgrade went on after a step failed"; fi
if grep -q 'update-function-code --function-name my-w' "$CALLS"; then fail "the functions changed although a step failed"; fi
lambda_upgrade --migrate-only
if grep -q 'update-function-code --function-name my-w' "$CALLS"; then fail "--migrate-only changed the functions"; fi
lambda_upgrade --plan
grep -qF '{"args":["schema","apply","-dry-run"]}' "$CALLS" || fail "--plan did not run the dry run on Lambda"
if grep -qF 'update-function-code --function-name my-w' "$CALLS"; then fail "--plan changed the functions on Lambda"; fi
lambda_upgrade --maintenance
head -n 1 "$CALLS" | grep -q 'get-function-configuration --function-name my-worker' || fail "--maintenance did not come first on Lambda: $(head -n 1 "$CALLS")"
grep -q 'update-function-configuration --function-name my-web .*"MY_MAINTENANCE":"true"' "$CALLS" || fail "--maintenance did not reach the web function: $(cat "$CALLS")"
tail -n 2 "$CALLS" | grep -q 'update-function-configuration --function-name my-web --region r --environment {"Variables":{"MY_HOST":"h"}}' || fail "--maintenance did not end with the new image on Lambda: $(tail -n 3 "$CALLS")"
if "$root/deploy/terraform/upgrade.sh" --platform elsewhere --region r --image i > /dev/null 2>&1; then fail "upgrade.sh took an unknown platform"; fi

# terraform-env-check: a setting of the reference passes, its _FILE form only where the reference has it.
cat > "$tmp/reference.md" << 'EOF'
- Environment variable: `MY_HOST`
- Environment variable: `MY_KEK` or `MY_KEK_FILE`
EOF
mkdir "$tmp/ok" "$tmp/bad"
echo 'MY_HOST = 1, MY_KEK_FILE = 2, MY_CONFIG = 3' > "$tmp/ok/main.tf"
echo 'MY_HOST_FILE = 1' > "$tmp/bad/main.tf"
"$root/scripts/terraform-env-check.sh" -r "$tmp/reference.md" "$tmp/ok" || fail "env check refused known names"
if "$root/scripts/terraform-env-check.sh" -r "$tmp/reference.md" "$tmp/bad" 2> /dev/null; then fail "env check passed an unknown name"; fi

# terraform-examples: the sources and the release gate, on a small tree.
tree="$tmp/tree"
mkdir -p "$tree/deploy/terraform/examples/e" "$tree/deploy/terraform/modules/m" "$tree/docs/operations"
echo v0.1.0 > "$tree/deploy/terraform/MODULE_REF"
good='git::https://github.com/mycroblog/my.git//deploy/terraform/modules/m?ref=v0.1.0&depth=1'
write_example() {
  printf 'terraform {\n  required_providers {\n    google = {\n      source = "hashicorp/google"\n    }\n  }\n}\nmodule "m" {\n  source = "%s"\n}\n' "$1" > "$tree/deploy/terraform/examples/e/main.tf"
}
examples() {
  TERRAFORM_EXAMPLES_ROOT="$tree" "$root/scripts/terraform-examples.sh" "$@" > /dev/null 2>&1
}
write_example "$good"
examples check || fail "a good source was refused"
for bad in '../../modules/m' 'git::https://github.com/mycroblog/my.git//deploy/terraform/modules/m' \
  'git::https://github.com/mycroblog/my.git//deploy/terraform/modules/m?ref=v0.0.9&depth=1' \
  'git::https://github.com/mycroblog/my.git//deploy/terraform/modules/none?ref=v0.1.0&depth=1' \
  'git::https://github.com/someone/else.git//deploy/terraform/modules/m?ref=v0.1.0&depth=1'; do
  write_example "$bad"
  if examples check; then fail "check passed: $bad"; fi
done
write_example "$good"
printf '#   source = "../../modules/m"\n' >> "$tree/deploy/terraform/examples/e/main.tf"
if examples check; then fail "check passed a relative source in a comment"; fi
write_example "$good"
examples check --release v0.1.0 || fail "the release of MODULE_REF was refused"
examples check --release v0.2.0-rc.1 || fail "a prerelease was checked"
if examples check --release v0.2.0; then fail "a stable release passed with another ref"; fi
examples bump v0.2.0 || fail "bump failed"
examples check --release v0.2.0 || fail "bump left something behind"
grep -qF 'ref=v0.2.0&depth=1' "$tree/deploy/terraform/examples/e/main.tf" || fail "bump did not change the source"

if [ "$failures" -gt 0 ]; then
  echo "terraform-scripts-test: $failures failed" >&2
  exit 1
fi
echo "terraform-scripts-test: ok"
