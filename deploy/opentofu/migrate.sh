#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Runs the migration of an upgrade: `my upgrade` as an execution of the migration job, which already has the new image, and
# waits for it. A public module runs it from an apply, before the workers and the service take the new image, on the machine
# that applies: it exits non-zero when the migration fails, which stops the apply there and leaves the step to run again on
# the next apply of the same version.
#
#   deploy/opentofu/migrate.sh --version VERSION [--allow-destructive-for VERSION] < migration.json
#
# Standard input describes the migration job in the shape of the console output of an app module, without a service
# (console.sh, which runs the command). `my upgrade` gets -allow-destructive only when --allow-destructive-for names the
# version being applied, so that a value left in the configuration lets no later release drop data.
#
# An interrupted apply leaves its execution running on the platform, and only one migration may run at a time: the script
# first waits for any execution of the job that has not ended. Each platform is a function wait_<platform> below.
set -euo pipefail

usage() {
  sed -n '/^#   deploy/p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

version="" allow_destructive_for=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) version="${2:-}" && shift ;;
    --allow-destructive-for) allow_destructive_for="${2:-}" && shift ;;
    *) usage ;;
  esac
  shift
done
[ ! -t 0 ] || usage

# wait_gcp_cloud_run MIGRATION waits until no execution of the job is running. An execution that has ended, whether it
# succeeded, failed, or was cancelled, has a completion time.
wait_gcp_cloud_run() {
  local project region job running waited=0
  project="$(jq -er .project <<< "$1")" || return 1
  region="$(jq -er .region <<< "$1")" || return 1
  job="$(jq -er .job <<< "$1")" || return 1
  while :; do
    running="$(gcloud run jobs executions list --job="$job" --project="$project" --region="$region" \
      --filter='NOT status.completionTime:*' --format='value(metadata.name)')" || return 1
    [ -n "$running" ] || return 0
    if [ "$waited" = 0 ]; then
      echo "migrate.sh: waiting for $(echo "$running" | paste -sd' ' -) of $job, which an earlier apply started, to end"
      waited=1
    fi
    sleep "${MIGRATE_POLL_SECONDS:-10}"
  done
}

migration="$(cat)"
platform="$(jq -er .platform <<< "$migration")" || usage
case "$platform" in
  gcp-cloud-run) wait_gcp_cloud_run "$migration" ;;
  *)
    echo "migrate.sh: this script does not know the platform $platform" >&2
    exit 1
    ;;
esac

command=(upgrade)
if [ -n "$allow_destructive_for" ]; then
  if [ -n "$version" ] && [ "$allow_destructive_for" = "$version" ]; then
    command+=(-allow-destructive)
  else
    echo "migrate.sh: upgrade.allow_destructive_for is $allow_destructive_for, not ${version:-a version}: my upgrade runs without -allow-destructive"
  fi
fi

echo "==> my ${command[*]}${version:+ to $version}"
code=0
"$(dirname "${BASH_SOURCE[0]}")/console.sh" "${command[@]}" <<< "$migration" || code=$?
if [ "$code" != 0 ]; then
  echo "migrate.sh: the migration failed, and the services stay on their revisions. Fix the cause and apply again with the same version: the steps that completed find nothing left to do." >&2
  exit "$code"
fi
