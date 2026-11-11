# SPDX-License-Identifier: AGPL-3.0-only

# A domain of Amazon OpenSearch Service that the operator created, reached with requests signed by the tasks' role. It reads
# the domain and creates nothing.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

variable "domain_name" {
  description = "The name of the domain, in the region of the provider. It must exist, and its access policy must let the tasks' role in."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,27}$", var.domain_name))
    error_message = "domain_name must be the name of a domain of Amazon OpenSearch Service: 3 to 28 lowercase letters, digits, and hyphens, starting with a letter."
  }
}

variable "index" {
  description = "The alias of the index, search_opensearch_index. Null keeps the server's default, my-statuses."
  type        = string
  default     = null

  validation {
    condition     = var.index == null || can(regex("^[a-z0-9][a-z0-9_-]*$", var.index))
    error_message = "index must hold lowercase letters, digits, - and _, and start with a letter or a digit."
  }
}

variable "security_group_ids" {
  description = "Security groups that the tasks join to reach a domain in the VPC, whose own security group admits them. Empty for a domain with a public endpoint."
  type        = list(string)
  default     = []
  nullable    = false
}

data "aws_opensearch_domain" "this" {
  domain_name = var.domain_name
}

locals {
  arn = data.aws_opensearch_domain.this.arn
  # A domain in a VPC has a VPC, which must be the app's.
  vpc_id = try(data.aws_opensearch_domain.this.vpc_options[0].vpc_id, "")
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "search = opensearch at the domain's endpoint, with every request signed for the service es in the domain's region."
  value = merge(
    {
      MY_SEARCH                        = "opensearch"
      MY_SEARCH_OPENSEARCH_URL         = "https://${data.aws_opensearch_domain.this.endpoint}"
      MY_SEARCH_OPENSEARCH_AWS_SIGNING = "es"
      MY_SEARCH_OPENSEARCH_AWS_REGION  = split(":", local.arn)[3]
    },
    var.index == null ? {} : { MY_SEARCH_OPENSEARCH_INDEX = var.index },
  )
}

output "secret_env" {
  description = "No secrets: the requests are signed with the tasks' role."
  value       = {}
}

output "permissions" {
  description = "The HTTP methods the server sends to the domain, on its indexes."
  value = [
    for method in ["Get", "Post", "Put", "Delete"] : { permission = "es:ESHttp${method}", resource = "${local.arn}/*" }
  ]
}

output "security_group_ids" {
  description = "The security groups the tasks join to reach the domain."
  value       = var.security_group_ids
}

output "private_network" {
  description = "The VPC of a domain in one, which must be the app's, or null for a public endpoint."
  value       = local.vpc_id == "" ? null : local.vpc_id
}
