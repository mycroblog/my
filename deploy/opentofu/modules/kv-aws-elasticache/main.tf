# SPDX-License-Identifier: AGPL-3.0-only

# ElastiCache Serverless for Valkey in the operator's VPC, with TLS, which it always has. No user group: a password would need
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

variable "vpc_id" {
  description = "VPC of the cache and of the tasks that reach it."
  type        = string
  nullable    = false
}

variable "subnet_ids" {
  description = "Subnets where the cache takes its addresses, which the tasks must reach."
  type        = list(string)
  nullable    = false

  validation {
    condition     = length(var.subnet_ids) > 0
    error_message = "subnet_ids must name at least one subnet: left empty, ElastiCache would choose the default ones of the VPC."
  }
}

resource "aws_security_group" "cache" {
  name_prefix = "${var.name}-cache-"
  description = "The cache of ${var.name}: only its clients reach it."
  vpc_id      = var.vpc_id

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group" "client" {
  name_prefix = "${var.name}-client-"
  description = "Attach to what reaches the cache of ${var.name}."
  vpc_id      = var.vpc_id

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
  subnet_ids         = var.subnet_ids
  security_group_ids = [aws_security_group.cache.id]
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
