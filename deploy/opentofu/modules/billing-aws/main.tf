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

variable "web_identity" {
  description = "Whether the server runs on Google Cloud, so that this module creates the role that billing_aws_role_arn names. Off, the app's own AWS identity gets the permission. A separate input because the unique ID is not known until the service account exists, and this must be known at plan time."
  type        = bool
  default     = false
  nullable    = false
}

variable "google_service_account_unique_id" {
  description = "Numeric unique ID of the server's Google service account, which alone may assume the role. Required with web_identity."
  type        = string
  default     = null

  validation {
    condition     = !var.web_identity || var.google_service_account_unique_id != null
    error_message = "web_identity needs google_service_account_unique_id: the trust policy must name the service account."
  }
}

variable "role_name" {
  description = "Name of the role that web_identity creates."
  type        = string
  default     = "my-billing-import"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z0-9+=,.@_-]{1,64}$", var.role_name))
    error_message = "role_name must be at most 64 letters, digits, and +=,.@_-."
  }
}

data "aws_caller_identity" "current" {
  lifecycle {
    enabled = var.web_identity
  }
}

data "aws_partition" "current" {
  lifecycle {
    enabled = var.web_identity
  }
}

locals {
  # Built from the inputs rather than read from the role: the trust policy names the role itself, and the output env must
  # not depend on the role, which depends on the app's service account, which depends on the backends.
  role_arn = var.web_identity ? "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:role/${var.role_name}" : null
}

resource "aws_iam_role" "import" {
  name        = var.role_name
  description = "Lets the usage cost import of my, on Google Cloud, read Cost Explorer."

  # aud is the azp claim of the token, the service account's ID. oaud is its aud claim, the audience that the server
  # requests, which is this role's ARN.
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = "accounts.google.com" }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "accounts.google.com:aud"  = var.google_service_account_unique_id
          "accounts.google.com:oaud" = local.role_arn
        }
      }
    }]
  })

  lifecycle {
    enabled = var.web_identity
  }
}

resource "aws_iam_role_policy" "import" {
  name = "cost-explorer"
  role = var.role_name

  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = "ce:GetCostAndUsage", Resource = "*" }]
  })

  depends_on = [aws_iam_role.import]

  lifecycle {
    enabled = var.web_identity
  }
}

output "billing_sources" {
  description = "What this module adds to MY_BILLING_SOURCES."
  value       = ["aws"]
}

output "env" {
  description = "The role to assume with web_identity. Without it the SDK's default chain finds the execution role."
  value       = var.web_identity ? { MY_BILLING_AWS_ROLE_ARN = local.role_arn } : {}
}

output "secret_env" {
  description = "No secrets: the server has an execution role or a token of its service account."
  value       = {}
}

output "permissions" {
  description = "The Cost Explorer action for the app's AWS identity. Empty with web_identity: the role here holds it, and a Google Cloud app refuses an IAM action."
  value = [
    for action in ["ce:GetCostAndUsage"] : { permission = action, scope = "policy", resource = "*", conditions = [] } if !var.web_identity
  ]
}

output "role_arn" {
  description = "The ARN of the role with web_identity, or null. Not part of the backend contract."
  value       = local.role_arn
}
