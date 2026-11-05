# SPDX-License-Identifier: AGPL-3.0-only

# PostgreSQL on Amazon RDS, in the operator's VPC, for the functions or tasks that join its client security group. This module
# has never been applied: read the README. The password is generated at apply through an ephemeral resource and goes to the
# instance and to the URL secret through write-only attributes, so it reaches neither the plan nor the state.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.7, < 4.0"
    }
  }
}

variable "name" {
  description = "Start of the names."
  type        = string
  default     = "my"
  nullable    = false
}

variable "vpc_id" {
  description = "VPC of the instance and its security groups."
  type        = string
  nullable    = false
}

variable "subnet_ids" {
  description = "Private subnets of the instance, in at least two Availability Zones, which RDS requires even of a single instance."
  type        = list(string)
  nullable    = false

  validation {
    condition     = length(var.subnet_ids) >= 2
    error_message = "RDS needs subnets in at least two Availability Zones."
  }
}

variable "database" {
  description = "Name of the database."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9_]*$", var.database))
    error_message = "database goes into a URL as it is: lower-case letters, digits, and underscores, starting with a letter."
  }
}

variable "username" {
  description = "Name of the master user, which the server connects as."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9_]*$", var.username))
    error_message = "username goes into a URL as it is: lower-case letters, digits, and underscores, starting with a letter."
  }
}

variable "url_secret_name" {
  description = "Name of the secret this module creates to hold the connection URL."
  type        = string
  default     = "my-database-url"
  nullable    = false
}

variable "engine_version" {
  description = "PostgreSQL version of the instance. The server is tested on 18."
  type        = string
  default     = "18"
  nullable    = false
}

variable "instance_class" {
  description = "Instance class. The default is the smallest of its family."
  type        = string
  default     = "db.t4g.micro"
  nullable    = false
}

variable "allocated_storage_gb" {
  description = "Storage at creation, in GB. It never shrinks."
  type        = number
  default     = 20
  nullable    = false
}

variable "max_allocated_storage_gb" {
  description = "How far storage may grow by itself, in GB. Equal to allocated_storage_gb, it does not grow."
  type        = number
  default     = 100
  nullable    = false
}

variable "backup_retention_days" {
  description = "How many days of automated backups and point-in-time recovery to keep, up to 35. 0 turns them off and leaves nothing to restore from."
  type        = number
  default     = 7
  nullable    = false
}

variable "multi_az" {
  description = "Keeps a standby in another Availability Zone, for about twice the price."
  type        = bool
  default     = false
  nullable    = false
}

variable "deletion_protection" {
  description = "Refuses to delete the instance in AWS's API. prevent_destroy refuses it in OpenTofu in any case."
  type        = bool
  default     = true
  nullable    = false
}

variable "password_version" {
  description = "Raise it to generate a new password, which goes to the instance and into a new version of the URL secret in the same apply. Also the way out of an apply that stopped halfway, and what writes the URL again after url_parameters changed. What already runs keeps the URL it started with."
  type        = number
  default     = 1
  nullable    = false
}

variable "url_parameters" {
  description = "Parameters to add to the URL beside sslmode. Takes effect when password_version is raised, since nothing else rewrites the secret. pool_max_conns may be one of them."
  type        = map(string)
  default     = {}
  nullable    = false
}

locals {
  parameters = merge({ sslmode = "require" }, var.url_parameters)
  query      = join("&", [for k, v in local.parameters : "${k}=${v}"])
  # The instance's own endpoint, not RDS Proxy: my schema apply holds a session-level lock. sslmode=require encrypts and does
  # not check the certificate, since the server image does not hold the CA bundle of RDS.
  url_after_password = "@${aws_db_instance.db.address}:${aws_db_instance.db.port}/${var.database}?${local.query}"
}

# Letters and digits only, so that the URL needs no escaping and RDS takes it; 40 of them are about 238 bits.
ephemeral "random_password" "db" {
  length  = 40
  special = false
}

resource "aws_db_subnet_group" "db" {
  name       = var.name
  subnet_ids = var.subnet_ids
}

resource "aws_security_group" "db" {
  name        = "${var.name}-db"
  description = "The database of my"
  vpc_id      = var.vpc_id
}

# What joins this group can reach the database, and nothing else about it changes: a function also needs a group of its own for
# the way out to the internet.
resource "aws_security_group" "clients" {
  name        = "${var.name}-db-clients"
  description = "What may connect to the database of my"
  vpc_id      = var.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "db_from_clients" {
  security_group_id            = aws_security_group.db.id
  referenced_security_group_id = aws_security_group.clients.id
  ip_protocol                  = "tcp"
  from_port                    = aws_db_instance.db.port
  to_port                      = aws_db_instance.db.port
}

resource "aws_vpc_security_group_egress_rule" "clients_to_db" {
  security_group_id            = aws_security_group.clients.id
  referenced_security_group_id = aws_security_group.db.id
  ip_protocol                  = "tcp"
  from_port                    = aws_db_instance.db.port
  to_port                      = aws_db_instance.db.port
}

resource "aws_db_instance" "db" {
  identifier     = var.name
  engine         = "postgres"
  engine_version = var.engine_version
  instance_class = var.instance_class
  db_name        = var.database

  username            = var.username
  password_wo         = ephemeral.random_password.db.result
  password_wo_version = var.password_version

  allocated_storage     = var.allocated_storage_gb
  max_allocated_storage = var.max_allocated_storage_gb
  storage_type          = "gp3"
  storage_encrypted     = true

  db_subnet_group_name   = aws_db_subnet_group.db.name
  vpc_security_group_ids = [aws_security_group.db.id]
  publicly_accessible    = false
  multi_az               = var.multi_az

  backup_retention_period  = var.backup_retention_days
  delete_automated_backups = false
  copy_tags_to_snapshot    = true

  deletion_protection       = var.deletion_protection
  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name}-final"

  auto_minor_version_upgrade = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_secretsmanager_secret" "url" {
  name = var.url_secret_name
}

# The same ephemeral value as the instance's within one apply. Keep the two versions together: raising only one would leave a
# URL whose password the database does not know.
resource "aws_secretsmanager_secret_version" "url" {
  secret_id                = aws_secretsmanager_secret.url.id
  secret_string_wo         = "postgres://${var.username}:${ephemeral.random_password.db.result}${local.url_after_password}"
  secret_string_wo_version = var.password_version
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The settings of the database."
  value       = { MY_DATABASE = "postgres" }
}

output "secret_env" {
  description = "The connection URL, by the ARN of its secret."
  value       = { MY_DATABASE_URL = aws_secretsmanager_secret.url.arn }
  depends_on  = [aws_secretsmanager_secret_version.url]
}

output "direct_secret_env" {
  description = "Empty: the only connection is direct, so one-off work and the service use the same one."
  value       = {}
}

output "permissions" {
  description = "Nothing beyond reading the secret, which the app module grants for every secret it is given."
  value       = []
}

output "security_group_ids" {
  description = "The group that the app joins to reach the database."
  value       = [aws_security_group.clients.id]
}

output "url_without_password" {
  description = "The URL in the secret, with PASSWORD in place of the password."
  value       = "postgres://${var.username}:PASSWORD${local.url_after_password}"
}
