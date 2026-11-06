# SPDX-License-Identifier: AGPL-3.0-only

# The value in the real blocks, on mocked providers: nothing here reaches a cloud. It shows that the blocks fit, that nothing is
# circular, and that a queue and an app derive the same names, not that the services run.

mock_provider "google" {
  mock_data "google_project" {
    defaults = {
      number = "123456789012"
    }
  }
  mock_resource "google_service_account" {
    defaults = {
      email  = "my-server@p.iam.gserviceaccount.com"
      member = "serviceAccount:my-server@p.iam.gserviceaccount.com"
    }
  }
}

mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "111122223333"
    }
  }
  mock_data "aws_partition" {
    defaults = {
      partition = "aws"
    }
  }
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
      arn = "arn:aws:iam::111122223333:role/my"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:ap-northeast-1:111122223333:log-group:/ecs/my"
    }
  }
}

run "cloud_tasks_and_the_app_agree_on_names" {
  command = plan

  module {
    source = "../../tests/workers-gcp"
  }

  assert {
    condition     = output.worker_service == "my-delivery" && strcontains(output.routes, "@https://my-delivery-123456789012.asia-northeast1.run.app")
    error_message = "the route must name the service that the app builds"
  }

  assert {
    condition     = output.worker_routes == output.routes && output.default_routes == output.routes
    error_message = "every process must hold the same routes, or a split is lost with no error"
  }

  assert {
    condition     = output.worker_db_env["MY_WORKER_JOB_TYPES"] == "federation.*" && output.worker_db_env["MY_WORKER_JOB_CONCURRENCY"] == "3" && output.default_db_env["MY_WORKER_JOB_TYPES_EXCEPT"] == "federation.*"
    error_message = "with queue-db the worker and the default must split by the types"
  }
}

run "the_queue_in_the_database_and_ecs_agree" {
  command = plan

  module {
    source = "../../tests/workers-aws"
  }

  assert {
    condition     = output.ecs_worker_env["MY_WORKER_JOB_TYPES"] == "federation.deliver,federation.fanout" && output.ecs_env["MY_WORKER_JOB_TYPES_EXCEPT"] == "federation.deliver,federation.fanout"
    error_message = "the worker and the default must split by the types"
  }
}
