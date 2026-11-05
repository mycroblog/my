# SPDX-License-Identifier: AGPL-3.0-only

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
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

variable "configuration_set" {
  description = "Name of a configuration set to create and apply to every message, for its event destinations. Null sends without one."
  type        = string
  default     = null
}

locals {
  domain = split("@", var.from)[1]
}

resource "aws_sesv2_configuration_set" "this" {
  count = var.configuration_set == null ? 0 : 1

  configuration_set_name = var.configuration_set
}

# The domain, so that any address of it can send. It verifies once dns_records are in place.
resource "aws_sesv2_email_identity" "domain" {
  email_identity = local.domain
}

output "env" {
  description = "The settings of the mailer. No region: Lambda sets AWS_REGION, which the server takes when mail_ses_region is empty, so the identity must be in the function's region."
  value = merge(
    {
      MY_MAIL      = "ses"
      MY_MAIL_FROM = var.from
    },
    var.configuration_set == null ? {} : { MY_MAIL_SES_CONFIGURATION_SET = var.configuration_set },
  )
}

output "secret_env" {
  description = "No secrets: the server authenticates with its execution role."
  value       = {}
}

output "roles" {
  description = "Sending from the identity and, with one, through the configuration set."
  value = concat(
    [{ role = "ses:SendEmail", scope = "policy", resource = aws_sesv2_email_identity.domain.arn }],
    [for set in aws_sesv2_configuration_set.this : { role = "ses:SendEmail", scope = "policy", resource = set.arn }],
  )
}

output "dns_records" {
  description = "The DKIM records that verify the domain, to add at the DNS provider."
  value = [
    for token in try(aws_sesv2_email_identity.domain.dkim_signing_attributes[0].tokens, []) : {
      name   = "${token}._domainkey.${local.domain}"
      type   = "CNAME"
      rrdata = "${token}.dkim.amazonses.com"
    }
  ]
}
