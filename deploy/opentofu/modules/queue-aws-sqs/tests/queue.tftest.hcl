# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/my-jobs-scheduler"
    }
  }
  mock_resource "aws_sqs_queue" {
    defaults = {
      arn = "arn:aws:sqs:ap-northeast-1:111122223333:my-jobs"
      url = "https://sqs.ap-northeast-1.amazonaws.com/111122223333/my-jobs"
    }
  }
}

run "the_tick_goes_through_the_queue" {
  assert {
    condition     = aws_scheduler_schedule.tick.schedule_expression == "rate(1 minute)" && jsondecode(aws_scheduler_schedule.tick.target[0].input).type == "tick" && aws_scheduler_schedule.tick.target[0].arn == aws_sqs_queue.jobs.arn
    error_message = "the server takes the message {\"type\":\"tick\"} from the queue"
  }

  assert {
    condition     = output.env["MY_QUEUE"] == "sqs" && output.env["MY_QUEUE_SQS_QUEUE_URL"] == aws_sqs_queue.jobs.url
    error_message = "the server must know the queue"
  }

  assert {
    condition     = output.event_sources["jobs"].arn == aws_sqs_queue.jobs.arn && output.event_sources["jobs"].visibility_timeout_seconds == 5400
    error_message = "the app maps the queue to its worker"
  }

  assert {
    condition     = one(output.permissions).permission == "sqs:SendMessage" && one(output.permissions).scope == "policy"
    error_message = "the server only sends to the queue; polling is the event source's"
  }
}
