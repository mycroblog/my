# SPDX-License-Identifier: AGPL-3.0-only

# Amazon Translate needs no resource: the provider aws of the server, its region, and two IAM actions. It declares no provider.

terraform {
  required_version = ">= 1.11"
}

variable "region" {
  description = "The region of Amazon Translate, such as us-east-1. Detecting the language of a post needs Amazon Comprehend there too."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^[a-z]{2}(-[a-z]+)+-[0-9]+$", var.region))
    error_message = "region must be an AWS region, such as us-east-1."
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The region of the provider aws, which the server registers and the instance setting translation_providers enables."
  value = {
    MY_TRANSLATOR_AWS_REGION = var.region
  }
}

output "secret_env" {
  description = "No secrets: the server authenticates with the tasks' role."
  value       = {}
}

output "permissions" {
  description = "translate:TranslateText, and comprehend:DetectDominantLanguage, which Amazon Translate calls with the caller's identity for a post whose language is unknown. Neither action takes a resource."
  value = [
    { permission = "translate:TranslateText", resource = "*" },
    { permission = "comprehend:DetectDominantLanguage", resource = "*" },
  ]
}
