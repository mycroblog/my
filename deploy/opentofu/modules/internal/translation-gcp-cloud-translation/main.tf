# SPDX-License-Identifier: AGPL-3.0-only

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
  description = "The project that the translations are billed to, where the Cloud Translation API is enabled and the server's service account gets roles/cloudtranslate.user."
  type        = string
  nullable    = false
}

variable "app_project_id" {
  description = "The project of the app: a grant in it takes no resource, and one in another project names it."
  type        = string
  nullable    = false
}

variable "location" {
  description = "The location of the requests: global, or a location of Cloud Translation that serves the general model, such as us-central1."
  type        = string
  default     = "global"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*[a-z0-9]$", var.location))
    error_message = "location must be global or a location of Cloud Translation, such as us-central1."
  }
}

# Kept when the module goes: another configuration may use the API in the same project.
resource "google_project_service" "translate" {
  project            = var.project_id
  service            = "translate.googleapis.com"
  disable_on_destroy = false
}

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
}

output "env" {
  description = "The settings of the provider google: the project and the location. The server registers the provider, which the instance setting translation_providers enables."
  value = {
    MY_TRANSLATOR_GOOGLE_PROJECT  = var.project_id
    MY_TRANSLATOR_GOOGLE_LOCATION = var.location
  }
}

output "secret_env" {
  description = "No secrets: the server authenticates with the service account of the service."
  value       = {}
}

output "permissions" {
  description = "roles/cloudtranslate.user on the project, once the API is enabled. Built from the inputs, so that the app's binding is keyed at plan time."
  value = [{
    permission = "roles/cloudtranslate.user"
    scope      = "project"
    resource   = var.project_id == var.app_project_id ? null : var.project_id
  }]
  depends_on = [google_project_service.translate]
}
