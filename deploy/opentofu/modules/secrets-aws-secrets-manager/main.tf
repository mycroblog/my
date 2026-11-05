# SPDX-License-Identifier: AGPL-3.0-only

# Secrets in AWS Secrets Manager. It holds no value of its own: the operator adds what comes from elsewhere, what can be
# generated goes through an ephemeral resource and a write-only attribute, and what another module of the configuration hands
# over goes through an ephemeral input and a write-only attribute, so none of it reaches this module's plan or state.

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

variable "secrets" {
  description = "Secret settings by variable name, to the name of the secret. They become secret_env."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "referenced" {
  description = "Secrets that another module takes, by a key of your own, to the name of the secret. They appear only in ids."
  type        = map(string)
  default     = {}
  nullable    = false

  validation {
    condition     = length(setintersection(keys(var.secrets), keys(var.referenced))) == 0
    error_message = "A key can be in secrets or in referenced, not both."
  }
}

variable "generate_p256_keys" {
  description = "Keys of var.secrets whose value the first apply generates: a P-256 private key as 32 bytes in base64url, the format of vapid_private_key. Decide before the first apply: added later, it writes a new current version over the value you have."
  type        = set(string)
  default     = []
  nullable    = false

  validation {
    condition     = length(setsubtract(var.generate_p256_keys, keys(var.secrets))) == 0
    error_message = "Each key of generate_p256_keys must be a key of secrets."
  }
}

variable "write_keys" {
  description = "Keys of var.secrets or var.referenced whose value comes from write_values, such as the connection URL of a database another module creates. A separate input because the keys must be known at plan time, and write_values need not be."
  type        = set(string)
  default     = []
  nullable    = false

  validation {
    condition     = length(setsubtract(var.write_keys, concat(keys(var.secrets), keys(var.referenced)))) == 0
    error_message = "Each key of write_keys must be a key of secrets or referenced."
  }

  validation {
    condition     = length(setintersection(var.write_keys, var.generate_p256_keys)) == 0
    error_message = "A key can be generated or written, not both."
  }
}

variable "write_values" {
  description = "The value of each key of write_keys. Ephemeral: this module keeps none of them in the state, whatever the module they come from keeps."
  type        = map(string)
  default     = {}
  nullable    = false
  ephemeral   = true
}

variable "write_version" {
  description = "Raise it to write write_values again, after the value changed where it comes from. Nothing else can tell: the value is not in the state to compare."
  type        = number
  default     = 1
  nullable    = false
}

resource "aws_secretsmanager_secret" "this" {
  for_each = toset(concat(values(var.secrets), values(var.referenced)))

  name = each.value
}

# Any 32 bytes make a P-256 private key except zero and values at or above the curve's order, a chance of about 2^-32.
ephemeral "random_bytes" "p256" {
  for_each = var.generate_p256_keys

  length = 32
}

resource "aws_secretsmanager_secret_version" "generated" {
  for_each = var.generate_p256_keys

  secret_id = aws_secretsmanager_secret.this[var.secrets[each.key]].id
  # The value of random_bytes in base64url without padding.
  secret_string_wo = replace(replace(trimsuffix(ephemeral.random_bytes.p256[each.key].base64, "="), "+", "-"), "/", "_")

  # Do not change this: a new number writes a new random key as the current version, which for a VAPID key ends every
  # subscription. It is deliberately not an input.
  secret_string_wo_version = 1

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_secretsmanager_secret_version" "written" {
  for_each = var.write_keys

  secret_id                = aws_secretsmanager_secret.this[merge(var.secrets, var.referenced)[each.key]].id
  secret_string_wo         = var.write_values[each.key]
  secret_string_wo_version = var.write_version
}

output "env" {
  description = "No plain settings."
  value       = {}
}

output "secret_env" {
  description = "The settings of var.secrets: variable name to the secret's ARN."
  value       = { for key, name in var.secrets : key => aws_secretsmanager_secret.this[name].arn }
  depends_on  = [aws_secretsmanager_secret_version.generated, aws_secretsmanager_secret_version.written]
}

output "permissions" {
  description = "Nothing: the app module grants access to every secret it is given."
  value       = []
}

output "ids" {
  description = "The ARN of every secret, by the keys of var.secrets and var.referenced, for db-external, mail-smtp, and the like."
  value       = { for key, name in merge(var.secrets, var.referenced) : key => aws_secretsmanager_secret.this[name].arn }
  depends_on  = [aws_secretsmanager_secret_version.generated]
}
