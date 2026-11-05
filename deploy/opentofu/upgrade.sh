#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Upgrades a deployment of deploy/opentofu to another image: the migration job first, then the service, and it stops at
# the first step that fails, with the service still on its image. Back up the database first.
#
#   deploy/opentofu/upgrade.sh --project P --region R --image ghcr.io/mycroblog/my:0.2.0 [--name my]
#   deploy/opentofu/upgrade.sh --platform aws-lambda --region R --image <ECR image> [--name my]
#   deploy/opentofu/upgrade.sh --platform aws-ecs --region R --image <image> [--name my]
#       [--maintenance]         switch the service to the maintenance mode first, and back with the new revision
#       [--allow-destructive]   pass -allow-destructive to my schema apply, when the release notes call for it
#       [--plan]                only print what the upgrade would run: upgrade-check and my schema apply -dry-run
#       [--migrate-only]        stop before the service, for the first deployment, when it does not exist yet
#
# The order of the steps is the same on every platform; each platform is the functions gcp_* or aws_* below. On aws-lambda
# nothing has run on AWS, and a step needs an image that answers the function's invocation: read modules/app-aws-lambda. On
# aws-ecs nothing has run on AWS either: read modules/app-aws-ecs.
set -euo pipefail

# shellcheck source=/dev/null # Beside this script, at the same release.
. "$(dirname "${BASH_SOURCE[0]}")/aws-ecs.sh"

usage() {
  sed -n '/^#   deploy/,/^#       \[--migrate-only/p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

platform="gcp-cloud-run"
project="" region="" image="" name="my"
maintenance=0 allow_destructive=0 plan=0 migrate_only=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --platform) platform="${2:-}" && shift ;;
    --project) project="${2:-}" && shift ;;
    --region) region="${2:-}" && shift ;;
    --image) image="${2:-}" && shift ;;
    --name) name="${2:-}" && shift ;;
    --maintenance) maintenance=1 ;;
    --allow-destructive) allow_destructive=1 ;;
    --plan) plan=1 ;;
    --migrate-only) migrate_only=1 ;;
    *) usage ;;
  esac
  shift
done
case "$platform" in
  gcp-cloud-run) [ -n "$project" ] || usage ;;
  aws-lambda | aws-ecs) ;;
  *) usage ;;
esac
if [ -z "$region" ] || [ -z "$image" ]; then
  usage
fi

where=(--project="$project" --region="$region")
job="$name-migrate"

gcp_logs_hint() {
  echo "gcloud logging read 'resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"$job\"' --project=$project --freshness=1h"
}

gcp_maintenance_on() {
  gcloud run services update "$name" "${where[@]}" --update-env-vars=MY_MAINTENANCE=true --quiet
}

gcp_one_off_takes_image() {
  gcloud run jobs update "$job" "${where[@]}" --image="$image" --quiet
}

# gcp_step ARG... runs one my command in the migration job and waits for it.
gcp_step() {
  local list="" arg
  for arg in "$@"; do
    list="$list${list:+,}$arg"
  done
  gcloud run jobs execute "$job" "${where[@]}" --args="$list" --wait --quiet
}

gcp_plan_hint() {
  echo "The SQL is in the logs: gcloud logging read 'resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"$job\"' --project=$project --freshness=1h --format='value(textPayload)' --order=asc"
}

# gcloud waits for the new revision's startup probe on /readyz. A revision that fails it takes no traffic.
gcp_service_takes_image() {
  local args=(--image="$image")
  if [ "$maintenance" = 1 ]; then
    args+=(--remove-env-vars=MY_MAINTENANCE)
  fi
  gcloud run services update "$name" "${where[@]}" "${args[@]}" --quiet
}

aws_logs_hint() {
  echo "aws logs tail /aws/lambda/$job --region $region --since 1h"
}

# aws_set_env FUNCTION JQ replaces the function's variables, since the API takes them only as a whole.
aws_set_env() {
  local variables
  variables="$(aws lambda get-function-configuration --function-name "$1" --region "$region" --query Environment.Variables --output json)" || return 1
  variables="$(jq -c "{Variables: ($variables | $2)}" <<< 'null')" || return 1
  aws lambda update-function-configuration --function-name "$1" --region "$region" --environment "$variables" > /dev/null || return 1
  aws lambda wait function-updated --function-name "$1" --region "$region"
}

aws_takes_image() {
  aws lambda update-function-code --function-name "$1" --region "$region" --image-uri "$image" > /dev/null || return 1
  aws lambda wait function-updated --function-name "$1" --region "$region"
}

aws_maintenance_on() {
  aws_set_env "$name-worker" '. + {MY_MAINTENANCE: "true"}' && aws_set_env "$name-web" '. + {MY_MAINTENANCE: "true"}'
}

aws_one_off_takes_image() {
  aws_takes_image "$job"
}

# aws_step ARG... runs one my command in one invocation of the migration function, which lasts 15 minutes at most.
aws_step() {
  "$(dirname "$0")/console.sh" "$@" <<< "$(jq -nc --arg region "$region" --arg function "$job" '{platform: "aws-lambda", region: $region, function: $function}')"
}

aws_plan_hint() {
  echo "The SQL is in the output above, and in the logs: $(aws_logs_hint)"
}

# Unlike Cloud Run, nothing holds a new image back: the function takes traffic as soon as it is updated, and a request that
# starts one on an image that fails /readyz gets an error. The worker goes first, since the web function enqueues for it.
aws_service_takes_image() {
  local function
  for function in "$name-worker" "$name-web"; do
    aws_takes_image "$function" || return 1
    if [ "$maintenance" = 1 ]; then
      aws_set_env "$function" 'del(.MY_MAINTENANCE)' || return 1
    fi
  done
}

ecs_logs_hint() {
  echo "aws logs tail /ecs/$name --region $region --since 1h"
}

# ecs_deploy TASK_DEFINITION moves the service to a revision and waits for the outcome. A deployment that fails goes back on its
# own, which leaves the service stable on the old revision, so it is the state of this revision's deployment that counts.
ecs_deploy() {
  local definition="$1" deployments state polls=0
  aws ecs update-service --cluster "$name" --service "$name" --task-definition "$definition" --region "$region" > /dev/null || return 1
  while :; do
    deployments="$(aws ecs describe-services --cluster "$name" --services "$name" --region "$region" --query 'services[0].deployments' --output json)" || return 1
    state="$(jq -r --arg definition "$definition" '[.[] | select(.taskDefinition == $definition)][0].rolloutState // "GONE"' <<< "$deployments")"
    case "$state" in
      COMPLETED) return 0 ;;
      FAILED | GONE)
        echo "upgrade.sh: the deployment of $definition failed and goes back: aws ecs describe-services --cluster $name --services $name --region $region --query 'services[0].events[:10]'" >&2
        return 1
        ;;
    esac
    polls=$((polls + 1))
    if [ "$polls" -ge 720 ]; then
      echo "upgrade.sh: the deployment of $definition is still in progress after an hour" >&2
      return 1
    fi
    sleep "${UPGRADE_POLL_SECONDS:-5}"
  done
}

ecs_maintenance_on() {
  local definition
  definition="$(ecs_new_revision "$region" "$name" '.environment = ([.environment[] | select(.name != "MY_MAINTENANCE")] + [{name: "MY_MAINTENANCE", value: "true"}])')" || return 1
  ecs_deploy "$definition"
}

ecs_one_off_takes_image() {
  if [ "$(ecs_image "$region" "$job")" != "$image" ]; then
    # shellcheck disable=SC2016 # For jq.
    ecs_new_revision "$region" "$job" '.image = $image' --arg image "$image" > /dev/null
  fi
}

# ecs_step ARG... runs one my command as a one-off task. Its network is in a parameter, since the service may not exist yet.
ecs_step() {
  local network
  network="$(aws ssm get-parameter --name "/my/$name/network" --region "$region" --query Parameter.Value --output text)" || return 1
  "$(dirname "$0")/console.sh" "$@" <<< "$(jq -nc --arg region "$region" --arg cluster "$name" --arg family "$job" --arg network "$network" --arg log_group "/ecs/$name" \
    '{platform: "aws-ecs", region: $region, cluster: $cluster, task_definition: $family, container: "my", network: $network, log_group: $log_group, log_prefix: "migrate"}')"
}

ecs_plan_hint() {
  echo "The SQL is in the output above, and in the logs: $(ecs_logs_hint)"
}

# A new task takes traffic only once the load balancer's health check on /readyz passes, and the old one stays until then. The
# parameter comes last and is what apply reads: until it changes, apply puts the image back.
ecs_service_takes_image() {
  # shellcheck disable=SC2016 # For jq.
  local filter='.image = $image' definition
  if [ "$maintenance" = 1 ]; then
    filter="$filter | .environment |= map(select(.name != \"MY_MAINTENANCE\"))"
  fi
  definition="$(ecs_new_revision "$region" "$name" "$filter" --arg image "$image")" || return 1
  ecs_deploy "$definition" || return 1
  aws ssm put-parameter --name "/my/$name/image" --value "$image" --type String --overwrite --region "$region" > /dev/null
}

case "$platform" in
  gcp-cloud-run) prefix=gcp ;;
  aws-lambda) prefix=aws ;;
  aws-ecs) prefix=ecs ;;
esac

failed() {
  echo "upgrade.sh: stopped: the step above failed, and the service stays on its revision." >&2
  echo "upgrade.sh: the logs: $("${prefix}_logs_hint")" >&2
  if [ "$maintenance" = 1 ]; then
    echo "upgrade.sh: the service is still in the maintenance mode; see the guide before you switch it back." >&2
  fi
  exit 1
}

# step ARG... runs one my command and waits for it. Only one runs at a time.
step() {
  echo "==> my $*"
  "${prefix}_step" "$@" || failed
}

if [ "$maintenance" = 1 ] && [ "$plan" = 0 ] && [ "$migrate_only" = 0 ]; then
  echo "==> the maintenance mode"
  "${prefix}_maintenance_on" || failed
fi

echo "==> $job takes $image"
"${prefix}_one_off_takes_image" || failed

step config check

if [ "$plan" = 1 ]; then
  step db upgrade -check
  step schema apply -dry-run
  "${prefix}_plan_hint"
  exit 0
fi

step db upgrade -check
if [ "$allow_destructive" = 1 ]; then
  step schema apply -allow-destructive
else
  step schema apply
fi
step db upgrade

if [ "$migrate_only" = 1 ]; then
  echo "The database is ready for $image. The service was not changed."
  exit 0
fi

echo "==> $name takes $image"
"${prefix}_service_takes_image" || failed
echo "Upgraded to $image."
