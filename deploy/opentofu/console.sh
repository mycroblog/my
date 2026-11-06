#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Runs one `my` command as a one-off, with the image, settings, and secrets of the app, and waits for it. It prints the
# command's output and exits with the command's exit code.
#
#   tofu output -json console | deploy/opentofu/console.sh admin create -username alice -email alice@example.com -password-link
#
# Standard input is the `console` output of an app module; the arguments are the `my` command. The command itself gets
# no standard input and no terminal. Each platform is one function below, chosen by the output's `platform`, that takes
# the output as JSON and the command, and returns the command's exit code (deploy/opentofu/README.md).
set -euo pipefail

# shellcheck source=/dev/null # Beside this script, at the same release.
. "$(dirname "${BASH_SOURCE[0]}")/aws-ecs.sh"

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

# run_aws_ecs CONSOLE ARG... runs the command as a one-off task and waits for it, for as long as it takes. The output of a task
# goes to CloudWatch Logs only, so it is read back from there, which needs logs:FilterLogEvents. No retry of the start: a command
# must not run twice because a socket closed.
run_aws_ecs() {
  local console="$1"
  shift
  local region cluster family container network log_group log_prefix service
  region="$(jq -er .region <<< "$console")" || return 1
  cluster="$(jq -er .cluster <<< "$console")" || return 1
  family="$(jq -er .task_definition <<< "$console")" || return 1
  container="$(jq -er .container <<< "$console")" || return 1
  network="$(jq -er .network <<< "$console")" || return 1
  log_group="$(jq -er .log_group <<< "$console")" || return 1
  log_prefix="$(jq -er .log_prefix <<< "$console")" || return 1
  service="$(jq -r '.service // empty' <<< "$console")"

  # As on the other platforms: apply follows the upgrade script only through what it reads, and a command of another release must
  # not touch the database. Absent when the caller has already chosen the image.
  if [ -n "$service" ]; then
    local running_definition running current
    running_definition="$(aws ecs describe-services --cluster "$cluster" --services "$service" --region "$region" --query 'services[0].taskDefinition' --output text)" || return 1
    running="$(ecs_image "$region" "$running_definition")" || return 1
    current="$(ecs_image "$region" "$family")" || return 1
    if [ "$running" != "$current" ]; then
      # shellcheck disable=SC2016 # For jq.
      ecs_new_revision "$region" "$family" '.image = $image' --arg image "$running" > /dev/null || return 1
    fi
  fi

  local overrides started task
  overrides="$(printf '%s\0' "$@" | jq -Rsc --arg container "$container" '{containerOverrides: [{name: $container, command: (split("\u0000")[:-1])}]}')" || return 1
  started="$(AWS_MAX_ATTEMPTS=1 aws ecs run-task --cluster "$cluster" --task-definition "$family" --region "$region" --launch-type FARGATE \
    --count 1 --started-by console.sh --network-configuration "$network" --overrides "$overrides" --output json)" || return 1
  task="$(jq -r '.tasks[0].taskArn // empty' <<< "$started")"
  if [ -z "$task" ]; then
    echo "console.sh: the task did not start: $(jq -c '.failures' <<< "$started")" >&2
    return 1
  fi

  # A failed read does not mean the task failed: it keeps running, so keep asking for a while.
  local described="" unread=0
  while :; do
    if described="$(aws ecs describe-tasks --cluster "$cluster" --tasks "$task" --region "$region" --query 'tasks[0]' --output json)"; then
      unread=0
      [ "$(jq -r .lastStatus <<< "$described")" != "STOPPED" ] || break
    else
      unread=$((unread + 1))
      if [ "$unread" -ge 12 ]; then
        echo "console.sh: cannot read the task, which may still be running: $task" >&2
        return 1
      fi
    fi
    sleep "${CONSOLE_POLL_SECONDS:-5}"
  done

  # Logs arrive a little after the task ends: read until two reads in a row agree, for a minute at most.
  local stream="$log_prefix/$container/${task##*/}" entries="[]" previous="" attempt
  for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
    entries="$(aws logs filter-log-events --log-group-name "$log_group" --log-stream-names "$stream" --region "$region" --query 'events[].message' --output json)" || entries="[]"
    if [ "$entries" != "[]" ] && [ "$entries" = "$previous" ]; then
      break
    fi
    previous="$entries"
    [ "$attempt" = 12 ] || sleep "${CONSOLE_LOG_WAIT_SECONDS:-5}"
  done
  if [ "$entries" = "[]" ] || [ "$entries" = "null" ]; then
    echo "console.sh: no output found in $log_group/$stream yet; reading it needs logs:FilterLogEvents" >&2
  else
    jq -r '.[]' <<< "$entries"
  fi

  # A task that could not start, such as on an image it cannot pull, has no exit code of its own.
  local code
  code="$(jq -r --arg container "$container" '.containers[] | select(.name == $container) | .exitCode // empty' <<< "$described")"
  if [ -z "$code" ]; then
    echo "console.sh: the task stopped with no exit code: $(jq -r '.stoppedReason // "see the task"' <<< "$described")" >&2
    return 1
  fi
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
  aws-ecs) run_aws_ecs "$console" "$@" || code=$? ;;
  *)
    echo "console.sh: this script does not know the platform $platform" >&2
    exit 1
    ;;
esac
echo "console.sh: exit code $code" >&2
exit "$code"
