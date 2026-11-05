# shellcheck shell=bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# What console.sh and upgrade.sh share on Amazon ECS, sourced by both. A task definition is an immutable revision that holds the
# image and every setting, so changing the image means registering a new revision of the family from the latest one.

# ecs_image REGION TASK_DEFINITION prints the image of a family's latest revision, or of one revision.
ecs_image() {
  aws ecs describe-task-definition --task-definition "$2" --region "$1" \
    --query 'taskDefinition.containerDefinitions[0].image' --output text
}

# ecs_new_revision REGION FAMILY FILTER [JQ ARGUMENT...] registers a revision of the family whose container is what FILTER makes
# of the latest one's, and prints its ARN. It keeps every other field, leaving out those that ECS sets itself and refuses.
ecs_new_revision() {
  local region="$1" family="$2" filter="$3" current input
  shift 3
  current="$(aws ecs describe-task-definition --task-definition "$family" --region "$region" --include TAGS --output json)" || return 1
  input="$(jq -c "$@" "(.taskDefinition + (if (.tags // []) | length > 0 then {tags: .tags} else {} end))
    | del(.taskDefinitionArn, .revision, .status, .requiresAttributes, .compatibilities, .registeredAt, .registeredBy, .deregisteredAt)
    | .containerDefinitions[0] |= ($filter)" <<< "$current")" || return 1
  aws ecs register-task-definition --region "$region" --cli-input-json "$input" \
    --query 'taskDefinition.taskDefinitionArn' --output text
}
