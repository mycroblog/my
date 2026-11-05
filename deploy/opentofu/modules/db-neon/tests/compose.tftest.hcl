# SPDX-License-Identifier: AGPL-3.0-only

# Every managed database in front of every app it serves, on mocked providers: nothing here reaches a cloud. It shows that the
# blocks fit and that nothing is circular, not that they run. By plan alone: prevent_destroy refuses the destroy after an apply.

mock_provider "google" {
  mock_resource "google_service_account" {
    defaults = {
      email  = "my-server@p.iam.gserviceaccount.com"
      member = "serviceAccount:my-server@p.iam.gserviceaccount.com"
    }
  }
}

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = {
      region = "ap-northeast-1"
    }
  }
  mock_data "aws_iam_policy_document" {
    defaults = {
      json          = "{}"
      minified_json = "{}"
    }
  }
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/my-server"
    }
  }
  mock_resource "aws_elasticache_serverless_cache" {
    defaults = {
      endpoint = [{ address = "my-valkey-abc.serverless.apne1.cache.amazonaws.com", port = 6379 }]
    }
  }
  mock_resource "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:s-AbCdEf"
    }
  }
}

mock_provider "random" {}

mock_provider "neon" {}

run "each_database_fits_each_app" {
  command = plan

  module {
    source = "../../tests/db-compose"
  }

  assert {
    condition     = output.neon_gcp_service_url == "my-database-url-pooled"
    error_message = "the service must use the pooled connection when it is given one"
  }

  assert {
    condition     = output.neon_gcp_job_url == "my-database-url"
    error_message = "one-off work must use the direct connection"
  }
}

run "a_task_on_ecs_reaches_the_database_and_the_cache_through_one_field" {
  command = plan

  module {
    source = "../../tests/db-compose"
  }

  override_resource {
    target = module.rds.aws_security_group.clients
    values = {
      id = "sg-db-clients"
    }
  }

  override_resource {
    target = module.valkey.aws_security_group.client
    values = {
      id = "sg-cache-clients"
    }
  }

  override_resource {
    target = module.app_on_rds_and_valkey.aws_security_group.task
    values = {
      id = "sg-task"
    }
  }

  assert {
    condition     = toset(output.ecs_task_security_group_ids) == toset(["sg-task", "sg-db-clients", "sg-cache-clients"])
    error_message = "a task must join the groups of both backends"
  }
}
