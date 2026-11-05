# SPDX-License-Identifier: AGPL-3.0-only

# The KEK in Cloud KMS, which it never leaves: key_manager = gcpkms.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

variable "project_id" {
  description = "Project of the key."
  type        = string
  nullable    = false
}

variable "location" {
  description = "Location of the key ring."
  type        = string
  nullable    = false
}

variable "key_ring" {
  description = "Name of the key ring to create."
  type        = string
  default     = "my"
  nullable    = false
}

variable "key" {
  description = "Name of the key to create."
  type        = string
  default     = "my-kek"
  nullable    = false
}

variable "crypto_key_id" {
  description = "Resource name of a key made elsewhere, without a version. Set, nothing is created."
  type        = string
  default     = null

  validation {
    condition     = var.crypto_key_id == null || can(regex("^projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+$", var.crypto_key_id))
    error_message = "crypto_key_id must be projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}, without a version."
  }
}

locals {
  create = var.crypto_key_id == null
  # Built from the inputs so that it is known at plan time: the app module keys its IAM bindings on it. The outputs depend
  # on the resources for the order.
  crypto_key_id = local.create ? "projects/${var.project_id}/locations/${var.location}/keyRings/${var.key_ring}/cryptoKeys/${var.key}" : var.crypto_key_id
}

resource "google_kms_key_ring" "this" {
  project  = var.project_id
  location = var.location
  name     = var.key_ring

  lifecycle {
    enabled = local.create
  }
}

resource "google_kms_crypto_key" "kek" {
  name     = var.key
  key_ring = google_kms_key_ring.this.id
  purpose  = "ENCRYPT_DECRYPT"

  # Destroying the key loses every secret it wrapped.
  lifecycle {
    enabled = local.create

    prevent_destroy = true
  }
}

output "env" {
  description = "The settings of the key manager."
  value = {
    MY_KEY_MANAGER = "gcpkms"
    MY_KMS_KEY     = local.crypto_key_id
  }
  depends_on = [google_kms_crypto_key.kek]
}

output "secret_env" {
  description = "None: the server authenticates with its service account."
  value       = {}
}

output "roles" {
  description = "Encrypting and decrypting with this key, not the others of the project."
  value = [
    { role = "roles/cloudkms.cryptoKeyEncrypterDecrypter", scope = "crypto_key", resource = local.crypto_key_id },
  ]
  depends_on = [google_kms_crypto_key.kek]
}
