# SPDX-License-Identifier: AGPL-3.0-only

# A stand-in with the interface of the settings module, so that mycroblog-gcp can be planned and tested. The real module, with
# the list of settings and the checks of what settings and secrets may name, replaces this file (#2835).

terraform {
  required_version = ">= 1.11"
}

variable "name" {
  description = "Prefix of the names of the secrets."
  type        = string
  nullable    = false
}

variable "settings" {
  description = "Settings by configuration key, such as translator_deepl_url."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secrets" {
  description = "Secret settings by configuration key."
  type        = set(string)
  default     = []
  nullable    = false
}

variable "managed" {
  description = "The configuration keys that the sections of the public module set."
  type        = set(string)
  default     = []
  nullable    = false
}

output "env" {
  description = "The settings: MY_ name to value."
  value       = { for key, value in var.settings : "MY_${upper(key)}" => value }
}

output "secrets" {
  description = "The secret settings: configuration key to its MY_ name and the ID of the secret that holds it."
  value       = { for key in var.secrets : key => { env = "MY_${upper(key)}", secret = "${var.name}-${replace(key, "_", "-")}" } }
}
