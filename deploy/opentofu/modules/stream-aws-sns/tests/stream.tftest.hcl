# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

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
}

run "the_queues_are_limited_to_the_prefix" {
  command = plan

  assert {
    condition = alltrue([
      for r in output.permissions : r.resource == "arn:aws:sqs:ap-northeast-1:111122223333:my-stream-*" if startswith(r.permission, "sqs:") && r.permission != "sqs:ListQueues"
    ])
    error_message = "the server manages its own queues alone"
  }

  assert {
    condition     = length([for r in output.permissions : r if r.permission == "sns:Subscribe" && length(r.conditions) == 2]) == 1
    error_message = "a subscription must be limited to SQS and to these queues"
  }
}

run "a_long_prefix_fails" {
  command = plan

  variables {
    name = "a-name-that-leaves-the-server-no-room-for-its-process-id"
  }

  expect_failures = [var.name]
}
