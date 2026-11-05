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
    condition     = one(output.permissions).permission == "sqs:SendMessage"
    error_message = "the server only sends to the queue; polling is the event source's"
  }
}

run "no_workers_is_one_queue" {
  command = plan

  assert {
    condition     = length(aws_sqs_queue.worker) == 0 && !contains(keys(output.env), "MY_QUEUE_SQS_ROUTES") && toset(keys(output.event_sources)) == toset(["jobs"]) && length(output.workers) == 0
    error_message = "no workers must leave the queue as it is"
  }
}

run "a_worker_is_a_queue_and_a_route_from_its_key" {
  command = apply

  override_resource {
    target = aws_sqs_queue.worker
    values = {
      arn = "arn:aws:sqs:r:1:my-jobs-delivery"
      url = "https://sqs.r.amazonaws.com/1/my-jobs-delivery"
    }
  }

  variables {
    workers = {
      delivery = { job_types = ["federation.*"], concurrency = 4 }
    }
  }

  assert {
    condition     = aws_sqs_queue.worker["delivery"].name == "my-jobs-delivery" && output.env["MY_QUEUE_SQS_ROUTES"] == "federation.*=https://sqs.r.amazonaws.com/1/my-jobs-delivery"
    error_message = "the queue and the route must come from the key"
  }

  assert {
    condition     = toset(keys(output.event_sources)) == toset(["jobs", "delivery"]) && length(output.permissions) == 2
    error_message = "the app must get the queue to poll, and the server the right to send to it"
  }
}

run "a_split_fails_here_too" {
  command = plan

  variables {
    workers = {
      delivery = { job_types = ["federation.deliver"] }
    }
  }

  expect_failures = [aws_sqs_queue.jobs]
}
