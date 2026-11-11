# SPDX-License-Identifier: AGPL-3.0-only

# An upgrade by apply, on mocked providers with a stand-in for migrate.sh: nothing here reaches a cloud. Each run applies, and
# the next compares its migration with the one before, through previous_migration. That the workers move after the migration,
# and the service after the workers, is the order of depends_on, which no assertion here can watch.

mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = {
      json          = "{}"
      minified_json = "{}"
    }
  }
  mock_data "aws_region" {
    defaults = {
      region = "ap-northeast-1"
    }
  }
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/my"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:ap-northeast-1:111122223333:log-group:/ecs/my"
    }
  }
  mock_resource "aws_ecs_cluster" {
    defaults = {
      arn = "arn:aws:ecs:ap-northeast-1:111122223333:cluster/my"
    }
  }
  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn = "arn:aws:ecs:ap-northeast-1:111122223333:task-definition/my-migrate:1"
    }
  }
}

run "the_first_deployment_migrates" {
  module {
    source = "../../../tests/upgrade-aws"
  }

  variables {
    version_applied = "0.1.0"
  }

  assert {
    condition     = output.migration_id != null
    error_message = "the first deployment must run the migration before the services"
  }

  assert {
    condition     = output.images == { service = "ghcr.io/mycroblog/my:0.1.0", workers = { delivery = "ghcr.io/mycroblog/my:0.1.0" }, migration = "ghcr.io/mycroblog/my:0.1.0" }
    error_message = "the migration, the worker, and the service must take the image"
  }
}

run "the_same_version_replaces_nothing" {
  module {
    source = "../../../tests/upgrade-aws"
  }

  variables {
    version_applied    = "0.1.0"
    previous_migration = run.the_first_deployment_migrates.migration_id
  }

  assert {
    condition     = output.migration_id == var.previous_migration
    error_message = "an apply of the same version must not run the migration again"
  }
}

run "another_version_migrates_and_moves_the_services" {
  module {
    source = "../../../tests/upgrade-aws"
  }

  variables {
    version_applied    = "0.2.0"
    previous_migration = run.the_same_version_replaces_nothing.migration_id
  }

  assert {
    condition     = output.migration_id != var.previous_migration
    error_message = "a new version must replace the migration, which runs it again"
  }

  assert {
    condition     = output.images == { service = "ghcr.io/mycroblog/my:0.2.0", workers = { delivery = "ghcr.io/mycroblog/my:0.2.0" }, migration = "ghcr.io/mycroblog/my:0.2.0" }
    error_message = "an apply of a new version must give every task definition the image"
  }
}
