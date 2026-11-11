# SPDX-License-Identifier: AGPL-3.0-only

# ElastiCache Serverless for Valkey on a network it takes from outside, with TLS, which it always has. No user group: a password would need
# a secret and the server reads the URL alone, so what keeps others out is the security group, and the URL holds no secret.
# Its public endpoint is not an option: that requires IAM authentication, which the server cannot do.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

variable "name" {
  description = "Name of the cache and start of the security groups' names."
  type        = string
  default     = "my-valkey"
  nullable    = false
}

variable "network" {
  description = "The network of the cache, from network-aws-vpc or an object of the same shape for a VPC of yours. The cache takes its addresses in the private subnets, and the app must take the same network."
  type = object({
    platform            = string
    vpc_id              = optional(string)
    vpc_cidr            = optional(string)
    public_subnet_ids   = optional(list(string), [])
    public_subnet_cidrs = optional(list(string), [])
    private_subnet_ids  = optional(list(string), [])
    nat                 = optional(bool, false)
  })
  nullable = false

  validation {
    condition     = var.network.platform == "aws"
    error_message = "network is of another platform: on AWS use network-aws-vpc, or an object of the shape in the README of deploy/opentofu."
  }

  validation {
    condition     = var.network.vpc_id != null
    error_message = "network.vpc_id is required."
  }

  validation {
    condition     = length(var.network.private_subnet_ids) > 0
    error_message = "network.private_subnet_ids must name at least one subnet: left empty, ElastiCache would choose the default ones of the VPC."
  }
}

resource "aws_security_group" "cache" {
  name_prefix = "${var.name}-cache-"
  description = "The cache of ${var.name}: only its clients reach it."
  vpc_id      = var.network.vpc_id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group" "client" {
  name_prefix = "${var.name}-client-"
  description = "Attach to what reaches the cache of ${var.name}."
  vpc_id      = var.network.vpc_id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "from_client" {
  security_group_id            = aws_security_group.cache.id
  referenced_security_group_id = aws_security_group.client.id
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  description                  = "Valkey over TLS from the clients"
}

# On its own the group would allow nothing out, so a task that has only this group can still reach the cache.
resource "aws_vpc_security_group_egress_rule" "to_cache" {
  security_group_id            = aws_security_group.client.id
  referenced_security_group_id = aws_security_group.cache.id
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  description                  = "Valkey over TLS to the cache"
}

resource "aws_elasticache_serverless_cache" "valkey" {
  name               = var.name
  engine             = "valkey"
  subnet_ids         = var.network.private_subnet_ids
  security_group_ids = [aws_security_group.cache.id]
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The settings of the key-value store. The URL holds no password, so it needs no secret, and no CA file is named, so the server uses the system roots."
  value = {
    MY_KV         = "valkey"
    MY_VALKEY_URL = "valkeys://${aws_elasticache_serverless_cache.valkey.endpoint[0].address}:${aws_elasticache_serverless_cache.valkey.endpoint[0].port}"
  }
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "permissions" {
  description = "Nothing: with no IAM authentication, no IAM action reaches the cache."
  value       = []
}

output "security_group_ids" {
  description = "The group that the app joins to reach the cache, in the same VPC: nothing else reaches it."
  value       = [aws_security_group.client.id]
}

output "private_network" {
  description = "The VPC of the cache, which must be the app's."
  value       = var.network.vpc_id
}
