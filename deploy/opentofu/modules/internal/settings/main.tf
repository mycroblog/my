# SPDX-License-Identifier: AGPL-3.0-only

# The settings and secrets inputs of a public module, checked against the server's configuration. It makes nothing: the public
# module passes what the operator wrote and the keys its sections set, and turns the outputs into environment variables and
# secrets. settings.json is generated from the configuration schema by scripts/task.sh go:config-docs, so it cannot drift.

terraform {
  required_version = ">= 1.11"
}

locals {
  # Every configuration key, with its MY_ name and whether it is a secret.
  all = jsondecode(file("${path.module}/settings.json"))
}

variable "name" {
  description = "The name of the deployment, which prefixes the names of the secrets."
  type        = string
  nullable    = false
}

variable "managed" {
  description = "The configuration keys that a section of the public module sets, or that the module fixes, which settings and secrets may not set again."
  type        = set(string)
  default     = []
  nullable    = false
}

variable "excluded" {
  description = "The configuration keys that the public module does not take, each to the reason, which settings and secrets refuse."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "settings" {
  description = "Configuration keys to their values, for the settings that are not secret."
  type        = map(string)
  default     = {}
  nullable    = false

  validation {
    condition     = length([for key in keys(var.settings) : key if !contains(keys(local.all), key)]) == 0
    error_message = "settings has keys that the configuration does not have: ${join(", ", sort([for key in keys(var.settings) : key if !contains(keys(local.all), key)]))}. The configuration reference lists the keys."
  }

  validation {
    condition     = length([for key in keys(var.settings) : key if try(local.all[key].secret, false)]) == 0
    error_message = "settings has secret settings, whose values would be in the plan and the state: ${join(", ", sort([for key in keys(var.settings) : key if try(local.all[key].secret, false)]))}. Name them in secrets instead, and fill the secrets that the module creates."
  }

  validation {
    condition     = length(setintersection(keys(var.settings), var.managed)) == 0
    error_message = "settings has keys that a section of this module sets: ${join(", ", sort(setintersection(keys(var.settings), var.managed)))}. Set them through the section."
  }

  validation {
    condition     = length(setintersection(keys(var.settings), keys(var.excluded))) == 0
    error_message = "settings has keys that this module does not take: ${join(", ", [for key in sort(setintersection(keys(var.settings), keys(var.excluded))) : "${key} (${var.excluded[key]})"])}."
  }
}

variable "secrets" {
  description = "The configuration keys of the secret settings that the module creates a secret for."
  type        = set(string)
  default     = []
  nullable    = false

  validation {
    condition     = length([for key in var.secrets : key if !contains(keys(local.all), key)]) == 0
    error_message = "secrets has keys that the configuration does not have: ${join(", ", sort([for key in var.secrets : key if !contains(keys(local.all), key)]))}. The configuration reference lists the keys."
  }

  validation {
    condition     = length([for key in var.secrets : key if contains(keys(local.all), key) && !try(local.all[key].secret, false)]) == 0
    error_message = "secrets has settings that are not secret: ${join(", ", sort([for key in var.secrets : key if contains(keys(local.all), key) && !try(local.all[key].secret, false)]))}. Set them in settings instead."
  }

  validation {
    condition     = length(setintersection(var.secrets, var.managed)) == 0
    error_message = "secrets has keys that a section of this module sets: ${join(", ", sort(setintersection(var.secrets, var.managed)))}. Set them through the section."
  }

  validation {
    condition     = length(setintersection(var.secrets, keys(var.excluded))) == 0
    error_message = "secrets has keys that this module does not take: ${join(", ", [for key in sort(setintersection(var.secrets, keys(var.excluded))) : "${key} (${var.excluded[key]})"])}."
  }
}

output "env" {
  description = "The environment variables of settings: each MY_ name to its value."
  value       = tomap({ for key, value in var.settings : local.all[key].env => value })
}

output "secrets" {
  description = "The secrets to create, by configuration key: the MY_ name that carries the value, and the name of the secret, <name>-<key> with hyphens for underscores."
  value = tomap({
    for key in var.secrets : key => {
      env    = local.all[key].env
      secret = "${var.name}-${replace(key, "_", "-")}"
    }
  })
}
