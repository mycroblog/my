#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Stands in for deploy/opentofu/migrate.sh in modules/internal/app-aws-ecs/tests/upgrade.tftest.hcl, which applies on mocked
# providers: it takes what migrate.sh takes, reaches nothing, and succeeds. scripts/tofu-scripts-test.sh tests the script
# itself.
set -euo pipefail

[ "$1" = --version ] && [ -n "$2" ] && [ "$3" = --allow-destructive-for ] || exit 2
jq -e '.platform == "aws-ecs" and .cluster == "my" and .task_definition != "" and .log_prefix == "migrate" and .service == null
  and (.network | fromjson | .awsvpcConfiguration.subnets == ["subnet-1", "subnet-2"])' > /dev/null
