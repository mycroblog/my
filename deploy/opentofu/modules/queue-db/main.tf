# SPDX-License-Identifier: AGPL-3.0-only

# The job queue in the database: settings only, with nothing to create. It suits a process that stays up, since that process
# both polls the table and runs the periodic jobs. This module has never been applied: read the README.

terraform {
  required_version = ">= 1.11"
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
