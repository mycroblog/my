# SPDX-License-Identifier: AGPL-3.0-only

# PostgreSQL LISTEN / NOTIFY as the broker of the real-time updates, on the database the server already has. It creates nothing.

terraform {
  required_version = ">= 1.11"
}

# Null by default, so that beside a db-* module that returns its direct connection, and sets MY_STREAM_POSTGRES_URL from it,
# this module names nothing and the setting has one owner. Set beside such a module, it fails the plan as a duplicate.
variable "listen_url_secret_id" {
  description = "Secret that holds a direct connection string to the same database, only when nothing else names one and the database's URL goes through a pooler in transaction mode, where LISTEN does not work. Null, the default, leaves it to the db-* module, or listens on the database's URL."
  type        = string
  default     = null
}

output "platform" {
  description = "The platform of this module: it works with any app."
  value       = "neutral"
}

output "env" {
  description = "The settings of the stream."
  value       = { MY_STREAM = "postgres" }
}

output "secret_env" {
  description = "The connection string of the listeners, by reference, when it is not the database's."
  value       = var.listen_url_secret_id == null ? {} : { MY_STREAM_POSTGRES_URL = var.listen_url_secret_id }
}

output "permissions" {
  description = "Nothing beyond reading the secret, which the app module grants for every secret it is given."
  value       = []
}
