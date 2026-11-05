#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Upgrades a Cloud Run deployment of deploy/opentofu to another image: the migration job first, then the service, and
# it stops at the first step that fails, with the service still on its revision. Back up the database first.
#
#   deploy/opentofu/upgrade.sh --project P --region R --image ghcr.io/mycroblog/my:0.2.0 [--name my]
#       [--maintenance]         switch the service to the maintenance mode first, and back with the new revision
#       [--allow-destructive]   pass -allow-destructive to my schema apply, when the release notes call for it
#       [--plan]                only print what the upgrade would run: upgrade-check and my schema apply -dry-run
#       [--migrate-only]        stop before the service, for the first deployment, when it does not exist yet
set -euo pipefail

usage() {
  sed -n '/^#   deploy/,/^#       \[--migrate-only/p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

project="" region="" image="" name="my"
maintenance=0 allow_destructive=0 plan=0 migrate_only=0
while [ "$#" -gt 0 ]; do
  case "$1" in
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
if [ -z "$project" ] || [ -z "$region" ] || [ -z "$image" ]; then
  usage
fi

where=(--project="$project" --region="$region")
job="$name-migrate"

failed() {
  echo "upgrade.sh: stopped: the step above failed, and the service stays on its revision." >&2
  echo "upgrade.sh: the logs: gcloud logging read 'resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"$job\"' --project=$project --freshness=1h" >&2
  if [ "$maintenance" = 1 ]; then
    echo "upgrade.sh: the service is still in the maintenance mode; see the guide before you switch it back." >&2
  fi
  exit 1
}

# step ARG... runs one my command in the migration job and waits for it. Only one runs at a time.
step() {
  echo "==> my $*"
  local list="" arg
  for arg in "$@"; do
    list="$list${list:+,}$arg"
  done
  gcloud run jobs execute "$job" "${where[@]}" --args="$list" --wait --quiet || failed
}

if [ "$maintenance" = 1 ] && [ "$plan" = 0 ] && [ "$migrate_only" = 0 ]; then
  echo "==> the maintenance mode"
  gcloud run services update "$name" "${where[@]}" --update-env-vars=MY_MAINTENANCE=true --quiet || failed
fi

echo "==> $job takes $image"
gcloud run jobs update "$job" "${where[@]}" --image="$image" --quiet || failed

step config check

if [ "$plan" = 1 ]; then
  step db upgrade -check
  step schema apply -dry-run
  echo "The SQL is in the logs: gcloud logging read 'resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"$job\"' --project=$project --freshness=1h --format='value(textPayload)' --order=asc"
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

# gcloud waits for the new revision's startup probe on /readyz. A revision that fails it takes no traffic.
echo "==> $name takes $image"
args=(--image="$image")
if [ "$maintenance" = 1 ]; then
  args+=(--remove-env-vars=MY_MAINTENANCE)
fi
gcloud run services update "$name" "${where[@]}" "${args[@]}" --quiet || failed
echo "Upgraded to $image."
