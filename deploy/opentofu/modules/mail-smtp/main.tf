# SPDX-License-Identifier: AGPL-3.0-only

# Email through any SMTP service. It creates nothing: the password is a secret the operator created by hand, so its value
# never reaches the state.

terraform {
  required_version = ">= 1.11"
}

variable "from" {
  description = "Sender address of emails, as a bare address such as noreply@social.example."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^[^@<>[:space:]]+@[^@<>[:space:]]+$", var.from))
    error_message = "from must be a bare address, such as noreply@social.example."
  }
}

variable "host" {
  description = "Host name of the SMTP server, which must offer STARTTLS with a certificate valid for this name."
  type        = string
  nullable    = false
}

variable "port" {
  description = "TCP port of the SMTP server (submission)."
  type        = number
  default     = 587
  nullable    = false
}

variable "username" {
  description = "User name for SMTP AUTH. Null for a server that needs no authentication."
  type        = string
  default     = null
}

variable "password_secret_id" {
  description = "Secret Manager secret that holds the SMTP password (its ID, or projects/<project>/secrets/<id>). Null for a server that needs no authentication."
  type        = string
  default     = null

  validation {
    condition     = (var.password_secret_id == null) == (var.username == null)
    error_message = "username and password_secret_id go together: set both, or neither for a server that needs no authentication."
  }
}

output "env" {
  description = "The settings of the mailer."
  value = merge(
    {
      MY_MAIL           = "smtp"
      MY_MAIL_FROM      = var.from
      MY_MAIL_SMTP_HOST = var.host
      MY_MAIL_SMTP_PORT = tostring(var.port)
    },
    var.username == null ? {} : { MY_MAIL_SMTP_USERNAME = var.username },
  )
}

output "secret_env" {
  description = "The secret settings of the mailer."
  value       = var.password_secret_id == null ? {} : { MY_MAIL_SMTP_PASSWORD = var.password_secret_id }
}

output "roles" {
  description = "Nothing beyond reading the secret, which the app module grants for every secret it is given."
  value       = []
}
