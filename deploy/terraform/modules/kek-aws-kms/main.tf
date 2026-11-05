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

variable "alias" {
  description = "Alias of the key it creates, without alias/."
  type        = string
  default     = "my-kek"
  nullable    = false
}

variable "key_arn" {
  description = "ARN of a symmetric customer managed key made elsewhere. Set, nothing is created."
  type        = string
  default     = null

  validation {
    condition     = var.key_arn == null || can(regex("^arn:[a-z-]+:kms:[a-z0-9-]+:[0-9]{12}:key/.+$", var.key_arn))
    error_message = "key_arn must be arn:aws:kms:{region}:{account}:key/{key-id}."
  }
}

locals {
  create = var.key_arn == null
  arn    = local.create ? aws_kms_key.kek[0].arn : var.key_arn
}

# Rotation keeps the ARN and the earlier key material, so it needs no my kek rewrap.
resource "aws_kms_key" "kek" {
  count = local.create ? 1 : 0

  description             = "Wraps the data-encryption keys of my"
  key_usage               = "ENCRYPT_DECRYPT"
  enable_key_rotation     = true
  deletion_window_in_days = 30

  # Destroying the key loses every secret it wrapped.
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_kms_alias" "kek" {
  count = local.create ? 1 : 0

  name          = "alias/${var.alias}"
  target_key_id = aws_kms_key.kek[0].key_id
}

output "env" {
  description = "The settings of the key manager."
  value = {
    MY_KEY_MANAGER = "awskms"
    MY_KMS_KEY     = local.arn
  }
}

output "secret_env" {
  description = "None: the server authenticates with its execution role."
  value       = {}
}

output "roles" {
  description = "Encrypting and decrypting with this key, not the others of the account."
  value = [
    { role = "kms:Encrypt", scope = "policy", resource = local.arn },
    { role = "kms:Decrypt", scope = "policy", resource = local.arn },
    { role = "kms:DescribeKey", scope = "policy", resource = local.arn },
  ]
}
