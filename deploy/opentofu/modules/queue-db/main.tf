# SPDX-License-Identifier: AGPL-3.0-only

# The job queue in the database: settings only, with nothing to create. It suits a process that stays up, since that process
# both polls the table and runs the periodic jobs. This module has never been applied: read the README.

terraform {
  required_version = ">= 1.11"
}

variable "workers" {
  description = "The same workers as the app takes. Nothing is needed of a queue in the database, since the app gives each worker worker_job_types. It is here so that the value is read once, and so that the app can see that its queue was given the same."
  type        = any
  default     = {}
  nullable    = false
}

module "workers" {
  source = "../workers"

  workers = var.workers
  fields  = ["min_instances", "max_instances", "instances", "cpu", "memory_mib", "concurrency"]
}

output "platform" {
  description = "The platform of this module: it works with any app."
  value       = "neutral"
}

output "env" {
  description = "The queue."
  value       = { MY_QUEUE = "db" }
}

output "secret_env" {
  description = "Nothing: the jobs are in the database that db-* connects to."
  value       = {}
}

output "permissions" {
  description = "Nothing."
  value       = []
}

output "workers" {
  description = "The job types of each worker, which the app compares with its own."
  value       = module.workers.job_types

  precondition {
    condition     = length(module.workers.problems) == 0
    error_message = "workers: ${join(" ", module.workers.problems)}"
  }
}
