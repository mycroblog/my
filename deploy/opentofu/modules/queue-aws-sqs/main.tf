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

variable "workers" {
  description = "The same workers as the app takes. Each gets a queue of its own, its job types a route to it, and the app a function that the queue invokes, all named by the key. Empty sends everything to the one queue."
  type        = any
  default     = {}
  nullable    = false
}

module "workers" {
  source = "../workers"

  workers = var.workers
  fields  = ["memory_mib", "concurrency"]
}

# A standard queue: the server refuses nothing else, and a signal delivered twice runs a job once.
resource "aws_sqs_queue" "jobs" {
  name                       = var.name
  visibility_timeout_seconds = var.visibility_timeout_seconds
  message_retention_seconds  = var.message_retention_seconds
  sqs_managed_sse_enabled    = true

  lifecycle {
    precondition {
      condition     = length(module.workers.problems) == 0
      error_message = "workers: ${join(" ", module.workers.problems)}"
    }
  }
}

resource "aws_sqs_queue" "worker" {
  for_each = module.workers.workers

  name                       = "${var.name}-${each.key}"
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
  value = merge(
    {
      MY_QUEUE               = "sqs"
      MY_QUEUE_SQS_QUEUE_URL = aws_sqs_queue.jobs.url
    },
    length(var.workers) == 0 ? {} : {
      MY_QUEUE_SQS_ROUTES = join(",", flatten([
        for key, w in module.workers.workers : [for pattern in w.job_types : "${pattern}=${aws_sqs_queue.worker[key].url}"]
      ]))
    },
  )
}

output "secret_env" {
  description = "No secrets: the server authenticates with its execution role."
  value       = {}
}

output "permissions" {
  description = "Sending the signals to the queue."
  value = concat(
    [{ permission = "sqs:SendMessage", resource = aws_sqs_queue.jobs.arn }],
    [for q in values(aws_sqs_queue.worker) : { permission = "sqs:SendMessage", resource = q.arn }],
  )
}

output "event_sources" {
  description = "The queues that invoke the workers, which the app module maps to its functions: jobs to the default, and each worker's key to that worker's. The keys are known at plan time."
  value = merge(
    {
      jobs = {
        arn                        = aws_sqs_queue.jobs.arn
        visibility_timeout_seconds = var.visibility_timeout_seconds
      }
    },
    {
      for key, q in aws_sqs_queue.worker : key => {
        arn                        = q.arn
        visibility_timeout_seconds = var.visibility_timeout_seconds
      }
    },
  )
}

output "workers" {
  description = "The job types of each worker, which the app compares with its own."
  value       = module.workers.job_types
}
