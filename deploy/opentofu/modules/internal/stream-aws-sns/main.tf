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

data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
data "aws_region" "current" {}

variable "name" {
  description = "Name of the topic and start of the names of the queues that the server creates, used by this instance alone."
  type        = string
  default     = "my-stream"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z0-9_-]{1,40}$", var.name))
    error_message = "name must be at most 40 letters, digits, - and _: a queue name has at most 80 characters, and the server appends its process ID."
  }
}

locals {
  queues = "arn:${data.aws_partition.current.partition}:sqs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:${var.name}-*"
}

resource "aws_sns_topic" "stream" {
  name = var.name
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The settings of the stream."
  value = {
    MY_STREAM                  = "sns"
    MY_STREAM_SNS_TOPIC_ARN    = aws_sns_topic.stream.arn
    MY_STREAM_SNS_QUEUE_PREFIX = var.name
  }
}

output "secret_env" {
  description = "No secrets: the server authenticates with its execution role."
  value       = {}
}

output "permissions" {
  description = "Publishing to the topic and subscribing the server's own queues to it, and managing those queues, whose names are not known in advance."
  value = concat(
    [
      { permission = "sns:Publish", resource = aws_sns_topic.stream.arn, conditions = [] },
      {
        permission = "sns:Subscribe"
        resource   = aws_sns_topic.stream.arn
        conditions = [
          { test = "StringEquals", variable = "sns:Protocol", values = ["sqs"] },
          { test = "StringLike", variable = "sns:Endpoint", values = [local.queues] },
        ]
      },
      { permission = "sns:Unsubscribe", resource = aws_sns_topic.stream.arn, conditions = [] },
      { permission = "sns:ListSubscriptionsByTopic", resource = aws_sns_topic.stream.arn, conditions = [] },
      # The sweep lists the queues, which this action cannot limit by name.
      { permission = "sqs:ListQueues", resource = "*", conditions = [] },
    ],
    [
      for action in ["CreateQueue", "TagQueue", "SetQueueAttributes", "GetQueueAttributes", "ReceiveMessage", "DeleteMessage", "DeleteQueue"] : {
        permission = "sqs:${action}", resource = local.queues, conditions = []
      }
    ],
  )
}
