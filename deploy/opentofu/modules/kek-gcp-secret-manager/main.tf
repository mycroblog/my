# SPDX-License-Identifier: AGPL-3.0-only

# The KEK in Secret Manager, which reaches the server as a mounted file. This module generates it on the first apply: the
# value goes to Secret Manager through an ephemeral resource and a write-only attribute, so it reaches neither the plan
# nor the state.

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
  description = "Project of the secret."
  type        = string
  nullable    = false
}

variable "secret_id" {
  description = "ID of the secret that holds the KEK."
  type        = string
  default     = "my-kek"
  nullable    = false
}

variable "create" {
  description = "Creates the secret and generates the KEK. Set it to false for a secret made elsewhere, whose value you manage."
  type        = bool
  default     = true
  nullable    = false
}

# Standard base64 of 32 random bytes, the format of kek.
ephemeral "random_bytes" "kek" {
  length = 32

  lifecycle {
    enabled = var.create
  }
}

resource "google_secret_manager_secret" "kek" {
  project   = var.project_id
  secret_id = var.secret_id

  replication {
    auto {}
  }

  lifecycle {
    enabled = var.create

    prevent_destroy = true
  }
}

resource "google_secret_manager_secret_version" "kek" {
  secret         = google_secret_manager_secret.kek.id
  secret_data_wo = ephemeral.random_bytes.kek.base64

  # NEVER CHANGE THIS. A new number writes a new random KEK as the latest version, and everything in the database that the
  # old one encrypted, every account's private key among it, becomes unreadable. It is deliberately not an input.
  # Rotate through my kek rewrap instead: docs/operations/key-management.md.
  secret_data_wo_version = 1

  # Should this resource ever leave the configuration, the version stays in Secret Manager.
  deletion_policy = "ABANDON"

  lifecycle {
    enabled = var.create

    prevent_destroy = true
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
}

output "env" {
  description = "No plain settings: key_manager stays at its default, local."
  value       = {}
}

output "secret_env" {
  description = "None: the KEK is a file, not an environment variable."
  value       = {}
}

output "secret_files" {
  description = "The KEK, mounted as a file that MY_KEK_FILE names."
  value       = { MY_KEK_FILE = var.secret_id }
  depends_on  = [google_secret_manager_secret_version.kek]
}

output "permissions" {
  description = "Nothing: the app module grants access to every secret it is given."
  value       = []
}
