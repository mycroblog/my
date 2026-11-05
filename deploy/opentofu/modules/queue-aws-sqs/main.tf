# SPDX-License-Identifier: AGPL-3.0-only

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

variable "name" {
  description = "Name of the queue and start of the names of the other resources."
  type        = string
  default     = "my-jobs"
  nullable    = false
}

variable "tick_schedule" {
  description = "Schedule expression of the tick. Each tick invokes the worker and touches the database, so it decides whether a scale-to-zero database can stop."
  type        = string
  default     = "rate(1 minute)"
  nullable    = false
}

variable "visibility_timeout_seconds" {
  description = "How long a message stays hidden once delivered. AWS asks for at least six times the function's timeout; the default is six times the longest timeout."
  type        = number
  default     = 5400
  nullable    = false
}

variable "message_retention_seconds" {
  description = "How long the queue keeps a signal. The jobs are in the database, and a tick signals again what was lost."
  type        = number
  default     = 345600
  nullable    = false
}

# A standard queue: the server refuses nothing else, and a signal delivered twice runs a job once.
resource "aws_sqs_queue" "jobs" {
  name                       = var.name
  visibility_timeout_seconds = var.visibility_timeout_seconds
  message_retention_seconds  = var.message_retention_seconds
  sqs_managed_sse_enabled    = true
}

resource "aws_iam_role" "scheduler" {
  name = "${var.name}-scheduler"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "scheduler.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy" "scheduler" {
  name = "${var.name}-scheduler"
  role = aws_iam_role.scheduler.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "sqs:SendMessage"
      Resource = aws_sqs_queue.jobs.arn
    }]
  })
}

# The tick goes through the same queue as the signals, as the server expects: there is no URL to call.
resource "aws_scheduler_schedule" "tick" {
  name                = "${var.name}-tick"
  schedule_expression = var.tick_schedule

  flexible_time_window {
    mode = "OFF"
  }

  target {
    arn      = aws_sqs_queue.jobs.arn
    role_arn = aws_iam_role.scheduler.arn
    input    = jsonencode({ type = "tick" })

    # A tick that fails is made up for by the next one.
    retry_policy {
      maximum_retry_attempts = 0
    }
  }

  depends_on = [aws_iam_role_policy.scheduler]
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The settings of the queue."
  value = {
    MY_QUEUE               = "sqs"
    MY_QUEUE_SQS_QUEUE_URL = aws_sqs_queue.jobs.url
  }
}

output "secret_env" {
  description = "No secrets: the server authenticates with its execution role."
  value       = {}
}

output "permissions" {
  description = "Sending the signals to the queue."
  value = [
    { permission = "sqs:SendMessage", resource = aws_sqs_queue.jobs.arn },
  ]
}

output "event_sources" {
  description = "The queue that invokes the worker, which the app module maps to its function. The key is known at plan time."
  value = {
    jobs = {
      arn                        = aws_sqs_queue.jobs.arn
      visibility_timeout_seconds = var.visibility_timeout_seconds
    }
  }
}
