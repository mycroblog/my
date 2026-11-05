#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Runs one `my` command as a one-off, with the image, settings, and secrets of the app, and waits for it. It prints the
# command's output and exits with the command's exit code.
#
#   tofu output -json console | deploy/terraform/console.sh admin create -username alice -email alice@example.com -password-link
#
# Standard input is the `console` output of an app module; the arguments are the `my` command. The command itself gets
# no standard input and no terminal. Each platform is one function below, chosen by the output's `platform`, that takes
# the output as JSON and the command, and returns the command's exit code (deploy/terraform/README.md).
set -euo pipefail

usage() {
  echo "usage: tofu output -json console | console.sh <my command>" >&2
  exit 2
}

# run_gcp_cloud_run CONSOLE ARG... runs the command as an execution of the console job. The output of a job goes to
# Cloud Logging only, so it is read back from there, which needs roles/logging.viewer.
run_gcp_cloud_run() {
  local console="$1"
  shift
  local project region job service
  project="$(jq -er .project <<< "$console")" || return 1
  region="$(jq -er .region <<< "$console")" || return 1
  job="$(jq -er .job <<< "$console")" || return 1
  service="$(jq -er .service <<< "$console")" || return 1
  local where=(--project="$project" --region="$region")

  # The upgrade script changes the service's image and apply does not, so the job may have been created with an older
  # one, and a command of another release must not touch the database.
  local running current
  running="$(gcloud run services describe "$service" "${where[@]}" --format='value(spec.template.spec.containers[0].image)')" || return 1
  current="$(gcloud run jobs describe "$job" "${where[@]}" --format='value(spec.template.spec.template.spec.containers[0].image)')" || return 1
  if [ "$running" != "$current" ]; then
    gcloud run jobs update "$job" "${where[@]}" --image="$running" --quiet >&2 || return 1
  fi

  # gcloud splits --args at commas unless given another delimiter (gcloud topic escaping).
  local delimiter='@@@' arg list=""
  for arg in "$@"; do
    case "$arg" in
      *"$delimiter"*)
        echo "console.sh: an argument must not contain $delimiter" >&2
        return 2
        ;;
    esac
    list="$list${list:+$delimiter}$arg"
  done

  local status=0
  gcloud run jobs execute "$job" "${where[@]}" --args="^$delimiter^$list" --wait --quiet > /dev/null 2>&1 || status=$?

  local execution
  execution="$(gcloud run jobs executions list --job="$job" "${where[@]}" --limit=1 --sort-by=~metadata.creationTimestamp --format='value(metadata.name)')"
  if [ -z "$execution" ]; then
    echo "console.sh: the job did not start; see: gcloud run jobs describe $job --project=$project --region=$region" >&2
    return "$((status == 0 ? 1 : status))"
  fi

  local filter="resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"$job\""
  filter="$filter AND labels.\"run.googleapis.com/execution_name\"=\"$execution\""
  filter="$filter AND (logName=\"projects/$project/logs/run.googleapis.com%2Fstdout\" OR logName=\"projects/$project/logs/run.googleapis.com%2Fstderr\")"

  # Logs arrive a little after the execution ends: read until two reads in a row agree, for a minute at most.
  local entries="[]" previous="" attempt
  for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
    entries="$(gcloud logging read "$filter" --project="$project" --order=asc --freshness=1d --format=json)" || entries="[]"
    if [ "$entries" != "[]" ] && [ "$entries" = "$previous" ]; then
      break
    fi
    previous="$entries"
    [ "$attempt" = 12 ] || sleep "${CONSOLE_LOG_WAIT_SECONDS:-5}"
  done
  if [ "$entries" = "[]" ]; then
    echo "console.sh: no output found for $execution yet; reading it needs roles/logging.viewer" >&2
  fi
  jq -r '.[] | .textPayload // (.jsonPayload | tojson)' <<< "$entries"

  local code
  code="$(gcloud run jobs executions tasks list --execution="$execution" "${where[@]}" --format='value(status.lastAttemptResult.exitCode)' 2> /dev/null | head -n 1)" || code=""
  case "$code" in
    '' | *[!0-9]*) code="$status" ;;
  esac
  # A task can fail with no exit code of its own, such as on the job's timeout.
  if [ "$status" -ne 0 ] && [ "$code" -eq 0 ]; then
    code="$status"
  fi
  return "$code"
}

# run_aws_lambda CONSOLE ARG... runs the command in one invocation of the console function, which takes the arguments as
# {"args":[...]} and answers {"exit_code":N,"output":"..."}. No released image answers that yet: the module's README says so.
# An invocation lasts 15 minutes at most and its answer 6 MB. No retry: a command must not run twice because a socket closed.
run_aws_lambda() {
  local console="$1"
  shift
  local region function service
  region="$(jq -er .region <<< "$console")" || return 1
  function="$(jq -er .function <<< "$console")" || return 1
  service="$(jq -r '.service // empty' <<< "$console")"

  # As on the other platforms: apply does not follow the upgrade script's image, and a command of another release must not
  # touch the database. Absent when the caller has already chosen the image.
  if [ -n "$service" ]; then
    local running current
    running="$(aws lambda get-function --function-name "$service" --region "$region" --query Code.ImageUri --output text)" || return 1
    current="$(aws lambda get-function --function-name "$function" --region "$region" --query Code.ImageUri --output text)" || return 1
    if [ "$running" != "$current" ]; then
      aws lambda update-function-code --function-name "$function" --region "$region" --image-uri "$running" > /dev/null || return 1
      aws lambda wait function-updated --function-name "$function" --region "$region" || return 1
    fi
  fi

  local payload answer meta
  payload="$(printf '%s\0' "$@" | jq -Rsc '{args: (split("\u0000")[:-1])}')" || return 1
  answer="$(mktemp)"
  meta="$(mktemp)"
  # shellcheck disable=SC2064 # The names are set here and must be expanded now.
  trap "rm -f '$answer' '$meta'" RETURN
  AWS_MAX_ATTEMPTS=1 aws lambda invoke --function-name "$function" --region "$region" \
    --cli-binary-format raw-in-base64-out --cli-read-timeout 0 --payload "$payload" "$answer" > "$meta" || return 1

  # The function itself failed, such as on the 15 minutes: the command may have done part of its work.
  if [ -n "$(jq -r '.FunctionError // empty' "$meta")" ]; then
    echo "console.sh: the invocation failed, and the command may have stopped part way: $(jq -r '.errorMessage // "see the logs of the function"' "$answer")" >&2
    return 1
  fi
  jq -r '.output // empty' "$answer"
  local code
  code="$(jq -er .exit_code "$answer")" || {
    echo "console.sh: the function did not answer with an exit code: is its image built for it?" >&2
    return 1
  }
  return "$code"
}

[ "$#" -gt 0 ] || usage
[ ! -t 0 ] || usage

console="$(cat)"
if [ -z "$console" ] || [ "$console" = "null" ]; then
  echo "console.sh: the console is off: set console_enabled = true and apply" >&2
  exit 1
fi
platform="$(jq -er .platform <<< "$console")"

code=0
case "$platform" in
  gcp-cloud-run) run_gcp_cloud_run "$console" "$@" || code=$? ;;
  aws-lambda) run_aws_lambda "$console" "$@" || code=$? ;;
  *)
    echo "console.sh: this script does not know the platform $platform" >&2
    exit 1
    ;;
esac
echo "console.sh: exit code $code" >&2
exit "$code"
