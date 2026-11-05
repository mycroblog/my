# SPDX-License-Identifier: AGPL-3.0-only

# The KEK in AWS Secrets Manager. This module generates it on the first apply: the value goes to Secrets Manager through an
# ephemeral resource and a write-only attribute, so it reaches neither the plan nor the state. Lambda mounts nothing, so unlike
# kek-gcp-secret-manager it returns secret_env rather than secret_files.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.7, < 4.0"
    }
  }
}

variable "secret_name" {
  description = "Name of the secret that holds the KEK."
  type        = string
  default     = "my-kek"
  nullable    = false
}

variable "secret_arn" {
  description = "ARN of a secret made elsewhere, whose value you manage. Set, nothing is created."
  type        = string
  default     = null
}

locals {
  create = var.secret_arn == null
}

# Standard base64 of 32 random bytes, the format of kek.
ephemeral "random_bytes" "kek" {
  count = local.create ? 1 : 0

  length = 32
}

resource "aws_secretsmanager_secret" "kek" {
  count = local.create ? 1 : 0

  name = var.secret_name

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_secretsmanager_secret_version" "kek" {
  count = local.create ? 1 : 0

  secret_id        = aws_secretsmanager_secret.kek[0].id
  secret_string_wo = ephemeral.random_bytes.kek[0].base64

  # NEVER CHANGE THIS. A new number writes a new random KEK as the current version, and everything in the database that the
  # old one encrypted, every account's private key among it, becomes unreadable. It is deliberately not an input.
  # Rotate through my kek rewrap instead: docs/operations/key-management.md.
  secret_string_wo_version = 1

  lifecycle {
    prevent_destroy = true
  }
}

output "env" {
  description = "No plain settings: key_manager stays at its default, local."
  value       = {}
}

output "secret_env" {
  description = "The KEK, by the ARN of its secret."
  value       = { MY_KEK = local.create ? aws_secretsmanager_secret.kek[0].arn : var.secret_arn }
  depends_on  = [aws_secretsmanager_secret_version.kek]
}

output "roles" {
  description = "Nothing: the app module grants access to every secret it is given."
  value       = []
}
