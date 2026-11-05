# SPDX-License-Identifier: AGPL-3.0-only

# A PostgreSQL that lives outside this configuration, such as Neon. It creates nothing: the connection URL is a secret
# the operator created by hand, so its value never reaches the state.

terraform {
  required_version = ">= 1.11"
}

variable "url_secret_id" {
  description = "Secret Manager secret that holds the connection URL (its ID, or projects/<project>/secrets/<id>)."
  type        = string
  nullable    = false
}

output "env" {
  description = "The settings of the database."
  value       = { MY_DATABASE = "postgres" }
}

output "secret_env" {
  description = "The secret settings of the database."
  value       = { MY_DATABASE_URL = var.url_secret_id }
}

output "roles" {
  description = "Nothing beyond reading the secret, which the app module grants for every secret it is given."
  value       = []
}
