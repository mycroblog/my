# SPDX-License-Identifier: AGPL-3.0-only

# Creates secrets. The values reach neither the plan nor this module's state: the operator adds each with gcloud secrets
# versions add; for the ones listed in generate_p256_keys, the first apply generates it through an ephemeral resource and a
# write-only attribute; and for the ones listed in write_keys, another module of the configuration hands it over, through an
# ephemeral input and a write-only attribute.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.7, < 4.0"
    }
  }
}

variable "project_id" {
  description = "Project of the secrets."
  type        = string
  nullable    = false
}

variable "secrets" {
  description = "Settings this module provides: variable name to the ID of the secret to create."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "referenced" {
  description = "Secrets another module sets its own setting from: any key to the ID of the secret to create. They are created and listed in ids, but not in secret_env, or the setting would be named twice."
  type        = map(string)
  default     = {}
  nullable    = false

  validation {
    condition     = length(setintersection(keys(var.secrets), keys(var.referenced))) == 0
    error_message = "A key can be in secrets or in referenced, not both."
  }
}

variable "generate_p256_keys" {
  description = "Keys of var.secrets whose value the first apply generates: a P-256 private key as 32 bytes in base64url, the format of vapid_private_key. Decide before the first apply: added later, it writes a new latest version over the value you have."
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

resource "google_secret_manager_secret" "this" {
  for_each = toset(concat(values(var.secrets), values(var.referenced)))

  project   = var.project_id
  secret_id = each.value

  replication {
    auto {}
  }
}

# Any 32 bytes make a P-256 private key except zero and values at or above the curve's order, a chance of about 2^-32.
ephemeral "random_bytes" "p256" {
  for_each = var.generate_p256_keys

  length = 32
}

resource "google_secret_manager_secret_version" "generated" {
  for_each = var.generate_p256_keys

  secret = google_secret_manager_secret.this[var.secrets[each.key]].id
  # The value of random_bytes in base64url without padding.
  secret_data_wo = replace(replace(trimsuffix(ephemeral.random_bytes.p256[each.key].base64, "="), "+", "-"), "/", "_")

  # Do not change this: a new number writes a new random key as the latest version, which for a VAPID key ends every
  # subscription. It is deliberately not an input.
  secret_data_wo_version = 1

  # Should this resource ever leave the configuration, the version stays in Secret Manager.
  deletion_policy = "ABANDON"

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_secret_manager_secret_version" "written" {
  for_each = var.write_keys

  secret                 = google_secret_manager_secret.this[merge(var.secrets, var.referenced)[each.key]].id
  secret_data_wo         = var.write_values[each.key]
  secret_data_wo_version = var.write_version

  # A new version replaces this one, and the app reads the latest: nothing has to delete the old one first.
  deletion_policy = "ABANDON"
}

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
}

output "env" {
  description = "No plain settings."
  value       = {}
}

output "secret_env" {
  description = "The settings of var.secrets: variable name to secret ID."
  value       = var.secrets
  depends_on  = [google_secret_manager_secret.this, google_secret_manager_secret_version.generated, google_secret_manager_secret_version.written]
}

output "permissions" {
  description = "Nothing: the app module grants access to every secret it is given."
  value       = []
}

output "ids" {
  description = "The IDs of every secret, by the keys of var.secrets and var.referenced, for db-external, mail-smtp, and the like."
  value       = merge(var.secrets, var.referenced)
  depends_on  = [google_secret_manager_secret.this]
}
